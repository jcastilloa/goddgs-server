package goddgs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	sshTunnel "github.com/jcastilloa/goddgs-server/platform/proxy/ssh"
	proxyApplication "github.com/jcastilloa/goddgs-server/proxy/application"
	configDomain "github.com/jcastilloa/goddgs-server/shared/config/domain"
)

type tunnelHandle interface {
	ProxyURL() string
	Close() error
}

type sshTunnelConfig struct {
	Host           string
	Port           int
	User           string
	PrivateKeyPath string
	HostKey        string
}

type GatewayBuilder struct {
	newClient   func(string, time.Duration) Client
	startTunnel func(context.Context, sshTunnelConfig, func(bool)) (tunnelHandle, error)
}

type ManagedGateway struct {
	Gateway *Gateway
	tunnels []tunnelHandle

	mu     sync.Mutex
	health map[string]bool
}

func NewGatewayBuilder() GatewayBuilder {
	return GatewayBuilder{
		newClient: NewClient,
		startTunnel: func(ctx context.Context, config sshTunnelConfig, report func(bool)) (tunnelHandle, error) {
			return sshTunnel.Start(ctx, sshTunnel.Config{
				Host:           config.Host,
				Port:           config.Port,
				User:           config.User,
				PrivateKeyPath: config.PrivateKeyPath,
				HostKey:        config.HostKey,
			}, report)
		},
	}
}

func (b GatewayBuilder) Build(ctx context.Context, config configDomain.ServerConfig) (*ManagedGateway, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if b.newClient == nil {
		return nil, errors.New("gateway builder is incomplete")
	}

	entries := make([]proxyApplication.Entry[Client], 0, len(config.Proxies))
	managed := &ManagedGateway{health: make(map[string]bool)}
	for _, proxy := range config.Proxies {
		entry, tunnel, err := b.entry(ctx, proxy, config.RequestTimeout, managed)
		if err != nil {
			_ = managed.Close()
			return nil, err
		}
		entries = append(entries, entry)
		if tunnel != nil {
			managed.tunnels = append(managed.tunnels, tunnel)
		}
	}

	gateway, err := NewGateway(entries, config.MaxProxyRetries)
	if err != nil {
		_ = managed.Close()
		return nil, err
	}
	managed.setGateway(gateway)
	return managed, nil
}

func (b GatewayBuilder) entry(ctx context.Context, proxy configDomain.ProxyConfig, timeout time.Duration, managed *ManagedGateway) (proxyApplication.Entry[Client], tunnelHandle, error) {
	switch strings.ToLower(proxy.Type) {
	case "direct":
		return proxyApplication.Entry[Client]{Key: proxy.Name, Value: b.newClient(proxy.URL, timeout)}, nil, nil
	case "ssh":
		if b.startTunnel == nil {
			return proxyApplication.Entry[Client]{}, nil, errors.New("SSH tunnel factory is unavailable")
		}
		managed.reportHealth(proxy.Name, false)
		tunnel, err := b.startTunnel(ctx, sshTunnelConfig{
			Host:           proxy.Host,
			Port:           proxy.Port,
			User:           proxy.User,
			PrivateKeyPath: proxy.PrivateKeyPath,
			HostKey:        proxy.HostKey,
		}, func(healthy bool) { managed.reportHealth(proxy.Name, healthy) })
		if err != nil {
			return proxyApplication.Entry[Client]{}, nil, fmt.Errorf("start SSH tunnel %q: %w", proxy.Name, err)
		}
		return proxyApplication.Entry[Client]{Key: proxy.Name, Value: b.newClient(tunnel.ProxyURL(), timeout)}, tunnel, nil
	default:
		return proxyApplication.Entry[Client]{}, nil, fmt.Errorf("unsupported proxy type %q", proxy.Type)
	}
}

func (g *ManagedGateway) setGateway(gateway *Gateway) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.Gateway = gateway
	for key, healthy := range g.health {
		g.setGatewayHealth(key, healthy)
	}
}

func (g *ManagedGateway) reportHealth(key string, healthy bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.health[key] = healthy
	g.setGatewayHealth(key, healthy)
}

func (g *ManagedGateway) setGatewayHealth(key string, healthy bool) {
	if g.Gateway == nil {
		return
	}
	if healthy {
		g.Gateway.MarkHealthy(key)
		return
	}
	g.Gateway.MarkUnhealthy(key)
}

func (g *ManagedGateway) Close() error {
	var closeErr error
	for index := len(g.tunnels) - 1; index >= 0; index-- {
		closeErr = errors.Join(closeErr, g.tunnels[index].Close())
	}
	return closeErr
}
