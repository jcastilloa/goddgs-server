package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jcastilloa/goddgs-server/search/domain"
	extractAIDomain "github.com/jcastilloa/goddgs-server/shared/extractai/domain"
)

func TestExtractHandlerForwardsURLAndFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useCase := &recordingExtractUseCase{result: domain.ExtractResult{URL: "https://example.com/article", Content: "article body"}}
	handler := NewExtract(useCase, nil)
	engine := gin.New()
	engine.GET("/extract", handler.Handle)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/extract?url=https%3A%2F%2Fexample.com%2Farticle&format=text_plain", nil)
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	want := domain.ExtractRequest{URL: "https://example.com/article", Format: "text_plain"}
	if len(useCase.requests) != 1 || useCase.requests[0] != want {
		t.Errorf("use-case requests = %#v, want %#v", useCase.requests, []domain.ExtractRequest{want})
	}
	var body domain.ExtractResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !reflect.DeepEqual(body, useCase.result) {
		t.Errorf("response = %#v, want %#v", body, useCase.result)
	}
}

func TestExtractHandlerUsesAIOnlyWhenRequested(t *testing.T) {
	gin.SetMode(gin.TestMode)
	heuristic := &recordingExtractUseCase{result: domain.ExtractResult{Content: "heuristic"}}
	ai := &recordingExtractAIUseCase{result: extractAIDomain.Result{URL: "https://example.com/article", Content: "<article>clean</article>"}}
	handler := NewExtract(heuristic, ai)
	engine := gin.New()
	engine.GET("/extract", handler.Handle)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/extract?url=https%3A%2F%2Fexample.com%2Farticle&mode=ai&format=text_plain", nil)
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if len(heuristic.requests) != 0 {
		t.Errorf("heuristic calls = %d, want 0", len(heuristic.requests))
	}
	if len(ai.requests) != 1 || ai.requests[0].URL != "https://example.com/article" {
		t.Errorf("AI requests = %#v", ai.requests)
	}
	var body extractAIDomain.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body != ai.result {
		t.Errorf("body = %#v, want %#v", body, ai.result)
	}
}

func TestExtractHandlerRejectsUnsupportedModeAndReportsUnavailableAI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	heuristic := &recordingExtractUseCase{}
	handler := NewExtract(heuristic, nil)
	engine := gin.New()
	engine.GET("/extract", handler.Handle)

	invalidRecorder := httptest.NewRecorder()
	engine.ServeHTTP(invalidRecorder, httptest.NewRequest(http.MethodGet, "/extract?url=https%3A%2F%2Fexample.com&mode=magic", nil))
	if invalidRecorder.Code != http.StatusBadRequest {
		t.Errorf("invalid mode status = %d, want 400", invalidRecorder.Code)
	}
	if len(heuristic.requests) != 0 {
		t.Errorf("heuristic calls = %d, want 0", len(heuristic.requests))
	}

	aiRecorder := httptest.NewRecorder()
	engine.ServeHTTP(aiRecorder, httptest.NewRequest(http.MethodGet, "/extract?url=https%3A%2F%2Fexample.com&mode=ai", nil))
	if aiRecorder.Code != http.StatusServiceUnavailable {
		t.Errorf("unavailable AI status = %d, want 503", aiRecorder.Code)
	}
	if body := aiRecorder.Body.String(); body != `{"error":"AI extraction is unavailable: configure llm.base_url, llm.api_key, extract_ai.model, extract_ai.timeout, extract_ai.temperature, and extract_ai.retries"}` {
		t.Errorf("unavailable AI body = %s", body)
	}
}

func TestExtractHandlerRejectsMissingOrUnsupportedURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useCase := &recordingExtractUseCase{}
	handler := NewExtract(useCase, nil)
	engine := gin.New()
	engine.GET("/extract", handler.Handle)

	for _, rawURL := range []string{"/extract", "/extract?url=file%3A%2F%2F%2Ftmp%2Fpage.html"} {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, rawURL, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("GET %s status = %d, want 400", rawURL, recorder.Code)
		}
	}
	if len(useCase.requests) != 0 {
		t.Errorf("use-case calls = %d, want 0", len(useCase.requests))
	}
}

func TestExtractHandlerMapsInvalidUseCaseRequestToBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewExtract(&recordingExtractUseCase{err: domain.ErrInvalidExtractRequest}, nil)
	engine := gin.New()
	engine.GET("/extract", handler.Handle)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/extract?url=https%3A%2F%2Fexample.com", nil)
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", recorder.Code)
	}
}

type recordingExtractUseCase struct {
	result   domain.ExtractResult
	err      error
	requests []domain.ExtractRequest
}

type recordingExtractAIUseCase struct {
	result   extractAIDomain.Result
	err      error
	requests []extractAIDomain.Request
}

func (u *recordingExtractAIUseCase) Extract(_ context.Context, request extractAIDomain.Request) (extractAIDomain.Result, error) {
	u.requests = append(u.requests, request)
	return u.result, u.err
}

func (u *recordingExtractUseCase) Extract(_ context.Context, request domain.ExtractRequest) (domain.ExtractResult, error) {
	u.requests = append(u.requests, request)
	return u.result, u.err
}
