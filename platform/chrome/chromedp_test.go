package chrome

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestChromedpFactoryUsesConfiguredExecutableAndProxy(t *testing.T) {
	configuredExecutable := "/configured/chromium"
	proxyURL := "socks5h://127.0.0.1:9050"
	var gotExecutable, gotProxyURL string

	factory := chromedpFactory(configuredExecutable, func(_ context.Context, executable, selectedProxyURL string) (Browser, error) {
		gotExecutable = executable
		gotProxyURL = selectedProxyURL
		return &recordingBrowser{}, nil
	})
	browser, err := factory(context.Background(), proxyURL)
	if err != nil {
		t.Fatalf("factory error = %v", err)
	}
	if browser == nil {
		t.Fatal("factory browser = nil")
	}
	if gotExecutable != configuredExecutable || gotProxyURL != proxyURL {
		t.Errorf("Chrome launch = executable %q proxy %q, want executable %q proxy %q", gotExecutable, gotProxyURL, configuredExecutable, proxyURL)
	}
}

func TestChromedpFactoryDoesNotExposeLaunchErrors(t *testing.T) {
	factory := chromedpFactory("/configured/chromium", func(context.Context, string, string) (Browser, error) {
		return nil, errors.New("/configured/chromium --proxy-server=socks5://user:secret@127.0.0.1:9050")
	})
	_, err := factory(context.Background(), "socks5://user:secret@127.0.0.1:9050")
	if err == nil {
		t.Fatal("factory error = nil")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("factory exposed proxy credential: %v", err)
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("factory error = %v, want ErrUnavailable", err)
	}
}

func TestFindChromeExecutableUsesPATH(t *testing.T) {
	executable, found := findChromeExecutable()
	if !found {
		t.Skip("Chrome or Chromium is not available on PATH")
	}
	if executable == "" {
		t.Error("findChromeExecutable() returned an empty path")
	}
}
