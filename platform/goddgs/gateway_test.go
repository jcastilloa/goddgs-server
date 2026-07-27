package goddgs

import (
	"context"
	"errors"
	"reflect"
	"testing"

	proxyApplication "github.com/jcastilloa/goddgs-server/proxy/application"
	"github.com/jcastilloa/goddgs-server/search/domain"
)

func TestGatewayRetriesTransportFailureWithAnotherHealthyClient(t *testing.T) {
	failed := &fakeClient{searchError: ErrTransport}
	want := []domain.RawResult{{"title": "result", "rank": 7}}
	succeeded := &fakeClient{results: want}
	gateway := newGateway(t, 1,
		proxyApplication.Entry[client]{Key: "tunnel-a", Value: failed},
		proxyApplication.Entry[client]{Key: "direct-b", Value: succeeded},
	)

	got, err := gateway.Search(context.Background(), domain.SearchRequest{Category: domain.CategoryText, Query: "metasearch"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Search() = %#v, want %#v", got, want)
	}
	if failed.searchCalls != 1 {
		t.Errorf("failed client calls = %d, want 1", failed.searchCalls)
	}
	if succeeded.searchCalls != 1 {
		t.Errorf("succeeded client calls = %d, want 1", succeeded.searchCalls)
	}
}

func TestGatewayDoesNotRetryApplicationFailure(t *testing.T) {
	rateLimit := errors.New("rate limited")
	failed := &fakeClient{searchError: rateLimit}
	next := &fakeClient{results: []domain.RawResult{{"title": "must not run"}}}
	gateway := newGateway(t, 3,
		proxyApplication.Entry[client]{Key: "direct-a", Value: failed},
		proxyApplication.Entry[client]{Key: "direct-b", Value: next},
	)

	_, err := gateway.Search(context.Background(), domain.SearchRequest{Category: domain.CategoryText, Query: "metasearch"})
	if !errors.Is(err, rateLimit) {
		t.Errorf("Search() error = %v, want rate-limit error", err)
	}
	if next.searchCalls != 0 {
		t.Errorf("next client calls = %d, want 0", next.searchCalls)
	}
}

func TestGatewayReturnsTransportErrorWhenAllCandidatesFail(t *testing.T) {
	gateway := newGateway(t, 1,
		proxyApplication.Entry[client]{Key: "direct-a", Value: &fakeClient{searchError: ErrTransport}},
	)

	_, err := gateway.Search(context.Background(), domain.SearchRequest{Category: domain.CategoryText, Query: "metasearch"})
	if !errors.Is(err, ErrTransport) {
		t.Errorf("Search() error = %v, want ErrTransport", err)
	}
}

func TestGatewayKeepsDirectClientAvailableAfterTransientTransportFailure(t *testing.T) {
	client := &fakeClient{searchError: ErrTransport}
	gateway := newGateway(t, 0, proxyApplication.Entry[Client]{Key: "direct", Value: client})
	request := domain.SearchRequest{Category: domain.CategoryText, Query: "metasearch"}

	if _, err := gateway.Search(context.Background(), request); !errors.Is(err, ErrTransport) {
		t.Fatalf("first Search() error = %v, want ErrTransport", err)
	}

	client.searchError = nil
	client.results = []domain.RawResult{{"title": "recovered"}}
	results, err := gateway.Search(context.Background(), request)
	if err != nil {
		t.Fatalf("Search() after transient failure error = %v", err)
	}
	if len(results) != 1 || results[0]["title"] != "recovered" {
		t.Errorf("Search() after transient failure = %#v, want recovered result", results)
	}
}

func TestGatewayExtractRetriesTransportFailure(t *testing.T) {
	failed := &fakeClient{extractError: ErrTransport}
	want := domain.ExtractResult{URL: "https://example.com", Content: []byte("source")}
	succeeded := &fakeClient{extractResult: want}
	gateway := newGateway(t, 1,
		proxyApplication.Entry[client]{Key: "tunnel-a", Value: failed},
		proxyApplication.Entry[client]{Key: "direct-b", Value: succeeded},
	)

	got, err := gateway.Extract(context.Background(), domain.ExtractRequest{URL: want.URL, Format: "raw"})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Extract() = %#v, want %#v", got, want)
	}
}

func TestGatewayNormalizesNegativeRetriesAndPropagatesCanceledContext(t *testing.T) {
	client := &fakeClient{results: []domain.RawResult{{"title": "result"}}}
	gateway := newGateway(t, -1, proxyApplication.Entry[Client]{Key: "direct", Value: client})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := gateway.Search(ctx, domain.SearchRequest{Category: domain.CategoryText, Query: "metasearch"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Search() error = %v, want context.Canceled", err)
	}
	if client.searchCalls != 0 {
		t.Errorf("client calls = %d, want 0", client.searchCalls)
	}
}

type fakeClient struct {
	results       []domain.RawResult
	searchError   error
	extractResult domain.ExtractResult
	extractError  error
	searchCalls   int
	extractCalls  int
}

func (c *fakeClient) Search(context.Context, domain.SearchRequest) ([]domain.RawResult, error) {
	c.searchCalls++
	return c.results, c.searchError
}

func (c *fakeClient) Extract(context.Context, domain.ExtractRequest) (domain.ExtractResult, error) {
	c.extractCalls++
	return c.extractResult, c.extractError
}

func newGateway(t *testing.T, retries int, entries ...proxyApplication.Entry[Client]) *Gateway {
	t.Helper()
	gateway, err := NewGateway(entries, retries)
	if err != nil {
		t.Fatalf("NewGateway() error = %v", err)
	}
	return gateway
}
