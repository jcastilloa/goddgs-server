package application

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jcastilloa/goddgs-server/search/domain"
)

func TestServiceSearchForwardsValidRequestAndPreservesDynamicResults(t *testing.T) {
	want := []domain.RawResult{{
		"title":  "example",
		"score":  42,
		"nested": map[string]any{"key": nil},
	}}
	gateway := &recordingGateway{results: want}
	service := NewService(gateway)
	request := domain.SearchRequest{
		Category:   domain.CategoryText,
		Query:      "metasearch",
		Region:     "es-es",
		SafeSearch: "off",
		Backend:    "brave",
	}

	got, err := service.Search(context.Background(), request)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Search() = %#v, want %#v", got, want)
	}
	if len(gateway.searches) != 1 || !reflect.DeepEqual(gateway.searches[0], request) {
		t.Errorf("gateway searches = %#v, want %#v", gateway.searches, []domain.SearchRequest{request})
	}
}

func TestServiceSearchRejectsInvalidRequestBeforeCallingGateway(t *testing.T) {
	gateway := &recordingGateway{}
	service := NewService(gateway)

	_, err := service.Search(context.Background(), domain.SearchRequest{Category: domain.CategoryText, Query: " "})
	if !errors.Is(err, domain.ErrInvalidSearchRequest) {
		t.Errorf("Search() error = %v, want ErrInvalidSearchRequest", err)
	}
	if len(gateway.searches) != 0 {
		t.Errorf("gateway calls = %d, want 0", len(gateway.searches))
	}
}

func TestServiceSearchHonorsCanceledContextBeforeCallingGateway(t *testing.T) {
	gateway := &recordingGateway{}
	service := NewService(gateway)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.Search(ctx, domain.SearchRequest{Category: domain.CategoryText, Query: "metasearch"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Search() error = %v, want context.Canceled", err)
	}
	if len(gateway.searches) != 0 {
		t.Errorf("gateway calls = %d, want 0", len(gateway.searches))
	}
}

func TestServiceExtractValidatesURLAndForwardsRequestedFormat(t *testing.T) {
	gateway := &recordingGateway{extractResult: domain.ExtractResult{URL: "https://example.com", Content: []byte("raw")}}
	service := NewService(gateway)
	request := domain.ExtractRequest{URL: "https://example.com", Format: "raw"}

	got, err := service.Extract(context.Background(), request)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if !reflect.DeepEqual(got, gateway.extractResult) {
		t.Errorf("Extract() = %#v, want %#v", got, gateway.extractResult)
	}
	if len(gateway.extracts) != 1 || gateway.extracts[0] != request {
		t.Errorf("gateway extracts = %#v, want %#v", gateway.extracts, []domain.ExtractRequest{request})
	}
}

func TestServiceExtractUsesHTMLLoaderOnlyForHTML(t *testing.T) {
	gateway := &recordingGateway{extractResult: domain.ExtractResult{Content: "gateway"}}
	loader := &recordingHTMLLoader{result: domain.ExtractResult{URL: "https://example.com/final", Content: "<article>rendered</article>"}}
	service := NewService(gateway, loader)

	got, err := service.Extract(context.Background(), domain.ExtractRequest{URL: "https://example.com/article", Format: "html"})
	if err != nil {
		t.Fatalf("Extract() HTML error = %v", err)
	}
	if !reflect.DeepEqual(got, loader.result) {
		t.Errorf("Extract() HTML = %#v, want %#v", got, loader.result)
	}
	if len(loader.urls) != 1 || loader.urls[0] != "https://example.com/article" {
		t.Errorf("HTML loader URLs = %#v", loader.urls)
	}
	if len(gateway.extracts) != 0 {
		t.Errorf("gateway HTML extracts = %#v, want none", gateway.extracts)
	}

	got, err = service.Extract(context.Background(), domain.ExtractRequest{URL: "https://example.com/article", Format: "text_plain"})
	if err != nil {
		t.Fatalf("Extract() non-HTML error = %v", err)
	}
	if !reflect.DeepEqual(got, gateway.extractResult) {
		t.Errorf("Extract() non-HTML = %#v, want %#v", got, gateway.extractResult)
	}
	if len(loader.urls) != 1 {
		t.Errorf("HTML loader URLs after non-HTML = %#v", loader.urls)
	}
	if len(gateway.extracts) != 1 || gateway.extracts[0].Format != "text_plain" {
		t.Errorf("gateway extracts = %#v", gateway.extracts)
	}
}

func TestServiceExtractHonorsCanceledContextAndUnavailableGateway(t *testing.T) {
	request := domain.ExtractRequest{URL: "https://example.com"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewService(&recordingGateway{}).Extract(ctx, request)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Extract() error = %v, want context.Canceled", err)
	}

	_, err = NewService(nil).Extract(context.Background(), request)
	if !errors.Is(err, ErrGatewayUnavailable) {
		t.Errorf("Extract() error = %v, want ErrGatewayUnavailable", err)
	}
}

func TestServiceSearchRejectsUnavailableGateway(t *testing.T) {
	_, err := NewService(nil).Search(context.Background(), domain.SearchRequest{Category: domain.CategoryText, Query: "metasearch"})
	if !errors.Is(err, ErrGatewayUnavailable) {
		t.Errorf("Search() error = %v, want ErrGatewayUnavailable", err)
	}
}

type recordingGateway struct {
	results       []domain.RawResult
	searchError   error
	extractResult domain.ExtractResult
	extractError  error
	searches      []domain.SearchRequest
	extracts      []domain.ExtractRequest
}

type recordingHTMLLoader struct {
	result domain.ExtractResult
	err    error
	urls   []string
}

func (l *recordingHTMLLoader) LoadHTML(_ context.Context, rawURL string) (domain.ExtractResult, error) {
	l.urls = append(l.urls, rawURL)
	return l.result, l.err
}

func (g *recordingGateway) Search(_ context.Context, request domain.SearchRequest) ([]domain.RawResult, error) {
	g.searches = append(g.searches, request)
	return g.results, g.searchError
}

func (g *recordingGateway) Extract(_ context.Context, request domain.ExtractRequest) (domain.ExtractResult, error) {
	g.extracts = append(g.extracts, request)
	return g.extractResult, g.extractError
}
