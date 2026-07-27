package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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

func TestServerRegistersOnlyPostResearchRouteAndProtectsIt(t *testing.T) {
	httpServer, closeContainer := newServer(t, "token", time.Second, &serverGateway{})
	defer closeContainer()

	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/v1/research", nil),
		httptest.NewRequest(http.MethodPost, "/v1/research", nil),
	} {
		recorder := httptest.NewRecorder()
		httpServer.engine.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s status = %d, want 401", request.Method, recorder.Code)
		}
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/research", nil)
	request.Header.Set("Authorization", "Bearer token")
	httpServer.engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Errorf("GET research status = %d, want 404", recorder.Code)
	}
}

func TestServerDoesNotRegisterRemovedHelloRoute(t *testing.T) {
	httpServer, closeContainer := newServer(t, "", time.Second, &serverGateway{})
	defer closeContainer()

	recorder := httptest.NewRecorder()
	httpServer.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/hello", nil))
	if recorder.Code != http.StatusNotFound {
		t.Errorf("GET /v1/hello status = %d, want 404", recorder.Code)
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

func TestRequestTimeoutUsesDedicatedTimeouts(t *testing.T) {
	engine := gin.New()
	engine.Use(requestTimeoutMiddleware(time.Millisecond, 40*time.Millisecond, "/v1/research", "/v1/extract"))
	engine.POST("/v1/research", func(context *gin.Context) {
		select {
		case <-time.After(10 * time.Millisecond):
			context.Status(http.StatusOK)
		case <-context.Request.Context().Done():
			context.Status(http.StatusGatewayTimeout)
		}
	})
	engine.GET("/v1/extract", func(context *gin.Context) {
		select {
		case <-time.After(10 * time.Millisecond):
			context.Status(http.StatusOK)
		case <-context.Request.Context().Done():
			context.Status(http.StatusGatewayTimeout)
		}
	})
	engine.GET("/v1/text", func(context *gin.Context) {
		<-context.Request.Context().Done()
		context.Status(http.StatusGatewayTimeout)
	})

	tests := []struct {
		name    string
		request *http.Request
		want    int
	}{
		{name: "research", request: httptest.NewRequest(http.MethodPost, "/v1/research", nil), want: http.StatusOK},
		{name: "AI extraction", request: httptest.NewRequest(http.MethodGet, "/v1/extract?mode=ai", nil), want: http.StatusOK},
		{name: "heuristic extraction", request: httptest.NewRequest(http.MethodGet, "/v1/extract", nil), want: http.StatusGatewayTimeout},
		{name: "search", request: httptest.NewRequest(http.MethodGet, "/v1/text", nil), want: http.StatusGatewayTimeout},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, test.request)
			if recorder.Code != test.want {
				t.Errorf("%s %s status = %d, want %d", test.request.Method, test.request.URL.String(), recorder.Code, test.want)
			}
		})
	}
}

func TestServerRunStopsWhenContextIsCanceled(t *testing.T) {
	httpServer, closeContainer := newServer(t, "", time.Second, &serverGateway{})
	defer closeContainer()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := httpServer.Run(ctx, "127.0.0.1:0"); err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}
}

func newServer(t *testing.T, token string, timeout time.Duration, gateway *serverGateway) (*Server, func()) {
	t.Helper()
	container, err := containerdi.New("test", searchApplication.NewService(gateway), nil, nil).Build()
	if err != nil {
		t.Fatalf("build container: %v", err)
	}
	return New(*container, "/v1", "test", token, timeout, time.Second), func() { _ = (*container).Delete() }
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
