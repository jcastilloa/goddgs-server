package goddgs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	operationsApplication "github.com/jcastilloa/goddgs-server/operations/application"
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
	Gateway            *Gateway
	proxies            *proxyApplication.Pool[proxyApplication.Endpoint]
	tunnels            []tunnelHandle
	targets            []operationsApplication.ProbeTarget
	reportTunnelHealth func(string, bool, uint64)

	mu     sync.Mutex
	health map[string]tunnelHealth
}

type tunnelHealth struct {
	connected bool
	version   uint64
}

const torBrowserProxyURL = "socks5h://127.0.0.1:9150"

type HealthProbeConfig struct {
	Interval         time.Duration
	URL              string
	SuccessThreshold int
	FailureThreshold int
	Store            operationsApplication.ProbeStore
	Prober           operationsApplication.ProbeClient
	Now              func() time.Time
	ReportError      func(error)
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

func (b GatewayBuilder) Build(ctx context.Context, config configDomain.ServerConfig, recorders ...operationsApplication.EventRecorder) (*ManagedGateway, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if b.newClient == nil {
		return nil, errors.New("gateway builder is incomplete")
	}

	endpoints := make([]proxyApplication.Entry[proxyApplication.Endpoint], 0, len(config.Proxies))
	clients := make(map[string]Client, len(config.Proxies))
	managed := &ManagedGateway{health: make(map[string]tunnelHealth)}
	for _, proxy := range config.Proxies {
		endpoint, client, target, tunnel, err := b.entry(ctx, proxy, config.RequestTimeout, managed)
		if err != nil {
			_ = managed.Close()
			return nil, err
		}
		endpoints = append(endpoints, endpoint)
		clients[endpoint.Key] = client
		managed.targets = append(managed.targets, target)
		if tunnel != nil {
			managed.tunnels = append(managed.tunnels, tunnel)
		}
	}

	selector, err := proxyApplication.NewPool(endpoints)
	if err != nil {
		_ = managed.Close()
		return nil, err
	}
	gateway, err := NewGatewayWithProxySelector(selector, clients, config.MaxProxyRetries, recorders...)
	if err != nil {
		_ = managed.Close()
		return nil, err
	}
	managed.proxies = selector
	managed.setGateway(gateway)
	return managed, nil
}

func (b GatewayBuilder) entry(ctx context.Context, proxy configDomain.ProxyConfig, timeout time.Duration, managed *ManagedGateway) (proxyApplication.Entry[proxyApplication.Endpoint], Client, operationsApplication.ProbeTarget, tunnelHandle, error) {
	switch strings.ToLower(proxy.Type) {
	case "direct":
		transportURL := effectiveTransportURL(proxy.URL)
		return proxyApplication.Entry[proxyApplication.Endpoint]{Key: proxy.Name, Value: proxyApplication.Endpoint{TransportURL: transportURL}}, b.newClient(transportURL, timeout), operationsApplication.ProbeTarget{Name: proxy.Name, TransportURL: transportURL}, nil, nil
	case "ssh":
		if b.startTunnel == nil {
			return proxyApplication.Entry[proxyApplication.Endpoint]{}, nil, operationsApplication.ProbeTarget{}, nil, errors.New("SSH tunnel factory is unavailable")
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
			return proxyApplication.Entry[proxyApplication.Endpoint]{}, nil, operationsApplication.ProbeTarget{}, nil, fmt.Errorf("start SSH tunnel %q: %w", proxy.Name, err)
		}
		transportURL := tunnel.ProxyURL()
		return proxyApplication.Entry[proxyApplication.Endpoint]{Key: proxy.Name, Value: proxyApplication.Endpoint{TransportURL: transportURL}}, b.newClient(transportURL, timeout), operationsApplication.ProbeTarget{Name: proxy.Name, TransportURL: transportURL, Tunnel: true}, tunnel, nil
	default:
		return proxyApplication.Entry[proxyApplication.Endpoint]{}, nil, operationsApplication.ProbeTarget{}, nil, fmt.Errorf("unsupported proxy type %q", proxy.Type)
	}
}

func (g *ManagedGateway) ProxySelector() *proxyApplication.Pool[proxyApplication.Endpoint] {
	return g.proxies
}

func effectiveTransportURL(configuredURL string) string {
	if strings.EqualFold(strings.TrimSpace(configuredURL), "tb") {
		return torBrowserProxyURL
	}
	return configuredURL
}

func (g *ManagedGateway) setGateway(gateway *Gateway) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.Gateway = gateway
	for key, health := range g.health {
		g.setGatewayHealth(key, health.connected)
	}
}

func (g *ManagedGateway) reportHealth(key string, healthy bool) {
	g.mu.Lock()
	health := g.health[key]
	health.connected = healthy
	health.version++
	g.health[key] = health
	reporter := g.reportTunnelHealth
	if reporter == nil {
		g.setGatewayHealth(key, healthy)
	}
	g.mu.Unlock()

	if reporter != nil {
		reporter(key, healthy, health.version)
	}
}

func (g *ManagedGateway) StartHealthProbes(ctx context.Context, config HealthProbeConfig) *operationsApplication.HealthSupervisor {
	targets := g.probeTargets()
	monitor := operationsApplication.NewHealthMonitor(targets, operationsApplication.HealthMonitorConfig{
		SuccessThreshold: config.SuccessThreshold,
		FailureThreshold: config.FailureThreshold,
	}, g.Gateway, config.Store, config.Now)
	g.setTunnelHealthReporter(func(name string, connected bool, version uint64) {
		if err := monitor.UpdateTunnelConnection(ctx, name, connected, version); err != nil && config.ReportError != nil {
			config.ReportError(err)
		}
	})
	return operationsApplication.StartHealthSupervisor(ctx, operationsApplication.HealthSupervisorConfig{
		Interval:    config.Interval,
		ProbeURL:    config.URL,
		Targets:     targets,
		Prober:      config.Prober,
		Monitor:     monitor,
		ReportError: config.ReportError,
	})
}

func (g *ManagedGateway) probeTargets() []operationsApplication.ProbeTarget {
	g.mu.Lock()
	defer g.mu.Unlock()
	targets := make([]operationsApplication.ProbeTarget, len(g.targets))
	copy(targets, g.targets)
	return targets
}

func (g *ManagedGateway) setTunnelHealthReporter(report func(string, bool, uint64)) {
	g.mu.Lock()
	g.reportTunnelHealth = report
	reports := make(map[string]tunnelHealth, len(g.health))
	for key, health := range g.health {
		reports[key] = health
	}
	g.mu.Unlock()

	if report == nil {
		return
	}
	for key, health := range reports {
		report(key, health.connected, health.version)
	}
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
