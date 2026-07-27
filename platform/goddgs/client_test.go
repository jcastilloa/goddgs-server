package goddgs

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	ddgs "github.com/jcastilloa/goddgs"
	"github.com/jcastilloa/goddgs-server/search/domain"
)

func TestNewClientUsesNoProxyOptionForDirectConnection(t *testing.T) {
	optionCount := 0
	client := newClient("", time.Second, func(options ...ddgs.Option) sourceClient {
		optionCount = len(options)
		return &recordingSource{}
	})

	if optionCount != 1 {
		t.Errorf("option count = %d, want timeout only", optionCount)
	}
	if client == nil {
		t.Error("NewClient() returned nil")
	}
}

func TestDDGSClientDispatchesSearchCategoryAndPreservesValues(t *testing.T) {
	source := &recordingSource{results: []ddgs.RawResult{{"number": 29_059, "nested": map[string]any{"value": nil}}}}
	client := ddgsClient{source: source}
	maxResults := 5
	page := 2
	request := domain.SearchRequest{
		Category:   domain.CategoryImages,
		Query:      "forest",
		Region:     "es-es",
		SafeSearch: "off",
		TimeLimit:  "w",
		MaxResults: &maxResults,
		Page:       &page,
		Backend:    "bing",
		Images: domain.ImageOptions{
			Size: "Large", Color: "Green", Type: "photo", Layout: "Wide", License: "Share",
		},
	}

	got, err := client.Search(context.Background(), request)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !reflect.DeepEqual(got, []domain.RawResult{{"number": 29_059, "nested": map[string]any{"value": nil}}}) {
		t.Errorf("Search() = %#v", got)
	}
	if source.method != "images" || source.query != "forest" {
		t.Errorf("source call = (%q, %q), want (images, forest)", source.method, source.query)
	}
	if source.optionCount != 11 {
		t.Errorf("source option count = %d, want 11", source.optionCount)
	}
}

func TestDDGSClientKeepsNonTransportErrors(t *testing.T) {
	rateLimit := errors.New("rate limited")
	client := ddgsClient{source: &recordingSource{searchError: rateLimit}}

	_, err := client.Search(context.Background(), domain.SearchRequest{Category: domain.CategoryText, Query: "forest"})
	if !errors.Is(err, rateLimit) {
		t.Errorf("Search() error = %v, want rate-limit error", err)
	}
	if errors.Is(err, ErrTransport) {
		t.Errorf("Search() error = %v must not classify as transport", err)
	}
}

func TestDDGSClientClassifiesNetworkErrorsAsTransport(t *testing.T) {
	networkError := &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}
	client := ddgsClient{source: &recordingSource{searchError: networkError}}

	_, err := client.Search(context.Background(), domain.SearchRequest{Category: domain.CategoryText, Query: "forest"})
	if !errors.Is(err, ErrTransport) {
		t.Errorf("Search() error = %v, want ErrTransport", err)
	}
	if !errors.Is(err, networkError) {
		t.Errorf("Search() error = %v, want network error", err)
	}
}

func TestDDGSClientClassifiesSourceRateLimitAndTimeout(t *testing.T) {
	tests := []struct {
		name    string
		source  error
		wantErr error
	}{
		{name: "rate limit", source: ddgs.ErrRateLimit, wantErr: domain.ErrRateLimited},
		{name: "timeout", source: ddgs.ErrTimeout, wantErr: domain.ErrSearchTimeout},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			client := ddgsClient{source: &recordingSource{searchError: testCase.source}}
			_, err := client.Search(context.Background(), domain.SearchRequest{Category: domain.CategoryText, Query: "forest"})
			if !errors.Is(err, testCase.wantErr) {
				t.Errorf("Search() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestDDGSClientExtractForwardsFormat(t *testing.T) {
	source := &recordingSource{extractResult: ddgs.ExtractResult{URL: "https://example.com", Content: []byte("raw")}}
	client := ddgsClient{source: source}

	got, err := client.Extract(context.Background(), domain.ExtractRequest{URL: "https://example.com", Format: "raw"})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if !reflect.DeepEqual(got, domain.ExtractResult{URL: "https://example.com", Content: []byte("raw")}) {
		t.Errorf("Extract() = %#v", got)
	}
	if source.method != "extract" || source.optionCount != 1 {
		t.Errorf("source extract = (%q, %d options), want (extract, 1)", source.method, source.optionCount)
	}
}

func TestDDGSClientDispatchesVideoOptionsAndRejectsUnknownCategory(t *testing.T) {
	source := &recordingSource{}
	client := ddgsClient{source: source}
	_, err := client.Search(context.Background(), domain.SearchRequest{
		Category: domain.CategoryVideos,
		Query:    "forest",
		Videos:   domain.VideoOptions{Resolution: "high", Duration: "short", License: "youtube"},
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if source.method != "videos" || source.optionCount != 3 {
		t.Errorf("video call = (%q, %d options), want (videos, 3)", source.method, source.optionCount)
	}

	_, err = client.Search(context.Background(), domain.SearchRequest{Category: "unknown", Query: "forest"})
	if !errors.Is(err, domain.ErrInvalidSearchRequest) {
		t.Errorf("Search() error = %v, want ErrInvalidSearchRequest", err)
	}
}

func TestDDGSClientExtractUsesDefaultFormatWhenOmitted(t *testing.T) {
	source := &recordingSource{extractResult: ddgs.ExtractResult{URL: "https://example.com"}}
	client := ddgsClient{source: source}
	_, err := client.Extract(context.Background(), domain.ExtractRequest{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if source.optionCount != 0 {
		t.Errorf("extract option count = %d, want 0", source.optionCount)
	}
}

func TestDDGSClientRendersSanitizedHTMLFromMarkdown(t *testing.T) {
	source := &recordingSource{extractResult: ddgs.ExtractResult{
		URL:     "https://example.com/article",
		Content: "# Article title\n\nText with [a link](https://example.com/source).\n\n<script>alert('x')</script>",
	}}
	client := ddgsClient{source: source}

	got, err := client.Extract(context.Background(), domain.ExtractRequest{URL: "https://example.com/article", Format: "html"})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	content, ok := got.Content.(string)
	if !ok {
		t.Fatalf("Content = %T, want string", got.Content)
	}
	for _, expected := range []string{"<h1>Article title</h1>", "<p>Text with <a href=\"https://example.com/source\">a link</a>.</p>"} {
		if !strings.Contains(content, expected) {
			t.Errorf("Content = %q, want %q", content, expected)
		}
	}
	if strings.Contains(strings.ToLower(content), "script") {
		t.Errorf("Content = %q, must not contain script", content)
	}
}

type recordingSource struct {
	results       []ddgs.RawResult
	searchError   error
	extractResult ddgs.ExtractResult
	extractError  error
	method        string
	query         string
	optionCount   int
}

func (s *recordingSource) Text(_ context.Context, query string, options ...ddgs.SearchOption) ([]ddgs.RawResult, error) {
	return s.search("text", query, len(options))
}

func (s *recordingSource) Images(_ context.Context, query string, options ...ddgs.SearchOption) ([]ddgs.RawResult, error) {
	return s.search("images", query, len(options))
}

func (s *recordingSource) News(_ context.Context, query string, options ...ddgs.SearchOption) ([]ddgs.RawResult, error) {
	return s.search("news", query, len(options))
}

func (s *recordingSource) Videos(_ context.Context, query string, options ...ddgs.SearchOption) ([]ddgs.RawResult, error) {
	return s.search("videos", query, len(options))
}

func (s *recordingSource) Books(_ context.Context, query string, options ...ddgs.SearchOption) ([]ddgs.RawResult, error) {
	return s.search("books", query, len(options))
}

func (s *recordingSource) Extract(_ context.Context, query string, options ...ddgs.ExtractOption) (ddgs.ExtractResult, error) {
	s.method = "extract"
	s.query = query
	s.optionCount = len(options)
	return s.extractResult, s.extractError
}

func (s *recordingSource) search(method, query string, optionCount int) ([]ddgs.RawResult, error) {
	s.method = method
	s.query = query
	s.optionCount = optionCount
	return s.results, s.searchError
}
