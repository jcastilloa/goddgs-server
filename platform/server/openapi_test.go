package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	if specification["openapi"] != "3.0.3" {
		t.Errorf("openapi = %#v, want 3.0.3", specification["openapi"])
	}
	if info, ok := specification["info"].(map[string]any); !ok || info["version"] != "test" {
		t.Errorf("info = %#v, want configured version", specification["info"])
	}
	paths, ok := specification["paths"].(map[string]any)
	if !ok || paths["/v1/text"] == nil || paths["/v1/extract"] == nil {
		t.Errorf("paths = %#v, want /v1/text and /v1/extract", specification["paths"])
	}
	if paths["/v1/hello"] != nil {
		t.Errorf("paths = %#v, must not expose removed /v1/hello endpoint", paths)
	}
	extractPath := paths["/v1/extract"].(map[string]any)
	extractGet := extractPath["get"].(map[string]any)
	if description := extractGet["description"].(string); description == "" || !containsAll(description, "mode=heuristic", "mode=ai", "llm.base_url", "format") {
		t.Errorf("extract description = %q", description)
	}
	parameters := extractGet["parameters"].([]any)
	if parameters[2].(map[string]any)["name"] != "mode" {
		t.Errorf("extract parameters = %#v, want mode", parameters)
	}
	if parameters[0].(map[string]any)["description"] == "" || parameters[1].(map[string]any)["description"] == "" || parameters[2].(map[string]any)["description"] == "" {
		t.Errorf("extract parameters need descriptions = %#v", parameters)
	}
	responses := extractGet["responses"].(map[string]any)
	if responses["503"].(map[string]any)["description"] != "AI extraction is not configured or unavailable. Heuristic extraction remains available." {
		t.Errorf("extract responses = %#v", responses)
	}
	if responses["200"].(map[string]any)["content"] == nil || responses["400"].(map[string]any)["content"] == nil || responses["503"].(map[string]any)["content"] == nil {
		t.Errorf("extract responses need documented payloads = %#v", responses)
	}
	assertDetailedDocumentation(t, paths)

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

func assertDetailedDocumentation(t *testing.T, paths map[string]any) {
	t.Helper()
	for _, path := range []string{"/v1/version", "/v1/text", "/v1/images", "/v1/news", "/v1/videos", "/v1/books"} {
		operation := documentedGetOperation(t, paths, path)
		if operation["summary"] == "" || operation["description"] == "" {
			t.Errorf("%s operation = %#v, want summary and description", path, operation)
		}
		responses := operation["responses"].(map[string]any)
		if responses["200"].(map[string]any)["content"] == nil || responses["401"].(map[string]any)["content"] == nil {
			t.Errorf("%s responses = %#v, want documented JSON responses", path, responses)
		}
	}

	text := documentedGetOperation(t, paths, "/v1/text")
	if !hasParameter(text, "q") || !hasParameter(text, "query") || !hasParameter(text, "safesearch") || !hasParameter(text, "backend") {
		t.Errorf("text parameters = %#v, want documented common search parameters", text["parameters"])
	}
	if text["responses"].(map[string]any)["503"] == nil || text["responses"].(map[string]any)["504"] == nil {
		t.Errorf("text responses = %#v, want documented upstream errors", text["responses"])
	}

	images := documentedGetOperation(t, paths, "/v1/images")
	for _, parameter := range []string{"size", "color", "type_image", "layout", "license_image"} {
		if !hasParameter(images, parameter) {
			t.Errorf("images parameters = %#v, want %q", images["parameters"], parameter)
		}
	}

	videos := documentedGetOperation(t, paths, "/v1/videos")
	for _, parameter := range []string{"resolution", "duration", "license_videos"} {
		if !hasParameter(videos, parameter) {
			t.Errorf("videos parameters = %#v, want %q", videos["parameters"], parameter)
		}
	}

	books := documentedGetOperation(t, paths, "/v1/books")
	if hasParameter(books, "region") || hasParameter(books, "safesearch") || hasParameter(books, "timelimit") {
		t.Errorf("books parameters = %#v, must only document supported book parameters", books["parameters"])
	}
}

func documentedGetOperation(t *testing.T, paths map[string]any, path string) map[string]any {
	t.Helper()
	pathItem, ok := paths[path].(map[string]any)
	if !ok {
		t.Fatalf("path %s = %#v, want an operation", path, paths[path])
	}
	operation, ok := pathItem["get"].(map[string]any)
	if !ok {
		t.Fatalf("path %s = %#v, want GET operation", path, pathItem)
	}
	return operation
}

func hasParameter(operation map[string]any, name string) bool {
	parameters, _ := operation["parameters"].([]any)
	for _, parameter := range parameters {
		definition, ok := parameter.(map[string]any)
		if ok && definition["name"] == name && definition["description"] != "" {
			return true
		}
	}
	return false
}

func containsAll(value string, values ...string) bool {
	for _, expected := range values {
		if !strings.Contains(value, expected) {
			return false
		}
	}
	return true
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
