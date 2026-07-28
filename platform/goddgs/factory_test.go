package goddgs

import (
	"context"
	"testing"
	"time"

	operationsApplication "github.com/jcastilloa/goddgs-server/operations/application"
	configDomain "github.com/jcastilloa/goddgs-server/shared/config/domain"
)

func TestGatewayBuilderCreatesClientForEveryDirectProxy(t *testing.T) {
	createdProxies := []string{}
	builder := newTestBuilder(&createdProxies)
	config := configDomain.ServerConfig{
		RequestTimeout: 12 * time.Second,
		Proxies: []configDomain.ProxyConfig{
			{Name: "eu", Type: "direct", URL: "socks5h://127.0.0.1:9050"},
			{Name: "us", Type: "direct", URL: "https://proxy.example.com:8443"},
		},
	}

	managed, err := builder.Build(context.Background(), config)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	defer managed.Close()

	if got, want := createdProxies, []string{"socks5h://127.0.0.1:9050", "https://proxy.example.com:8443"}; !equalStrings(got, want) {
		t.Errorf("created proxies = %#v, want %#v", got, want)
	}
}

func TestGatewayBuilderCreatesStableClientForSSHTunnelAndTracksHealth(t *testing.T) {
	createdProxies := []string{}
	builder := newTestBuilder(&createdProxies)
	tunnel := &fakeTunnel{proxyURL: "socks5h://127.0.0.1:38123"}
	builder.startTunnel = func(_ context.Context, config sshTunnelConfig, report func(bool)) (tunnelHandle, error) {
		if config.Host != "proxy.example.com" || config.Port != 2222 || config.User != "deploy" {
			t.Errorf("tunnel config = %#v", config)
		}
		tunnel.report = report
		return tunnel, nil
	}
	config := configDomain.ServerConfig{
		RequestTimeout: 5 * time.Second,
		Proxies: []configDomain.ProxyConfig{{
			Name: "tunnel", Type: "ssh", Host: "proxy.example.com", Port: 2222, User: "deploy", PrivateKeyPath: "/key", HostKey: "ssh-ed25519 key",
		}},
	}

	managed, err := builder.Build(context.Background(), config)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got, want := createdProxies, []string{tunnel.proxyURL}; !equalStrings(got, want) {
		t.Errorf("created proxies = %#v, want %#v", got, want)
	}
	if _, err := managed.Gateway.clients.Select(context.Background()); err == nil {
		t.Error("Select() before healthy report error = nil, want error")
	}

	tunnel.report(false)
	_, err = managed.Gateway.clients.Select(context.Background())
	if err == nil {
		t.Error("Select() after unhealthy report error = nil, want error")
	}
	tunnel.report(true)
	lease, err := managed.Gateway.clients.Select(context.Background())
	if err != nil {
		t.Fatalf("Select() after healthy report error = %v", err)
	}
	if lease.Key != "tunnel" {
		t.Errorf("Select() key = %q, want tunnel", lease.Key)
	}

	if err := managed.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
	if !tunnel.closed {
		t.Error("tunnel was not closed")
	}
}

func TestGatewayBuilderKeepsEffectiveProbeTransportAndForwardsTunnelSignals(t *testing.T) {
	createdProxies := []string{}
	builder := newTestBuilder(&createdProxies)
	tunnel := &fakeTunnel{proxyURL: "socks5h://127.0.0.1:38123"}
	builder.startTunnel = func(_ context.Context, _ sshTunnelConfig, report func(bool)) (tunnelHandle, error) {
		tunnel.report = report
		return tunnel, nil
	}
	managed, err := builder.Build(context.Background(), configDomain.ServerConfig{
		Proxies: []configDomain.ProxyConfig{
			{Name: "direct", Type: "direct", URL: "http://proxy.example.com:8080"},
			{Name: "tor-browser", Type: "direct", URL: "tb"},
			{Name: "tunnel", Type: "ssh", Host: "proxy.example.com", User: "deploy", PrivateKeyPath: "/key", HostKey: "ssh-ed25519 key"},
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	defer managed.Close()

	if got, want := managed.probeTargets(), []operationsApplication.ProbeTarget{
		{Name: "direct", TransportURL: "http://proxy.example.com:8080"},
		{Name: "tor-browser", TransportURL: "socks5h://127.0.0.1:9150"},
		{Name: "tunnel", TransportURL: tunnel.proxyURL, Tunnel: true},
	}; !equalProbeTargets(got, want) {
		t.Errorf("probe targets = %#v, want %#v", got, want)
	}

	var reports []bool
	managed.setTunnelHealthReporter(func(name string, connected bool, _ uint64) {
		if name != "tunnel" {
			t.Errorf("tunnel name = %q, want tunnel", name)
		}
		reports = append(reports, connected)
	})
	tunnel.report(true)
	if got, want := reports, []bool{false, true}; !equalBools(got, want) {
		t.Errorf("tunnel reports = %#v, want %#v", got, want)
	}
}

type fakeTunnel struct {
	proxyURL string
	report   func(bool)
	closed   bool
}

func (t *fakeTunnel) ProxyURL() string {
	return t.proxyURL
}

func (t *fakeTunnel) Close() error {
	t.closed = true
	return nil
}

func newTestBuilder(createdProxies *[]string) *GatewayBuilder {
	builder := &GatewayBuilder{}
	builder.newClient = func(proxy string, _ time.Duration) Client {
		*createdProxies = append(*createdProxies, proxy)
		return &fakeClient{}
	}
	return builder
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func equalProbeTargets(got, want []operationsApplication.ProbeTarget) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func equalBools(got, want []bool) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
