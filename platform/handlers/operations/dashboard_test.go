package operations

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	operationsDomain "github.com/jcastilloa/goddgs-server/operations/domain"

	"github.com/gin-gonic/gin"
)

func TestSummaryRejectsInvalidRange(t *testing.T) {
	engine := handlerEngine(NewSummary(&fakeUseCase{}))

	recorder := serve(engine, "/?range=90d")

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", recorder.Code)
	}
}

func TestTimeSeriesRejectsUnsupportedInterval(t *testing.T) {
	engine := handlerEngine(NewTimeSeries(&fakeUseCase{}))

	recorder := serve(engine, "/?range=24h&interval=30m")

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", recorder.Code)
	}
}

func TestListReturnsRequestedPagination(t *testing.T) {
	useCase := fakeUseCase{page: operationsDomain.OperationsPage{Operations: []operationsDomain.Operation{{ID: "operation-1"}}, Total: 4}}
	engine := handlerEngine(NewList(&useCase))

	recorder := serve(engine, "/?range=7d&limit=2&offset=1")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", recorder.Code, recorder.Body.String())
	}
	if useCase.query.Limit != 2 || useCase.query.Offset != 1 {
		t.Errorf("query = %#v, want limit 2 and offset 1", useCase.query)
	}
	var page operationsDomain.OperationsPage
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if page.Total != 4 || len(page.Operations) != 1 || page.Operations[0].ID != "operation-1" {
		t.Errorf("page = %#v", page)
	}
}

func TestListRejectsOutOfBoundsLimit(t *testing.T) {
	engine := handlerEngine(NewList(&fakeUseCase{}))

	recorder := serve(engine, "/?limit=101")

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", recorder.Code)
	}
}

func TestDetailReturnsNotFoundForMissingOperation(t *testing.T) {
	engine := gin.New()
	engine.GET("/:id", NewDetail(&fakeUseCase{}).Handle)

	recorder := serve(engine, "/missing")

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", recorder.Code)
	}
}

func TestListReturnsEmptyOperationsArray(t *testing.T) {
	engine := handlerEngine(NewList(&fakeUseCase{page: operationsDomain.OperationsPage{Operations: []operationsDomain.Operation{}}}))

	recorder := serve(engine, "/")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body struct {
		Operations []operationsDomain.Operation `json:"operations"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Operations == nil || len(body.Operations) != 0 {
		t.Errorf("operations = %#v, want non-nil empty array", body.Operations)
	}
}

func TestListOmitsZeroFinishedAt(t *testing.T) {
	page := operationsDomain.OperationsPage{Operations: []operationsDomain.Operation{{
		ID:        "running-operation",
		Type:      operationsDomain.OperationSearch,
		Status:    operationsDomain.StatusRunning,
		StartedAt: time.Date(2026, time.July, 28, 10, 0, 0, 0, time.UTC),
	}}}
	engine := handlerEngine(NewList(&fakeUseCase{page: page}))

	recorder := serve(engine, "/")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "finished_at") {
		t.Errorf("response = %s, must omit zero finished_at", recorder.Body.String())
	}
}

func TestSummaryReturnsInternalServerErrorWhenStoreFails(t *testing.T) {
	engine := handlerEngine(NewSummary(&fakeUseCase{err: errors.New("database unavailable")}))

	recorder := serve(engine, "/")

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", recorder.Code)
	}
}

func TestDashboardContainsOperationalControlsAndLiveDuration(t *testing.T) {
	engine := handlerEngine(NewDashboard())

	recorder := serve(engine, "/")

	if recorder.Code != http.StatusOK || !containsAll(recorder.Body.String(),
		"<html lang=\"en\">",
		"<title>Operations</title>",
		"en-GB",
		"Running ·",
		"https://cdn.tailwindcss.com",
		"https://cdn.jsdelivr.net/npm/chart.js",
		"setInterval(refresh, 5000)",
		"setInterval(updateLiveDurations, 1000)",
		"formatOperationDuration",
		"operations-status",
		"operations-type",
		"load-more",
		"operation-inspector",
		"Open JSON viewer",
		"updated-at",
		"proxy-health",
		"account-username",
		"Change password",
		"Sign out",
		"operations_csrf",
		"/operations/api/auth/logout",
		"/operations/api/auth/password",
		"await handleResponse(await fetch('/operations/api/auth/logout'",
		"function setAccountMenu(open)",
		"function openPasswordDialog()",
		"function closePasswordDialog()",
		"event.key === 'Escape'",
	) {
		t.Errorf("dashboard response = status %d body %q", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "finally { window.location.assign('/operations/login'); }") {
		t.Error("dashboard logout redirects even when the logout request fails")
	}
}

func TestDashboardContainsSearchableJSONViewer(t *testing.T) {
	engine := handlerEngine(NewDashboard())

	recorder := serve(engine, "/")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if !containsAll(recorder.Body.String(),
		"json-viewer",
		"json-viewer-dialog",
		"json-viewer-dialog { display: none; }",
		"json-viewer-dialog[open] {",
		"grid-template-rows: auto minmax(0, 1fr);",
		"#json-viewer-content { min-height: 0; overflow: hidden; }",
		"grid-template-rows: auto auto minmax(0, 1fr);",
		"overscroll-behavior: contain;",
		"html.json-viewer-open, body.json-viewer-open { overflow: hidden; }",
		"setJSONViewerScrollLock(true)",
		"setJSONViewerScrollLock(false)",
		"tree.hidden = true",
		"tree.hidden = false",
		"Open JSON viewer",
		"Search JSON",
		"Previous",
		"Next",
		"Expand all",
		"Collapse all",
		"Copy JSON",
		"Raw JSON",
		"openJSONViewer",
		"closeJSONViewer",
		"renderJSONNode",
		"filterJSONViewer",
		"matchingJSONNodes",
		"nextJSONMatchIndex",
	) {
		t.Errorf("dashboard must include the generic JSON viewer")
	}
}

func handlerEngine(handler interface{ Handle(*gin.Context) }) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/", handler.Handle)
	return engine
}

func serve(engine *gin.Engine, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func containsAll(value string, values ...string) bool {
	for _, expected := range values {
		if !strings.Contains(value, expected) {
			return false
		}
	}
	return true
}

type fakeUseCase struct {
	query operationsDomain.OperationQuery
	page  operationsDomain.OperationsPage
	err   error
}

func (f *fakeUseCase) Summary(context.Context, operationsDomain.DashboardRange) (operationsDomain.DashboardSummary, error) {
	return operationsDomain.DashboardSummary{}, f.err
}

func (f *fakeUseCase) TimeSeries(context.Context, operationsDomain.TimeSeriesQuery) ([]operationsDomain.TimeSeriesBucket, error) {
	return []operationsDomain.TimeSeriesBucket{}, f.err
}

func (f *fakeUseCase) ListOperations(_ context.Context, query operationsDomain.OperationQuery) (operationsDomain.OperationsPage, error) {
	f.query = query
	return f.page, f.err
}

func (f *fakeUseCase) GetOperation(context.Context, string) (operationsDomain.OperationDetail, bool, error) {
	return operationsDomain.OperationDetail{}, false, f.err
}

func (f *fakeUseCase) ListProxies(context.Context, operationsDomain.DashboardRange) ([]operationsDomain.ProxyDashboard, error) {
	return []operationsDomain.ProxyDashboard{}, f.err
}

var _ DashboardUseCase = (*fakeUseCase)(nil)

func TestParseDateRangeRejectsOverThirtyDays(t *testing.T) {
	_, err := parseDateRange("", "2026-06-01T00:00:00Z", "2026-07-02T00:00:00Z", time.Now())
	if err == nil {
		t.Error("parseDateRange() error = nil, want invalid range")
	}
}

func TestParseDateRangeRejectsMixedRangeAndTimestamps(t *testing.T) {
	_, err := parseDateRange("24h", "2026-07-27T00:00:00Z", "2026-07-28T00:00:00Z", time.Now())
	if err == nil {
		t.Error("parseDateRange() error = nil, want invalid combined range")
	}
}
