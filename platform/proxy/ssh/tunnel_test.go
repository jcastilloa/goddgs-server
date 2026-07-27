package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	sshLib "golang.org/x/crypto/ssh"
)

func TestTunnelServesSOCKS5ConnectThroughRemoteDialer(t *testing.T) {
	remote := newRecordingRemote(t)
	tunnel := newTunnelForTest(remote)
	client, server := net.Pipe()
	defer client.Close()

	done := make(chan struct{})
	go func() {
		tunnel.serveSOCKS(server)
		close(done)
	}()

	writeAll(t, client, []byte{5, 1, 0})
	if got := readExactly(t, client, 2); string(got) != string([]byte{5, 0}) {
		t.Fatalf("greeting response = %v, want [5 0]", got)
	}

	writeAll(t, client, append([]byte{5, 1, 0, 3, byte(len("example.com"))}, append([]byte("example.com"), 0, 80)...))
	if got := readExactly(t, client, 10); string(got) != string([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}) {
		t.Fatalf("connect response = %v, want success", got)
	}

	call := <-remote.calls
	if call.network != "tcp" || call.address != "example.com:80" {
		t.Errorf("remote dial = (%q, %q), want (tcp, example.com:80)", call.network, call.address)
	}

	writeAll(t, client, []byte("ping"))
	if got := readExactly(t, remote.peer, 4); string(got) != "ping" {
		t.Errorf("remote peer read = %q, want ping", got)
	}
	writeAll(t, remote.peer, []byte("pong"))
	if got := readExactly(t, client, 4); string(got) != "pong" {
		t.Errorf("client read = %q, want pong", got)
	}

	client.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SOCKS handler did not return")
	}
}

func TestTunnelRejectsSOCKS5WhenNoAuthenticationMethodIsAccepted(t *testing.T) {
	tunnel := newTunnelForTest(newRecordingRemote(t))
	client, server := net.Pipe()
	defer client.Close()
	done := make(chan struct{})
	go func() {
		tunnel.serveSOCKS(server)
		close(done)
	}()

	writeAll(t, client, []byte{5, 1, 2})
	if got := readExactly(t, client, 2); string(got) != string([]byte{5, 255}) {
		t.Errorf("greeting response = %v, want [5 255]", got)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SOCKS handler did not return")
	}
}

func TestConfigRejectsIncompleteTunnel(t *testing.T) {
	tests := []Config{
		{},
		{Host: "proxy.example.com", User: "deploy", PrivateKeyPath: "/key"},
	}
	for _, config := range tests {
		if err := config.validate(); err == nil {
			t.Errorf("validate(%#v) error = nil, want error", config)
		}
	}
}

func TestReconnectDelayBacksOffWithLimit(t *testing.T) {
	if got := nextReconnectDelay(time.Second); got != 2*time.Second {
		t.Errorf("nextReconnectDelay(1s) = %v, want 2s", got)
	}
	if got := nextReconnectDelay(maximumReconnectDelay); got != maximumReconnectDelay {
		t.Errorf("nextReconnectDelay(max) = %v, want %v", got, maximumReconnectDelay)
	}
}

func TestReadAddressSupportsIPAddressesAndRejectsUnknownType(t *testing.T) {
	tests := []struct {
		name        string
		addressType byte
		payload     []byte
		want        string
		wantErr     bool
	}{
		{name: "IPv4", addressType: socksAddressIPv4, payload: []byte{192, 0, 2, 1}, want: "192.0.2.1"},
		{name: "IPv6", addressType: socksAddressIPv6, payload: net.ParseIP("2001:db8::1").To16(), want: "2001:db8::1"},
		{name: "unknown", addressType: 99, wantErr: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			result := make(chan struct {
				address string
				err     error
			}, 1)
			go func() {
				address, err := readAddress(server, testCase.addressType)
				result <- struct {
					address string
					err     error
				}{address, err}
				_ = server.Close()
			}()
			if len(testCase.payload) > 0 {
				writeAll(t, client, testCase.payload)
			}
			got := <-result
			if (got.err != nil) != testCase.wantErr {
				t.Errorf("readAddress() error = %v, wantErr %v", got.err, testCase.wantErr)
			}
			if got.address != testCase.want {
				t.Errorf("readAddress() = %q, want %q", got.address, testCase.want)
			}
		})
	}
}

func TestTunnelConfigurationDefaults(t *testing.T) {
	config := Config{Host: "proxy.example.com"}
	if got := config.address(); got != "proxy.example.com:22" {
		t.Errorf("address() = %q, want proxy.example.com:22", got)
	}
	if got := config.reconnectDelay(); got != defaultReconnectDelay {
		t.Errorf("reconnectDelay() = %v, want %v", got, defaultReconnectDelay)
	}
	if got := config.dialTimeout(); got != defaultSSHDialTimeout {
		t.Errorf("dialTimeout() = %v, want %v", got, defaultSSHDialTimeout)
	}
}

func TestTunnelConnectsToVerifiedSSHDaemonAndForwardsSOCKS(t *testing.T) {
	clientPublicKey, privateKeyPath := writeSSHPrivateKey(t)
	address, hostKey, closeServer := startSSHEchoServer(t, clientPublicKey)
	defer closeServer()

	host, rawPort, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("split SSH server address: %v", err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatalf("parse SSH server port: %v", err)
	}
	health := make(chan bool, 4)
	tunnel, err := Start(context.Background(), Config{
		Host:           host,
		Port:           port,
		User:           "deploy",
		PrivateKeyPath: privateKeyPath,
		HostKey:        hostKey,
	}, func(healthy bool) { health <- healthy })
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer tunnel.Close()

	waitForHealth(t, health, true)
	proxyURL, err := url.Parse(tunnel.ProxyURL())
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}
	connection, err := net.Dial("tcp", proxyURL.Host)
	if err != nil {
		t.Fatalf("dial SOCKS listener: %v", err)
	}
	defer connection.Close()

	writeAll(t, connection, []byte{5, 1, 0})
	if got := readExactly(t, connection, 2); string(got) != string([]byte{5, 0}) {
		t.Fatalf("greeting response = %v, want [5 0]", got)
	}
	writeAll(t, connection, append([]byte{5, 1, 0, socksAddressDomain, byte(len("example.com"))}, append([]byte("example.com"), 0, 80)...))
	if got := readExactly(t, connection, 10); got[1] != socksReplySucceeded {
		t.Fatalf("connect response = %v, want success", got)
	}
	writeAll(t, connection, []byte("ping"))
	if got := string(readExactly(t, connection, 4)); got != "ping" {
		t.Errorf("echo = %q, want ping", got)
	}
}

type recordingRemote struct {
	calls chan dialCall
	peer  net.Conn
}

type dialCall struct {
	network string
	address string
}

func newRecordingRemote(t *testing.T) *recordingRemote {
	t.Helper()
	return &recordingRemote{calls: make(chan dialCall, 1)}
}

func (r *recordingRemote) DialContext(_ context.Context, network, address string) (net.Conn, error) {
	server, peer := net.Pipe()
	r.peer = peer
	r.calls <- dialCall{network: network, address: address}
	return server, nil
}

func writeAll(t *testing.T, conn net.Conn, value []byte) {
	t.Helper()
	if _, err := conn.Write(value); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readExactly(t *testing.T, conn net.Conn, length int) []byte {
	t.Helper()
	value := make([]byte, length)
	if _, err := io.ReadFull(conn, value); err != nil {
		t.Fatalf("read: %v", err)
	}
	return value
}

func writeSSHPrivateKey(t *testing.T) (sshLib.PublicKey, string) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	block, err := sshLib.MarshalPrivateKey(privateKey, "")
	if err != nil {
		t.Fatalf("marshal client key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write client key: %v", err)
	}
	signer, err := sshLib.NewPublicKey(publicKey)
	if err != nil {
		t.Fatalf("create client public key: %v", err)
	}
	return signer, path
}

func startSSHEchoServer(t *testing.T, clientPublicKey sshLib.PublicKey) (string, string, func()) {
	t.Helper()
	_, hostPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	hostSigner, err := sshLib.NewSignerFromKey(hostPrivateKey)
	if err != nil {
		t.Fatalf("create host signer: %v", err)
	}
	configuration := &sshLib.ServerConfig{
		PublicKeyCallback: func(_ sshLib.ConnMetadata, key sshLib.PublicKey) (*sshLib.Permissions, error) {
			if string(key.Marshal()) != string(clientPublicKey.Marshal()) {
				return nil, os.ErrPermission
			}
			return nil, nil
		},
	}
	configuration.AddHostKey(hostSigner)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen SSH server: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go serveSSHConnection(connection, configuration)
		}
	}()
	return listener.Addr().String(), string(sshLib.MarshalAuthorizedKey(hostSigner.PublicKey())), func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("SSH server did not stop")
		}
	}
}

func serveSSHConnection(connection net.Conn, configuration *sshLib.ServerConfig) {
	server, channels, requests, err := sshLib.NewServerConn(connection, configuration)
	if err != nil {
		return
	}
	defer server.Close()
	go sshLib.DiscardRequests(requests)
	for channel := range channels {
		if channel.ChannelType() != "direct-tcpip" {
			_ = channel.Reject(sshLib.UnknownChannelType, "unsupported channel")
			continue
		}
		accepted, requests, err := channel.Accept()
		if err != nil {
			continue
		}
		go sshLib.DiscardRequests(requests)
		go func() {
			defer accepted.Close()
			_, _ = io.Copy(accepted, accepted)
		}()
	}
}

func waitForHealth(t *testing.T, health <-chan bool, expected bool) {
	t.Helper()
	timeout := time.NewTimer(time.Second)
	defer timeout.Stop()
	for {
		select {
		case actual := <-health:
			if actual == expected {
				return
			}
		case <-timeout.C:
			t.Fatalf("health report %v was not received", expected)
		}
	}
}
