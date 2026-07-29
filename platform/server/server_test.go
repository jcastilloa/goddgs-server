package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	operationsApplication "github.com/jcastilloa/goddgs-server/operations/application"
	operations "github.com/jcastilloa/goddgs-server/operations/domain"
	containerdi "github.com/jcastilloa/goddgs-server/platform/di"
	operationsHandler "github.com/jcastilloa/goddgs-server/platform/handlers/operations"
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

func TestServerRequiresDashboardSessionWithoutAffectingBearerEndpoints(t *testing.T) {
	httpServer, closeContainer := newServer(t, "token", time.Second, &serverGateway{})
	defer closeContainer()

	recorder := httptest.NewRecorder()
	httpServer.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/operations", nil))
	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != "/operations/setup" {
		t.Errorf("GET /operations status = %d location = %q, want 303 /operations/setup", recorder.Code, recorder.Header().Get("Location"))
	}
	recorder = httptest.NewRecorder()
	httpServer.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/operations/api/summary", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("GET /operations/api/summary status = %d, want 401; body = %s", recorder.Code, recorder.Body.String())
	}

	for _, path := range []string{"/v1/version", "/openapi.json", "/docs/"} {
		recorder := httptest.NewRecorder()
		httpServer.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("GET %s status = %d, want 401", path, recorder.Code)
		}
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

func TestServerWithRecorderKeepsSearchContractUnchanged(t *testing.T) {
	gateway := &serverGateway{results: []domain.RawResult{{"title": "result"}}}
	container, err := containerdi.New("test", searchApplication.NewService(gateway), nil, nil, operationsHandler.EmptyUseCase{}).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer (*container).Delete()
	server := NewWithRecorder(*container, "/v1", "test", "", time.Second, time.Second, operationsApplication.NewEventRecorder(&noopRepository{}, time.Now, func() string { return "operation-1" }))

	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/text?q=query", nil))
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", recorder.Code)
	}
}

func TestServerWithRecorderInstrumentsEverySearchRoute(t *testing.T) {
	gateway := &serverGateway{results: []domain.RawResult{{"title": "result"}}}
	recorderRepository := &recordingRepository{}
	container, err := containerdi.New("test", searchApplication.NewService(gateway), nil, nil, operationsHandler.EmptyUseCase{}).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer (*container).Delete()

	server := NewWithRecorder(*container, "/v1", "test", "", time.Second, time.Second, operationsApplication.NewEventRecorder(recorderRepository, time.Now, func() string { return "operation-1" }))
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/v1/text?q=query", nil),
		httptest.NewRequest(http.MethodGet, "/v1/news?q=query", nil),
	} {
		recorder := httptest.NewRecorder()
		server.engine.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", request.URL.Path, recorder.Code)
		}
	}
	if len(recorderRepository.operations) != 2 {
		t.Fatalf("operations = %d, want 2", len(recorderRepository.operations))
	}
	if got := recorderRepository.operations[0].Metadata["category"]; got != "text" {
		t.Errorf("text category = %q, want text", got)
	}
	if got := recorderRepository.operations[1].Metadata["category"]; got != "news" {
		t.Errorf("news category = %q, want news", got)
	}
}

func TestOperationStartRecordsResearchRequestWithoutConsumingIt(t *testing.T) {
	body := []byte(`{"query":"why are URLs strange?","query_count":3,"query_languages":["en","es"]}`)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/research", bytes.NewReader(body))

	start, recorded := operationStart(context)

	if !recorded || start.Type != operations.OperationResearch {
		t.Fatalf("operationStart() = %#v, %t", start, recorded)
	}
	var details map[string]any
	if err := json.Unmarshal(start.Details, &details); err != nil {
		t.Fatalf("decode details: %v", err)
	}
	request, ok := details["request"].(map[string]any)
	if !ok || request["query"] != "why are URLs strange?" || request["query_count"] != float64(3) {
		t.Errorf("recorded request = %#v", details)
	}
	restored, err := io.ReadAll(context.Request.Body)
	if err != nil {
		t.Fatalf("read restored request body: %v", err)
	}
	if !bytes.Equal(restored, body) {
		t.Errorf("restored body = %q, want %q", restored, body)
	}
}

func newServer(t *testing.T, token string, timeout time.Duration, gateway *serverGateway) (*Server, func()) {
	t.Helper()
	container, err := containerdi.New("test", searchApplication.NewService(gateway), nil, nil, operationsHandler.EmptyUseCase{}).Build()
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

type noopRepository struct{}

func (noopRepository) CreateOperation(context.Context, operations.Operation) error { return nil }
func (noopRepository) FinishOperation(context.Context, operations.Operation) error { return nil }
func (noopRepository) AddStep(context.Context, operations.Step) error              { return nil }
func (noopRepository) FinishStep(context.Context, operations.Step) error           { return nil }
func (noopRepository) AddError(context.Context, operations.OperationError) error   { return nil }
func (noopRepository) RecordProbe(context.Context, operations.ProxyProbe) error    { return nil }
func (noopRepository) RecordHealthTransition(context.Context, operations.ProxyHealthTransition) error {
	return nil
}
func (noopRepository) ListOperations(context.Context, operations.OperationQuery) ([]operations.Operation, error) {
	return nil, nil
}

type recordingRepository struct {
	operations []operations.Operation
}

func (r *recordingRepository) CreateOperation(_ context.Context, operation operations.Operation) error {
	r.operations = append(r.operations, operation)
	return nil
}
func (r *recordingRepository) FinishOperation(_ context.Context, operation operations.Operation) error {
	for index := range r.operations {
		if r.operations[index].ID == operation.ID {
			operation.Metadata = r.operations[index].Metadata
			operation.Type = r.operations[index].Type
			operation.HTTPMethod = r.operations[index].HTTPMethod
			operation.HTTPPath = r.operations[index].HTTPPath
			r.operations[index] = operation
			return nil
		}
	}
	return nil
}
func (r *recordingRepository) AddStep(context.Context, operations.Step) error            { return nil }
func (r *recordingRepository) FinishStep(context.Context, operations.Step) error         { return nil }
func (r *recordingRepository) AddError(context.Context, operations.OperationError) error { return nil }
func (r *recordingRepository) RecordProbe(context.Context, operations.ProxyProbe) error  { return nil }
func (r *recordingRepository) RecordHealthTransition(context.Context, operations.ProxyHealthTransition) error {
	return nil
}
func (r *recordingRepository) ListOperations(context.Context, operations.OperationQuery) ([]operations.Operation, error) {
	return nil, nil
}
