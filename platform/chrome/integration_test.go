package chrome

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"

	proxyApplication "github.com/jcastilloa/goddgs-server/proxy/application"
)

func TestLoaderCapturesJavaScriptRenderedDOM(t *testing.T) {
	executable, err := exec.LookPath("google-chrome")
	if err != nil {
		t.Skip("compatible Chrome executable is not available")
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`<!doctype html><html><body><main id="content">Loading</main><script>document.getElementById('content').innerHTML = '<article><h1>Rendered</h1><p>Body</p></article>'</script></body></html>`))
	}))
	defer server.Close()

	selector, err := proxyApplication.NewPool([]proxyApplication.Entry[proxyApplication.Endpoint]{{Key: "direct"}})
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	manager := NewManager(ManagerConfig{
		MaxBrowsers:        1,
		MaxPagesPerBrowser: 1,
		IdleTimeout:        time.Minute,
		Factory:            NewChromedpFactory(executable),
	})
	loader := NewLoader(selector, manager, 10*time.Second, nil)
	result, err := loader.LoadHTML(context.Background(), server.URL)
	if err != nil {
		_ = manager.Close()
		t.Fatalf("LoadHTML() error = %v", err)
	}
	if result.Content != "<main><article><h1>Rendered</h1><p>Body</p></article></main>" {
		_ = manager.Close()
		t.Errorf("rendered HTML = %q", result.Content)
	}
	if err := manager.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestLoaderUsesIsolatedBrowserContexts(t *testing.T) {
	executable, err := exec.LookPath("google-chrome")
	if err != nil {
		t.Skip("compatible Chrome executable is not available")
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/set" {
			_, _ = writer.Write([]byte(`<!doctype html><html><body><script>localStorage.setItem('request-state', 'private')</script><main>set</main></body></html>`))
			return
		}
		_, _ = writer.Write([]byte(`<!doctype html><html><body><main id="content"></main><script>document.getElementById('content').textContent = localStorage.getItem('request-state') || 'empty'</script></body></html>`))
	}))
	defer server.Close()

	selector, err := proxyApplication.NewPool([]proxyApplication.Entry[proxyApplication.Endpoint]{{Key: "direct"}})
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	manager := NewManager(ManagerConfig{
		MaxBrowsers:        1,
		MaxPagesPerBrowser: 2,
		IdleTimeout:        time.Minute,
		Factory:            NewChromedpFactory(executable),
	})
	defer manager.Close()
	loader := NewLoader(selector, manager, 10*time.Second, nil)
	if _, err := loader.LoadHTML(context.Background(), server.URL+"/set"); err != nil {
		t.Fatalf("LoadHTML(set) error = %v", err)
	}
	result, err := loader.LoadHTML(context.Background(), server.URL+"/read")
	if err != nil {
		t.Fatalf("LoadHTML(read) error = %v", err)
	}
	if result.Content != "<main>empty</main>" {
		t.Errorf("isolated HTML = %q, want empty local storage", result.Content)
	}
}
