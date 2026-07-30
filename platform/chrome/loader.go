package chrome

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	operationsApplication "github.com/jcastilloa/goddgs-server/operations/application"
	operations "github.com/jcastilloa/goddgs-server/operations/domain"
	proxyApplication "github.com/jcastilloa/goddgs-server/proxy/application"
	"github.com/jcastilloa/goddgs-server/search/domain"
	extractAIApplication "github.com/jcastilloa/goddgs-server/shared/extractai/application"
)

var ErrNavigation = domain.ErrHTMLLoaderNavigation

type PageResult struct {
	URL  string
	HTML string
}

type PageRunner interface {
	Run(context.Context, Browser, string, string) (PageResult, error)
}

type Loader struct {
	proxies  *proxyApplication.Pool[proxyApplication.Endpoint]
	manager  *Manager
	timeout  time.Duration
	runner   PageRunner
	recorder *operationsApplication.EventRecorder
}

func NewLoader(proxies *proxyApplication.Pool[proxyApplication.Endpoint], manager *Manager, timeout time.Duration, runner PageRunner, recorders ...operationsApplication.EventRecorder) Loader {
	if runner == nil {
		runner = chromedpPageRunner{}
	}
	loader := Loader{proxies: proxies, manager: manager, timeout: timeout, runner: runner}
	if len(recorders) > 0 {
		loader.recorder = &recorders[0]
	}
	return loader
}

func (l Loader) LoadHTML(ctx context.Context, rawURL string) (result domain.ExtractResult, err error) {
	if err := ctx.Err(); err != nil {
		return domain.ExtractResult{}, err
	}
	if l.proxies == nil || l.manager == nil || l.runner == nil {
		return domain.ExtractResult{}, ErrUnavailable
	}
	proxy, err := l.proxies.Select(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return domain.ExtractResult{}, ctx.Err()
		}
		return domain.ExtractResult{}, classifyError(ErrUnavailable, err)
	}
	proxyURL, err := chromeProxyURL(proxy.Value.TransportURL)
	if err != nil {
		return domain.ExtractResult{}, err
	}
	step := l.startStep(ctx, rawURL, proxy.Key)
	defer func() { _ = l.finishStep(ctx, step, err) }()
	pageCtx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()
	lease, err := l.manager.Acquire(pageCtx, proxy.Key, proxyURL)
	if err != nil {
		return domain.ExtractResult{}, err
	}
	defer lease.Release()

	page, err := l.runner.Run(pageCtx, lease.Browser(), proxy.Key, rawURL)
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			return domain.ExtractResult{}, err
		}
		if pageCtx.Err() != nil {
			return domain.ExtractResult{}, pageCtx.Err()
		}
		return domain.ExtractResult{}, classifyError(ErrNavigation, err)
	}
	content, err := extractAIApplication.SanitizeHTML(page.HTML)
	if err != nil {
		return domain.ExtractResult{}, classifyError(ErrNavigation, err)
	}
	if page.URL == "" {
		page.URL = rawURL
	}
	return domain.ExtractResult{URL: page.URL, Content: content}, nil
}

func chromeProxyURL(rawURL string) (string, error) {
	proxyURL := strings.TrimSpace(rawURL)
	if proxyURL == "" {
		return "", nil
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil || parsed.User != nil {
		return "", ErrUnavailable
	}
	if strings.EqualFold(parsed.Scheme, "socks5h") {
		parsed.Scheme = "socks5"
	}
	return parsed.String(), nil
}

func (l Loader) startStep(ctx context.Context, rawURL, proxyKey string) operations.Step {
	if l.recorder == nil {
		return operations.Step{}
	}
	step, _ := l.recorder.StartStep(ctx, operations.StepStart{
		Type:     operations.StepExtractHeuristic,
		Provider: "chrome",
		Proxy:    proxyKey,
		Metadata: map[string]string{"url": rawURL, "format": "html"},
	})
	return step
}

func (l Loader) finishStep(ctx context.Context, step operations.Step, err error) error {
	if l.recorder == nil {
		return nil
	}
	return l.recorder.FinishStep(ctx, step, err)
}
