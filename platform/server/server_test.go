package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	containerdi "github.com/jcastilloa/goddgs-server/platform/di"
	searchApplication "github.com/jcastilloa/goddgs-server/search/application"
	"github.com/jcastilloa/goddgs-server/search/domain"
)

func TestServerRegistersEveryVersionedSearchRoute(t *testing.T) {
	gateway := &serverGateway{results: []domain.RawResult{{"title": "result", "rank": 9}}, extractResult: domain.ExtractResult{URL: "https://example.com", Content: "body"}}
	httpServer, closeContainer := newServer(t, "", time.Second, gateway)
	defer closeContainer()

	for _, path := range []string{
		"/v1/text?q=query",
		"/v1/images?q=query",
		"/v1/news?q=query",
		"/v1/videos?q=query",
		"/v1/books?q=query",
		"/v1/extract?url=https%3A%2F%2Fexample.com",
	} {
		recorder := httptest.NewRecorder()
		httpServer.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Errorf("GET %s status = %d, body = %s", path, recorder.Code, recorder.Body.String())
		}
	}
	if len(gateway.searches) != 5 {
		t.Errorf("search calls = %d, want 5", len(gateway.searches))
	}
}

func TestServerAppliesAuthenticationToSearchRoutes(t *testing.T) {
	httpServer, closeContainer := newServer(t, "token", time.Second, &serverGateway{})
	defer closeContainer()

	recorder := httptest.NewRecorder()
	httpServer.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/text?q=query", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("missing auth status = %d, want 401", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/text?q=query", nil)
	request.Header.Set("Authorization", "Bearer token")
	httpServer.engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Errorf("valid auth status = %d, want 200", recorder.Code)
	}
}

func TestServerAppliesRequestTimeout(t *testing.T) {
	gateway := &serverGateway{search: func(ctx context.Context, _ domain.SearchRequest) ([]domain.RawResult, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	httpServer, closeContainer := newServer(t, "", time.Millisecond, gateway)
	defer closeContainer()

	recorder := httptest.NewRecorder()
	httpServer.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/text?q=query", nil))
	if recorder.Code != http.StatusGatewayTimeout {
		t.Errorf("status = %d, want 504; body = %s", recorder.Code, recorder.Body.String())
	}
}

func newServer(t *testing.T, token string, timeout time.Duration, gateway *serverGateway) (*Server, func()) {
	t.Helper()
	container, err := containerdi.New("test", searchApplication.NewService(gateway)).Build()
	if err != nil {
		t.Fatalf("build container: %v", err)
	}
	return New(*container, "/v1", token, timeout), func() { _ = (*container).Delete() }
}

type serverGateway struct {
	results       []domain.RawResult
	extractResult domain.ExtractResult
	search        func(context.Context, domain.SearchRequest) ([]domain.RawResult, error)
	searches      []domain.SearchRequest
}

func (g *serverGateway) Search(ctx context.Context, request domain.SearchRequest) ([]domain.RawResult, error) {
	g.searches = append(g.searches, request)
	if g.search != nil {
		return g.search(ctx, request)
	}
	return g.results, nil
}

func (g *serverGateway) Extract(_ context.Context, _ domain.ExtractRequest) (domain.ExtractResult, error) {
	return g.extractResult, nil
}

func decodeJSON(t *testing.T, source []byte) any {
	t.Helper()
	var value any
	if err := json.Unmarshal(source, &value); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return value
}
