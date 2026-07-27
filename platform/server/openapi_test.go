package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestServerServesDynamicOpenAPISpecificationAndSwaggerUI(t *testing.T) {
	httpServer, closeContainer := newServer(t, "", time.Second, &serverGateway{})
	defer closeContainer()

	recorder := httptest.NewRecorder()
	httpServer.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("openapi status = %d, want 200", recorder.Code)
	}
	var specification map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &specification); err != nil {
		t.Fatalf("decode specification: %v", err)
	}
	if specification["openapi"] != "3.1.0" {
		t.Errorf("openapi = %#v, want 3.1.0", specification["openapi"])
	}
	if info, ok := specification["info"].(map[string]any); !ok || info["version"] != "test" {
		t.Errorf("info = %#v, want configured version", specification["info"])
	}
	paths, ok := specification["paths"].(map[string]any)
	if !ok || paths["/v1/text"] == nil || paths["/v1/extract"] == nil {
		t.Errorf("paths = %#v, want /v1/text and /v1/extract", specification["paths"])
	}

	recorder = httptest.NewRecorder()
	httpServer.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("docs status = %d, want 200", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Errorf("docs content type = %q, want HTML", contentType)
	}

	recorder = httptest.NewRecorder()
	httpServer.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/docs/swagger-ui.css", nil))
	if recorder.Code != http.StatusOK {
		t.Errorf("Swagger CSS status = %d, want 200", recorder.Code)
	}
}

func TestServerProtectsDocumentationWhenAuthenticationIsEnabled(t *testing.T) {
	httpServer, closeContainer := newServer(t, "token", time.Second, &serverGateway{})
	defer closeContainer()

	recorder := httptest.NewRecorder()
	httpServer.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("openapi status = %d, want 401", recorder.Code)
	}
}

func TestServerDocumentsBearerRequirementWhenAuthenticationIsEnabled(t *testing.T) {
	httpServer, closeContainer := newServer(t, "token", time.Second, &serverGateway{})
	defer closeContainer()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	request.Header.Set("Authorization", "Bearer token")
	httpServer.engine.ServeHTTP(recorder, request)

	var specification struct {
		Security []map[string][]string `json:"security"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &specification); err != nil {
		t.Fatalf("decode specification: %v", err)
	}
	if len(specification.Security) != 1 || specification.Security[0]["bearerAuth"] == nil {
		t.Errorf("security = %#v, want bearerAuth requirement", specification.Security)
	}
}
