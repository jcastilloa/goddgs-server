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
	if !ok || paths["/v1/text"] == nil || paths["/v1/extract"] == nil || paths["/v1/research"] == nil || paths["/operations"] == nil || paths["/operations/api/summary"] == nil || paths["/operations/api/timeseries"] == nil || paths["/operations/api/operations"] == nil || paths["/operations/api/operations/{id}"] == nil || paths["/operations/api/proxies"] == nil {
		t.Errorf("paths = %#v, want versioned API and operations dashboard paths", specification["paths"])
	}
	if paths["/v1/hello"] != nil {
		t.Errorf("paths = %#v, must not expose removed /v1/hello endpoint", paths)
	}
	extractPath := paths["/v1/extract"].(map[string]any)
	extractGet := extractPath["get"].(map[string]any)
	if description := extractGet["description"].(string); description == "" || !containsAll(description, "mode=heuristic", "mode=ai", "llm.base_url", "format=html", "service.request_timeout", "extract_ai.timeout", "additional attempts", "attempt timeouts") {
		t.Errorf("extract description = %q", description)
	}
	parameters := extractGet["parameters"].([]any)
	if parameters[2].(map[string]any)["name"] != "mode" {
		t.Errorf("extract parameters = %#v, want mode", parameters)
	}
	if parameters[0].(map[string]any)["description"] == "" || parameters[1].(map[string]any)["description"] == "" || parameters[2].(map[string]any)["description"] == "" {
		t.Errorf("extract parameters need descriptions = %#v", parameters)
	}
	format := parameters[1].(map[string]any)
	formats := format["schema"].(map[string]any)["enum"].([]any)
	if !containsString(formats, "html") {
		t.Errorf("extract formats = %#v, want html", formats)
	}
	responses := extractGet["responses"].(map[string]any)
	if responses["503"].(map[string]any)["description"] != "AI extraction is not configured or unavailable. Heuristic extraction remains available." {
		t.Errorf("extract responses = %#v", responses)
	}
	if responses["200"].(map[string]any)["content"] == nil || responses["400"].(map[string]any)["content"] == nil || responses["503"].(map[string]any)["content"] == nil {
		t.Errorf("extract responses need documented payloads = %#v", responses)
	}
	assertDetailedDocumentation(t, paths)
	assertResearchDocumentation(t, paths)
	assertOperationsDocumentation(t, paths)

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

func assertOperationsDocumentation(t *testing.T, paths map[string]any) {
	t.Helper()
	dashboard := documentedGetOperation(t, paths, "/operations")
	if !hasOperationsSessionSecurity(dashboard) || dashboard["responses"].(map[string]any)["200"] == nil || dashboard["responses"].(map[string]any)["303"] == nil {
		t.Errorf("dashboard operation = %#v, want authenticated HTML contract", dashboard)
	}
	for _, path := range []string{"/operations/api/summary", "/operations/api/timeseries", "/operations/api/operations", "/operations/api/operations/{id}", "/operations/api/proxies"} {
		operation := documentedGetOperation(t, paths, path)
		if !hasOperationsSessionSecurity(operation) || operation["description"] == "" {
			t.Errorf("%s operation = %#v, want cookie security and description", path, operation)
		}
		responses := operation["responses"].(map[string]any)
		for _, status := range []string{"200", "400", "401", "500"} {
			if responses[status] == nil || responses[status].(map[string]any)["content"] == nil {
				t.Errorf("%s responses = %#v, missing %s", path, responses, status)
			}
		}
	}
	operations := documentedGetOperation(t, paths, "/operations/api/operations")
	for _, parameter := range []string{"range", "from", "to", "status", "type", "limit", "offset"} {
		if !hasParameter(operations, parameter) {
			t.Errorf("operations parameters = %#v, want %s", operations["parameters"], parameter)
		}
	}
	timeseries := documentedGetOperation(t, paths, "/operations/api/timeseries")
	if !hasParameter(timeseries, "interval") {
		t.Errorf("time series parameters = %#v, want interval", timeseries["parameters"])
	}
	detail := documentedGetOperation(t, paths, "/operations/api/operations/{id}")
	if detail["responses"].(map[string]any)["404"] == nil {
		t.Errorf("detail responses = %#v, want 404", detail["responses"])
	}
	for _, path := range []string{"/operations/setup", "/operations/login"} {
		operation := documentedGetOperation(t, paths, path)
		if !hasEmptySecurity(operation) || operation["responses"].(map[string]any)["303"] == nil {
			t.Errorf("%s = %#v, want public page and redirection", path, operation)
		}
	}
	for _, path := range []string{"/operations/api/auth/setup", "/operations/api/auth/login"} {
		operation := documentedPostOperation(t, paths, path)
		if !hasEmptySecurity(operation) || !hasDashboardCredentialRequestBody(operation) || !hasSetCookieHeader(operation) {
			t.Errorf("%s operation = %#v, want public credentials contract and session cookies", path, operation)
		}
	}
	for _, path := range []string{"/operations/api/auth/setup", "/operations/api/auth/login", "/operations/api/auth/session", "/operations/api/auth/logout", "/operations/api/auth/password"} {
		pathItem, ok := paths[path].(map[string]any)
		if !ok {
			t.Errorf("missing auth path %s", path)
			continue
		}
		method := "post"
		if path == "/operations/api/auth/session" {
			method = "get"
		}
		operation, ok := pathItem[method].(map[string]any)
		if !ok {
			t.Errorf("%s = %#v, want %s operation", path, pathItem, method)
			continue
		}
		if operation["description"] == "" || operation["responses"] == nil {
			t.Errorf("%s operation = %#v", path, operation)
		}
	}
	for _, path := range []string{"/operations/api/auth/session", "/operations/api/auth/logout", "/operations/api/auth/password"} {
		method := "post"
		if path == "/operations/api/auth/session" {
			method = "get"
		}
		operation := paths[path].(map[string]any)[method].(map[string]any)
		if !hasOperationsSessionSecurity(operation) || operation["responses"].(map[string]any)["401"] == nil {
			t.Errorf("%s operation = %#v, want protected session contract", path, operation)
		}
	}
	for _, path := range []string{"/operations/api/auth/logout", "/operations/api/auth/password"} {
		operation := documentedPostOperation(t, paths, path)
		if !hasParameter(operation, "X-Operations-CSRF") || operation["responses"].(map[string]any)["403"] == nil {
			t.Errorf("%s operation = %#v, want CSRF contract", path, operation)
		}
	}
}

func documentedPostOperation(t *testing.T, paths map[string]any, path string) map[string]any {
	t.Helper()
	pathItem, ok := paths[path].(map[string]any)
	if !ok {
		t.Fatalf("path %s = %#v", path, paths[path])
	}
	operation, ok := pathItem["post"].(map[string]any)
	if !ok {
		t.Fatalf("post %s = %#v", path, pathItem)
	}
	return operation
}

func hasDashboardCredentialRequestBody(operation map[string]any) bool {
	requestBody, ok := operation["requestBody"].(map[string]any)
	if !ok {
		return false
	}
	content, ok := requestBody["content"].(map[string]any)
	if !ok {
		return false
	}
	jsonContent, ok := content["application/json"].(map[string]any)
	if !ok {
		return false
	}
	schema, ok := jsonContent["schema"].(map[string]any)
	if !ok {
		return false
	}
	properties, ok := schema["properties"].(map[string]any)
	return ok && properties["username"] != nil && properties["password"] != nil
}

func hasSetCookieHeader(operation map[string]any) bool {
	responses, ok := operation["responses"].(map[string]any)
	if !ok {
		return false
	}
	for _, status := range []string{"200", "201"} {
		response, ok := responses[status].(map[string]any)
		if !ok {
			continue
		}
		headers, ok := response["headers"].(map[string]any)
		return ok && headers["Set-Cookie"] != nil
	}
	return false
}

func hasOperationsSessionSecurity(operation map[string]any) bool {
	security, ok := operation["security"].([]any)
	if !ok || len(security) != 1 {
		return false
	}
	requirement, ok := security[0].(map[string]any)
	return ok && requirement["operationsSession"] != nil
}

func hasEmptySecurity(operation map[string]any) bool {
	security, ok := operation["security"].([]any)
	return ok && len(security) == 0
}

func assertResearchDocumentation(t *testing.T, paths map[string]any) {
	t.Helper()
	path, ok := paths["/v1/research"].(map[string]any)
	if !ok {
		t.Fatalf("research path = %#v", paths["/v1/research"])
	}
	operation, ok := path["post"].(map[string]any)
	if !ok || operation["summary"] == "" || operation["description"] == "" {
		t.Fatalf("research operation = %#v", path)
	}
	description := operation["description"].(string)
	if !containsAll(description, "query_count × results_per_query", "candidate ID, title, description, and URL", "not crawled", "research.selection_ai.*", "research.max_selection_candidates", "research.max_selected_sources", "source_selection_ms", "candidates_found", "candidates_selected", "timeout applies to an attempt", "additional retryable attempts") {
		t.Errorf("research description = %q", description)
	}
	requestBody := operation["requestBody"].(map[string]any)
	if requestBody["required"] != true {
		t.Errorf("request body = %#v, want required", requestBody)
	}
	content := requestBody["content"].(map[string]any)["application/json"].(map[string]any)
	schema := content["schema"].(map[string]any)
	properties := schema["properties"].(map[string]any)
	for _, field := range []string{"query", "report_language", "query_languages", "query_count", "results_per_query", "region"} {
		if properties[field] == nil {
			t.Errorf("research properties = %#v, missing %s", properties, field)
		}
	}
	if properties["report_language"].(map[string]any)["default"] != "en" || properties["query_count"].(map[string]any)["default"] != float64(10) || properties["results_per_query"].(map[string]any)["default"] != float64(10) {
		t.Errorf("research defaults = %#v", properties)
	}
	responses := operation["responses"].(map[string]any)
	for _, status := range []string{"200", "400", "401", "429", "499", "502", "503", "504"} {
		if responses[status] == nil || responses[status].(map[string]any)["content"] == nil {
			t.Errorf("research responses = %#v, missing documented %s", responses, status)
		}
	}
	responseSchema := responses["200"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	diagnostics := responseSchema["properties"].(map[string]any)["diagnostics"].(map[string]any)
	for _, field := range []string{"backends", "query_planning_ms", "search_ms", "source_selection_ms", "source_extraction_ms", "report_generation_ms", "total_ms", "candidates_found", "candidates_selected"} {
		if diagnostics["properties"].(map[string]any)[field] == nil {
			t.Errorf("research diagnostics = %#v, missing %s", diagnostics, field)
		}
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

func containsString(values []any, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
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
