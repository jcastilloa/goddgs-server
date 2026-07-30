package extractai

import (
	"context"
	"errors"
	"testing"

	searchApplication "github.com/jcastilloa/goddgs-server/search/application"
	searchDomain "github.com/jcastilloa/goddgs-server/search/domain"
	extractAIDomain "github.com/jcastilloa/goddgs-server/shared/extractai/domain"
)

func TestSourceFetchesCleanHTMLThroughTheSearchExtractor(t *testing.T) {
	extractor := &recordingExtractor{result: searchDomain.ExtractResult{URL: "https://example.com/final", Content: "<article>Source</article>"}}
	source := NewSource(extractor)

	got, err := source.Fetch(context.Background(), extractAIDomain.Request{URL: "https://example.com/article"})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got.URL != "https://example.com/final" || got.HTML != "<article>Source</article>" {
		t.Errorf("Fetch() = %#v", got)
	}
	want := searchDomain.ExtractRequest{URL: "https://example.com/article", Format: "html"}
	if len(extractor.requests) != 1 || extractor.requests[0] != want {
		t.Errorf("extractor requests = %#v, want %#v", extractor.requests, []searchDomain.ExtractRequest{want})
	}
}

func TestSourceRejectsUnsupportedSourceContent(t *testing.T) {
	source := NewSource(&recordingExtractor{result: searchDomain.ExtractResult{Content: 42}})

	_, err := source.Fetch(context.Background(), extractAIDomain.Request{URL: "https://example.com/article"})
	if !errors.Is(err, extractAIDomain.ErrInvalidSource) {
		t.Errorf("Fetch() error = %v, want ErrInvalidSource", err)
	}
}

func TestSourceUsesConfiguredHTMLLoaderThroughSearchService(t *testing.T) {
	gateway := &recordingGateway{result: searchDomain.ExtractResult{Content: "gateway"}}
	loader := &recordingHTMLLoader{result: searchDomain.ExtractResult{URL: "https://example.com/final", Content: "<article>Rendered</article>"}}
	source := NewSource(searchApplication.NewService(gateway, loader))

	page, err := source.Fetch(context.Background(), extractAIDomain.Request{URL: "https://example.com/article"})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if page.URL != "https://example.com/final" || page.HTML != "<article>Rendered</article>" {
		t.Errorf("Fetch() = %#v", page)
	}
	if len(loader.urls) != 1 || len(gateway.requests) != 0 {
		t.Errorf("loader URLs = %#v, gateway requests = %#v", loader.urls, gateway.requests)
	}
}

type recordingExtractor struct {
	result   searchDomain.ExtractResult
	err      error
	requests []searchDomain.ExtractRequest
}

type recordingGateway struct {
	result   searchDomain.ExtractResult
	requests []searchDomain.ExtractRequest
}

func (g *recordingGateway) Search(context.Context, searchDomain.SearchRequest) ([]searchDomain.RawResult, error) {
	return nil, nil
}

func (g *recordingGateway) Extract(_ context.Context, request searchDomain.ExtractRequest) (searchDomain.ExtractResult, error) {
	g.requests = append(g.requests, request)
	return g.result, nil
}

type recordingHTMLLoader struct {
	result searchDomain.ExtractResult
	urls   []string
}

func (l *recordingHTMLLoader) LoadHTML(_ context.Context, rawURL string) (searchDomain.ExtractResult, error) {
	l.urls = append(l.urls, rawURL)
	return l.result, nil
}

func (e *recordingExtractor) Extract(_ context.Context, request searchDomain.ExtractRequest) (searchDomain.ExtractResult, error) {
	e.requests = append(e.requests, request)
	return e.result, e.err
}
