package search

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jcastilloa/goddgs-server/search/domain"
)

func TestGetHandlerForwardsQueryParametersAndPreservesResults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	maxResults := 3
	page := 2
	useCase := &recordingSearchUseCase{results: []domain.RawResult{{"title": "result", "rank": 12, "nested": map[string]any{"value": nil}}}}
	handler := NewGet(domain.CategoryImages, useCase)
	engine := gin.New()
	engine.GET("/images", handler.Handle)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/images?q=mountain&region=es-es&safesearch=off&timelimit=w&max_results=3&page=2&backend=bing&size=Large&color=Blue&type_image=photo&layout=Wide&license_image=Share", nil)
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	want := domain.SearchRequest{
		Category: domain.CategoryImages, Query: "mountain", Region: "es-es", SafeSearch: "off", TimeLimit: "w", MaxResults: &maxResults, Page: &page, Backend: "bing",
		Images: domain.ImageOptions{Size: "Large", Color: "Blue", Type: "photo", Layout: "Wide", License: "Share"},
	}
	if len(useCase.requests) != 1 || !reflect.DeepEqual(useCase.requests[0], want) {
		t.Errorf("use-case requests = %#v, want %#v", useCase.requests, []domain.SearchRequest{want})
	}

	var body []map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body[0]["title"] != "result" || body[0]["rank"] != float64(12) {
		t.Errorf("response = %#v", body)
	}
}

func TestGetHandlerRejectsInvalidParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useCase := &recordingSearchUseCase{}
	handler := NewGet(domain.CategoryText, useCase)
	engine := gin.New()
	engine.GET("/text", handler.Handle)

	for _, rawURL := range []string{"/text", "/text?q=query&max_results=bad", "/text?q=query&page=0"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, rawURL, nil)
		engine.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("GET %s status = %d, want 400", rawURL, recorder.Code)
		}
	}
	if len(useCase.requests) != 0 {
		t.Errorf("use-case calls = %d, want 0", len(useCase.requests))
	}
}

func TestGetHandlerMapsContextTimeoutAndUnexpectedError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "timeout", err: context.DeadlineExceeded, wantStatus: http.StatusGatewayTimeout},
		{name: "source timeout", err: domain.ErrSearchTimeout, wantStatus: http.StatusGatewayTimeout},
		{name: "rate limit", err: domain.ErrRateLimited, wantStatus: http.StatusTooManyRequests},
		{name: "failure", err: errors.New("source unavailable"), wantStatus: http.StatusBadGateway},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			useCase := &recordingSearchUseCase{err: testCase.err}
			handler := NewGet(domain.CategoryText, useCase)
			engine := gin.New()
			engine.GET("/text", handler.Handle)
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/text?q=query", nil))
			if recorder.Code != testCase.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, testCase.wantStatus)
			}
		})
	}
}

type recordingSearchUseCase struct {
	results  []domain.RawResult
	err      error
	requests []domain.SearchRequest
}

func (u *recordingSearchUseCase) Search(_ context.Context, request domain.SearchRequest) ([]domain.RawResult, error) {
	u.requests = append(u.requests, request)
	return u.results, u.err
}
