package ssh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	sshLib "golang.org/x/crypto/ssh"
)

const (
	socksVersion             = 5
	socksNoAuthentication    = 0
	socksNoAcceptableMethods = 255
	socksConnect             = 1
	socksAddressIPv4         = 1
	socksAddressDomain       = 3
	socksAddressIPv6         = 4
	socksReplySucceeded      = 0
	socksReplyGeneralFailure = 1
	defaultReconnectDelay    = time.Second
	maximumReconnectDelay    = 30 * time.Second
	defaultSSHDialTimeout    = 10 * time.Second
)

type Config struct {
	Host           string
	Port           int
	User           string
	PrivateKeyPath string
	HostKey        string
	ReconnectDelay time.Duration
	DialTimeout    time.Duration
}

type remoteDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

type Tunnel struct {
	config Config

	listener net.Listener
	cancel   context.CancelFunc
	ctx      context.Context
	onHealth func(bool)

	mu     sync.RWMutex
	remote remoteDialer

	closeOnce sync.Once
}

func Start(ctx context.Context, config Config, onHealth func(bool)) (*Tunnel, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen SOCKS: %w", err)
	}

	runContext, cancel := context.WithCancel(ctx)
	tunnel := &Tunnel{config: config, listener: listener, cancel: cancel, ctx: runContext, onHealth: onHealth}
	tunnel.setHealth(false)
	go tunnel.accept(runContext)
	go tunnel.supervise(runContext)
	go func() {
		<-runContext.Done()
		_ = listener.Close()
	}()
	return tunnel, nil
}

func (t *Tunnel) ProxyURL() string {
	return "socks5h://" + t.listener.Addr().String()
}

func (t *Tunnel) Close() error {
	var closeErr error
	t.closeOnce.Do(func() {
		t.cancel()
		closeErr = t.listener.Close()
		if client, ok := t.currentRemote().(*sshLib.Client); ok {
			_ = client.Close()
		}
	})
	return closeErr
}

func (t *Tunnel) accept(ctx context.Context) {
	for {
		connection, err := t.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		go t.serveSOCKS(connection)
	}
}

func (t *Tunnel) supervise(ctx context.Context) {
	delay := t.config.reconnectDelay()
	for ctx.Err() == nil {
		client, err := t.connect(ctx)
		if err != nil {
			t.setHealth(false)
			if !wait(ctx, delay) {
				return
			}
			delay = nextReconnectDelay(delay)
			continue
		}

		t.setRemote(client)
		t.setHealth(true)
		delay = t.config.reconnectDelay()
		if !t.waitForDisconnect(ctx, client) {
			return
		}
		t.setRemote(nil)
		t.setHealth(false)
		if !wait(ctx, delay) {
			return
		}
		delay = nextReconnectDelay(delay)
	}
}

func nextReconnectDelay(delay time.Duration) time.Duration {
	if delay >= maximumReconnectDelay/2 {
		return maximumReconnectDelay
	}
	return delay * 2
}

func (t *Tunnel) connect(ctx context.Context) (*sshLib.Client, error) {
	config, err := t.config.clientConfig()
	if err != nil {
		return nil, err
	}

	dialer := net.Dialer{Timeout: t.config.dialTimeout()}
	connection, err := dialer.DialContext(ctx, "tcp", t.config.address())
	if err != nil {
		return nil, err
	}

	sshConnection, channels, requests, err := sshLib.NewClientConn(connection, t.config.address(), config)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	return sshLib.NewClient(sshConnection, channels, requests), nil
}

func (t *Tunnel) waitForDisconnect(ctx context.Context, client *sshLib.Client) bool {
	disconnected := make(chan struct{})
	go func() {
		_ = client.Wait()
		close(disconnected)
	}()

	select {
	case <-ctx.Done():
		_ = client.Close()
		<-disconnected
		return false
	case <-disconnected:
		return true
	}
}

func (t *Tunnel) serveSOCKS(connection net.Conn) {
	defer connection.Close()
	if err := negotiate(connection); err != nil {
		return
	}

	address, err := readConnectRequest(connection)
	if err != nil {
		_ = writeReply(connection, socksReplyGeneralFailure)
		return
	}

	remote := t.currentRemote()
	if remote == nil {
		_ = writeReply(connection, socksReplyGeneralFailure)
		return
	}

	remoteConnection, err := remote.DialContext(t.context(), "tcp", address)
	if err != nil {
		_ = writeReply(connection, socksReplyGeneralFailure)
		return
	}
	defer remoteConnection.Close()
	if err := writeReply(connection, socksReplySucceeded); err != nil {
		return
	}

	copyConnections(connection, remoteConnection)
}

func negotiate(connection net.Conn) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(connection, header); err != nil {
		return err
	}
	if header[0] != socksVersion {
		return errors.New("unsupported SOCKS version")
	}

	methods := make([]byte, header[1])
	if _, err := io.ReadFull(connection, methods); err != nil {
		return err
	}
	for _, method := range methods {
		if method == socksNoAuthentication {
			_, err := connection.Write([]byte{socksVersion, socksNoAuthentication})
			return err
		}
	}
	if _, err := connection.Write([]byte{socksVersion, socksNoAcceptableMethods}); err != nil {
		return err
	}
	return errors.New("SOCKS authentication is required")
}

func readConnectRequest(connection net.Conn) (string, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(connection, header); err != nil {
		return "", err
	}
	if header[0] != socksVersion || header[1] != socksConnect || header[2] != 0 {
		return "", errors.New("unsupported SOCKS request")
	}

	host, err := readAddress(connection, header[3])
	if err != nil {
		return "", err
	}
	port := make([]byte, 2)
	if _, err := io.ReadFull(connection, port); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(port[0])<<8|int(port[1]))), nil
}

func readAddress(connection net.Conn, addressType byte) (string, error) {
	switch addressType {
	case socksAddressIPv4:
		address := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(connection, address); err != nil {
			return "", err
		}
		return net.IP(address).String(), nil
	case socksAddressDomain:
		length := make([]byte, 1)
		if _, err := io.ReadFull(connection, length); err != nil {
			return "", err
		}
		address := make([]byte, length[0])
		if _, err := io.ReadFull(connection, address); err != nil {
			return "", err
		}
		return string(address), nil
	case socksAddressIPv6:
		address := make([]byte, net.IPv6len)
		if _, err := io.ReadFull(connection, address); err != nil {
			return "", err
		}
		return net.IP(address).String(), nil
	default:
		return "", errors.New("unsupported SOCKS address type")
	}
}

func writeReply(connection net.Conn, code byte) error {
	_, err := connection.Write([]byte{socksVersion, code, 0, socksAddressIPv4, 0, 0, 0, 0, 0, 0})
	return err
}

func copyConnections(client, remote net.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(remote, client)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, remote)
		done <- struct{}{}
	}()
	<-done
}

func (t *Tunnel) currentRemote() remoteDialer {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.remote
}

func (t *Tunnel) context() context.Context {
	if t.ctx == nil {
		return context.Background()
	}
	return t.ctx
}

func (t *Tunnel) setRemote(remote remoteDialer) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.remote = remote
}

func (t *Tunnel) setHealth(healthy bool) {
	if t.onHealth != nil {
		t.onHealth(healthy)
	}
}

func (c Config) validate() error {
	if c.Host == "" || c.User == "" || c.PrivateKeyPath == "" || c.HostKey == "" {
		return errors.New("incomplete SSH tunnel configuration")
	}
	return nil
}

func (c Config) address() string {
	port := c.Port
	if port == 0 {
		port = 22
	}
	return net.JoinHostPort(c.Host, strconv.Itoa(port))
}

func (c Config) reconnectDelay() time.Duration {
	if c.ReconnectDelay <= 0 {
		return defaultReconnectDelay
	}
	return c.ReconnectDelay
}

func (c Config) dialTimeout() time.Duration {
	if c.DialTimeout <= 0 {
		return defaultSSHDialTimeout
	}
	return c.DialTimeout
}

func (c Config) clientConfig() (*sshLib.ClientConfig, error) {
	privateKey, err := os.ReadFile(c.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read SSH private key: %w", err)
	}
	signer, err := sshLib.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("parse SSH private key: %w", err)
	}
	hostKey, _, _, _, err := sshLib.ParseAuthorizedKey([]byte(c.HostKey))
	if err != nil {
		return nil, fmt.Errorf("parse SSH host key: %w", err)
	}
	return &sshLib.ClientConfig{
		User:            c.User,
		Auth:            []sshLib.AuthMethod{sshLib.PublicKeys(signer)},
		HostKeyCallback: sshLib.FixedHostKey(hostKey),
		Timeout:         c.dialTimeout(),
	}, nil
}

func wait(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func newTunnelForTest(remote remoteDialer) *Tunnel {
	return &Tunnel{ctx: context.Background(), remote: remote}
}
