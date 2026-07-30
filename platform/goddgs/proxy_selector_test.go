package goddgs

import (
	"context"
	"testing"

	"github.com/jcastilloa/goddgs-server/search/domain"
	configDomain "github.com/jcastilloa/goddgs-server/shared/config/domain"
)

func TestManagedGatewaySharesEffectiveProxyRotation(t *testing.T) {
	createdProxies := []string{}
	builder := newTestBuilder(&createdProxies)
	tunnel := &fakeTunnel{proxyURL: "socks5h://127.0.0.1:38123"}
	builder.startTunnel = func(_ context.Context, _ sshTunnelConfig, report func(bool)) (tunnelHandle, error) {
		tunnel.report = report
		return tunnel, nil
	}

	managed, err := builder.Build(context.Background(), configDomain.ServerConfig{Proxies: []configDomain.ProxyConfig{
		{Name: "direct", Type: "direct", URL: "http://proxy.example.com:8080"},
		{Name: "tor", Type: "direct", URL: "tb"},
		{Name: "tunnel", Type: "ssh", Host: "proxy.example.com", User: "deploy", PrivateKeyPath: "/key", HostKey: "ssh-ed25519 key"},
	}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	defer managed.Close()
	tunnel.report(true)

	first, err := managed.ProxySelector().Select(context.Background())
	if err != nil {
		t.Fatalf("first proxy selection error = %v", err)
	}
	if first.Key != "direct" || first.Value.TransportURL != "http://proxy.example.com:8080" {
		t.Errorf("first endpoint = %#v", first)
	}
	if _, err := managed.Gateway.Search(context.Background(), domain.SearchRequest{Category: domain.CategoryText, Query: "query"}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	third, err := managed.ProxySelector().Select(context.Background())
	if err != nil {
		t.Fatalf("third proxy selection error = %v", err)
	}
	if third.Key != "tunnel" || third.Value.TransportURL != tunnel.proxyURL {
		t.Errorf("third endpoint = %#v", third)
	}

	managed.Gateway.MarkUnhealthy("tunnel")
	for range 3 {
		lease, selectErr := managed.ProxySelector().Select(context.Background())
		if selectErr != nil {
			t.Fatalf("Select() after unhealthy error = %v", selectErr)
		}
		if lease.Key == "tunnel" {
			t.Error("unhealthy tunnel selected")
		}
	}
}
