package extractai

import (
	"context"
	"errors"
	"testing"

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

type recordingExtractor struct {
	result   searchDomain.ExtractResult
	err      error
	requests []searchDomain.ExtractRequest
}

func (e *recordingExtractor) Extract(_ context.Context, request searchDomain.ExtractRequest) (searchDomain.ExtractResult, error) {
	e.requests = append(e.requests, request)
	return e.result, e.err
}
