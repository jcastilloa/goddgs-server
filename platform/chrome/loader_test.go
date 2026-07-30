package chrome

import (
	"context"
	"errors"
	"testing"
	"time"

	operationsApplication "github.com/jcastilloa/goddgs-server/operations/application"
	operations "github.com/jcastilloa/goddgs-server/operations/domain"
	proxyApplication "github.com/jcastilloa/goddgs-server/proxy/application"
	"github.com/jcastilloa/goddgs-server/search/domain"
)

func TestLoaderSelectsSharedProxyAndReturnsSanitizedRenderedHTML(t *testing.T) {
	selector := newSelector(t, proxyApplication.Entry[proxyApplication.Endpoint]{Key: "tunnel", Value: proxyApplication.Endpoint{TransportURL: "socks5h://127.0.0.1:38123"}})
	runner := &recordingPageRunner{result: PageResult{
		URL:  "https://example.com/final",
		HTML: `<html><body><article class="story"><h1>Title</h1><script>alert(1)</script><p onclick="x()">Body</p></article></body></html>`,
	}}
	factory := &recordingBrowserFactory{}
	manager := NewManager(ManagerConfig{MaxBrowsers: 1, MaxPagesPerBrowser: 1, IdleTimeout: time.Minute, Factory: factory.New})
	t.Cleanup(func() { _ = manager.Close() })
	loader := NewLoader(selector, manager, 10*time.Second, runner)

	got, err := loader.LoadHTML(context.Background(), "https://example.com/article")
	if err != nil {
		t.Fatalf("LoadHTML() error = %v", err)
	}
	if got.URL != "https://example.com/final" || got.Content != "<article><h1>Title</h1><p>Body</p></article>" {
		t.Errorf("LoadHTML() = %#v", got)
	}
	if runner.proxyKey != "tunnel" || runner.url != "https://example.com/article" {
		t.Errorf("runner = %#v", runner)
	}
	if factory.Count() != 1 {
		t.Errorf("browser creations = %d, want 1", factory.Count())
	}
}

func TestLoaderClassifiesUnavailableTimeoutAndNavigationFailures(t *testing.T) {
	selector := newSelector(t, proxyApplication.Entry[proxyApplication.Endpoint]{Key: "direct", Value: proxyApplication.Endpoint{}})
	manager := NewManager(ManagerConfig{MaxBrowsers: 1, MaxPagesPerBrowser: 1, IdleTimeout: time.Minute, Factory: (&recordingBrowserFactory{}).New})
	t.Cleanup(func() { _ = manager.Close() })

	tests := []struct {
		name    string
		runner  PageRunner
		context func() (context.Context, context.CancelFunc)
		wantErr error
	}{
		{
			name:   "navigation failure",
			runner: &recordingPageRunner{err: errors.New("navigation failed")},
			context: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
			wantErr: ErrNavigation,
		},
		{
			name:   "page timeout",
			runner: &recordingPageRunner{waitForContext: true},
			context: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), time.Millisecond)
			},
			wantErr: context.DeadlineExceeded,
		},
		{
			name:   "request canceled",
			runner: &recordingPageRunner{waitForContext: true},
			context: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, func() {}
			},
			wantErr: context.Canceled,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := testCase.context()
			defer cancel()
			_, err := NewLoader(selector, manager, time.Millisecond, testCase.runner).LoadHTML(ctx, "https://example.com")
			if !errors.Is(err, testCase.wantErr) {
				t.Errorf("LoadHTML() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestLoaderPreservesNavigationCauseWithoutExposingIt(t *testing.T) {
	selector := newSelector(t, proxyApplication.Entry[proxyApplication.Endpoint]{Key: "direct"})
	manager := NewManager(ManagerConfig{MaxBrowsers: 1, MaxPagesPerBrowser: 1, IdleTimeout: time.Minute, Factory: (&recordingBrowserFactory{}).New})
	t.Cleanup(func() { _ = manager.Close() })
	cause := errors.New("chrome stderr proxy-password=secret")

	_, err := NewLoader(selector, manager, time.Second, &recordingPageRunner{err: cause}).LoadHTML(context.Background(), "https://example.com")
	if !errors.Is(err, ErrNavigation) || !errors.Is(err, cause) {
		t.Fatalf("LoadHTML() error = %v, want navigation classification and cause", err)
	}
	if err.Error() != ErrNavigation.Error() {
		t.Errorf("LoadHTML() exposed navigation detail: %v", err)
	}
}

func TestLoaderReportsUnavailableSelectorAndBrowser(t *testing.T) {
	selector := newSelector(t, proxyApplication.Entry[proxyApplication.Endpoint]{Key: "direct", Value: proxyApplication.Endpoint{}})
	selector.MarkUnhealthy("direct")
	manager := NewManager(ManagerConfig{MaxBrowsers: 1, MaxPagesPerBrowser: 1, IdleTimeout: time.Minute, Factory: (&recordingBrowserFactory{}).New})
	t.Cleanup(func() { _ = manager.Close() })
	_, err := NewLoader(selector, manager, time.Second, &recordingPageRunner{}).LoadHTML(context.Background(), "https://example.com")
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("LoadHTML() error = %v, want ErrUnavailable", err)
	}
}

func TestLoaderPreservesCanceledContextWhenNoProxyIsHealthy(t *testing.T) {
	selector := newSelector(t, proxyApplication.Entry[proxyApplication.Endpoint]{Key: "direct"})
	selector.MarkUnhealthy("direct")
	manager := NewManager(ManagerConfig{MaxBrowsers: 1, MaxPagesPerBrowser: 1, IdleTimeout: time.Minute, Factory: (&recordingBrowserFactory{}).New})
	t.Cleanup(func() { _ = manager.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewLoader(selector, manager, time.Second, &recordingPageRunner{}).LoadHTML(ctx, "https://example.com")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("LoadHTML() error = %v, want context.Canceled", err)
	}
}

func TestLoaderRejectsAuthenticatedProxyBeforeLaunchingChrome(t *testing.T) {
	selector := newSelector(t, proxyApplication.Entry[proxyApplication.Endpoint]{Key: "direct", Value: proxyApplication.Endpoint{TransportURL: "socks5://user:secret@127.0.0.1:9050"}})
	factory := &recordingBrowserFactory{}
	manager := NewManager(ManagerConfig{MaxBrowsers: 1, MaxPagesPerBrowser: 1, IdleTimeout: time.Minute, Factory: factory.New})
	t.Cleanup(func() { _ = manager.Close() })

	_, err := NewLoader(selector, manager, time.Second, &recordingPageRunner{}).LoadHTML(context.Background(), "https://example.com")
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("LoadHTML() error = %v, want ErrUnavailable", err)
	}
	if factory.Count() != 0 {
		t.Errorf("browser creations = %d, want 0", factory.Count())
	}
}

func TestChromeProxyURLNormalizesSocks5hForChrome(t *testing.T) {
	proxyURL, err := chromeProxyURL("socks5h://127.0.0.1:9150")
	if err != nil {
		t.Fatalf("chromeProxyURL() error = %v", err)
	}
	if proxyURL != "socks5://127.0.0.1:9150" {
		t.Errorf("chromeProxyURL() = %q, want socks5 Chrome form", proxyURL)
	}
}

func TestLoaderRecordsOnlyChromeProviderAndProxyKey(t *testing.T) {
	selector := newSelector(t, proxyApplication.Entry[proxyApplication.Endpoint]{Key: "direct", Value: proxyApplication.Endpoint{TransportURL: "socks5h://127.0.0.1:9050"}})
	manager := NewManager(ManagerConfig{MaxBrowsers: 1, MaxPagesPerBrowser: 1, IdleTimeout: time.Minute, Factory: (&recordingBrowserFactory{}).New})
	t.Cleanup(func() { _ = manager.Close() })
	repository := &stepRepository{}
	recorder := operationsApplication.NewEventRecorder(repository, time.Now, func() string { return "step-1" })
	ctx, err := recorder.StartOperation(context.Background(), operations.OperationStart{Type: operations.OperationExtract})
	if err != nil {
		t.Fatalf("StartOperation() error = %v", err)
	}
	loader := NewLoader(selector, manager, time.Second, &recordingPageRunner{result: PageResult{URL: "https://example.com/final", HTML: "<p>Rendered</p>"}}, recorder)
	if _, err := loader.LoadHTML(ctx, "https://example.com/article?token=secret"); err != nil {
		t.Fatalf("LoadHTML() error = %v", err)
	}
	if len(repository.steps) != 1 {
		t.Fatalf("steps = %#v", repository.steps)
	}
	step := repository.steps[0]
	if step.Provider != "chrome" || step.Proxy != "direct" || step.Metadata["format"] != "html" {
		t.Errorf("step = %#v", step)
	}
	if step.Metadata["url"] != "https://example.com/article" {
		t.Errorf("sanitized step URL = %q", step.Metadata["url"])
	}
	if step.Metadata["proxy"] != "" || step.Metadata["html"] != "" || step.Metadata["dom"] != "" {
		t.Errorf("unsafe step metadata = %#v", step.Metadata)
	}
}

func TestLoaderRecordsSanitizedNavigationFailure(t *testing.T) {
	selector := newSelector(t, proxyApplication.Entry[proxyApplication.Endpoint]{Key: "direct"})
	manager := NewManager(ManagerConfig{MaxBrowsers: 1, MaxPagesPerBrowser: 1, IdleTimeout: time.Minute, Factory: (&recordingBrowserFactory{}).New})
	t.Cleanup(func() { _ = manager.Close() })
	repository := &stepRepository{}
	recorder := operationsApplication.NewEventRecorder(repository, time.Now, func() string { return "step-1" })
	ctx, err := recorder.StartOperation(context.Background(), operations.OperationStart{Type: operations.OperationExtract})
	if err != nil {
		t.Fatalf("StartOperation() error = %v", err)
	}
	loader := NewLoader(selector, manager, time.Second, &recordingPageRunner{err: errors.New("chrome stderr secret=sensitive")}, recorder)
	_, err = loader.LoadHTML(ctx, "https://example.com")
	if !errors.Is(err, ErrNavigation) {
		t.Fatalf("LoadHTML() error = %v, want ErrNavigation", err)
	}
	if len(repository.errors) != 1 {
		t.Fatalf("recorded errors = %#v", repository.errors)
	}
	if repository.errors[0].Message != ErrNavigation.Error() || repository.errors[0].Category != operations.ErrorUnknown {
		t.Errorf("recorded error = %#v", repository.errors[0])
	}
}

func newSelector(t *testing.T, entries ...proxyApplication.Entry[proxyApplication.Endpoint]) *proxyApplication.Pool[proxyApplication.Endpoint] {
	t.Helper()
	selector, err := proxyApplication.NewPool(entries)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	return selector
}

type recordingPageRunner struct {
	result         PageResult
	err            error
	waitForContext bool
	proxyKey       string
	url            string
}

func (r *recordingPageRunner) Run(ctx context.Context, _ Browser, proxyKey, rawURL string) (PageResult, error) {
	r.proxyKey = proxyKey
	r.url = rawURL
	if r.waitForContext {
		<-ctx.Done()
		return PageResult{}, ctx.Err()
	}
	return r.result, r.err
}

var _ = domain.ExtractResult{}

type stepRepository struct {
	steps  []operations.Step
	errors []operations.OperationError
}

func (r *stepRepository) CreateOperation(context.Context, operations.Operation) error { return nil }
func (r *stepRepository) FinishOperation(context.Context, operations.Operation) error { return nil }
func (r *stepRepository) AddStep(_ context.Context, step operations.Step) error {
	r.steps = append(r.steps, step)
	return nil
}
func (r *stepRepository) FinishStep(_ context.Context, step operations.Step) error {
	for index := range r.steps {
		if r.steps[index].ID == step.ID {
			r.steps[index] = step
		}
	}
	return nil
}
func (r *stepRepository) AddError(_ context.Context, operationError operations.OperationError) error {
	r.errors = append(r.errors, operationError)
	return nil
}
func (r *stepRepository) RecordProbe(context.Context, operations.ProxyProbe) error { return nil }
func (r *stepRepository) RecordHealthTransition(context.Context, operations.ProxyHealthTransition) error {
	return nil
}
func (r *stepRepository) ListOperations(context.Context, operations.OperationQuery) ([]operations.Operation, error) {
	return nil, nil
}
func (r *stepRepository) GetOperation(context.Context, string) (operations.OperationDetail, error) {
	return operations.OperationDetail{}, nil
}
