package main

import (
	"context"
	"testing"
	"time"

	operationsApplication "github.com/jcastilloa/goddgs-server/operations/application"
	chromePlatform "github.com/jcastilloa/goddgs-server/platform/chrome"
	goddgsPlatform "github.com/jcastilloa/goddgs-server/platform/goddgs"
	configDomain "github.com/jcastilloa/goddgs-server/shared/config/domain"
)

func TestHTMLLoaderIsDisabledUnlessChromeIsEnabled(t *testing.T) {
	gateway, err := goddgsPlatform.NewGatewayBuilder().Build(context.Background(), configDomain.ServerConfig{Proxies: []configDomain.ProxyConfig{{Name: "direct", Type: "direct"}}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	defer gateway.Close()

	executable := chromePlatform.NewExecutableLocator(context.Background(), "/configured/chrome", nil)
	loader, closeLoader := htmlLoader(configDomain.ServerConfig{}, gateway, operationsApplication.EventRecorder{}, executable)
	defer closeChromeManager(closeLoader)
	if loader != nil {
		t.Errorf("disabled loader = %#v, want nil", loader)
	}
	loader, closeLoader = htmlLoader(configDomain.ServerConfig{Chrome: configDomain.ChromeConfig{Enabled: true, Timeout: time.Second, MaxBrowsers: 1, MaxPagesPerBrowser: 1, IdleTimeout: time.Minute}}, gateway, operationsApplication.EventRecorder{}, executable)
	defer closeChromeManager(closeLoader)
	if loader == nil {
		t.Error("enabled loader = nil")
	}
}

func TestChromeManagerClosesBeforeGateway(t *testing.T) {
	order := []string{}
	manager := closeRecorder{onClose: func() { order = append(order, "chrome") }}
	gateway := closeRecorder{onClose: func() { order = append(order, "gateway") }}

	func() {
		defer closeGateway(gateway)
		defer closeChromeManager(manager)
	}()

	if want := []string{"chrome", "gateway"}; !equalCloseOrder(order, want) {
		t.Errorf("close order = %#v, want %#v", order, want)
	}
}

type closeRecorder struct {
	onClose func()
}

func (r closeRecorder) Close() error {
	if r.onClose != nil {
		r.onClose()
	}
	return nil
}

func equalCloseOrder(got, want []string) bool {
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
