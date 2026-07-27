package research

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jcastilloa/goddgs-server/research/domain"
	extractAIDomain "github.com/jcastilloa/goddgs-server/shared/extractai/domain"
)

func TestPostHandlerUsesDefaultsAndReturnsResearch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useCase := &recordingUseCase{result: domain.Result{ReportHTML: "<article><p>Report</p></article>", Sources: []domain.Source{{URL: "https://example.com", Title: "Example"}}}}
	engine := gin.New()
	engine.POST("/research", NewPost(useCase).Handle)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/research", strings.NewReader(`{"query":"E.T. opening box office"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if len(useCase.requests) != 1 || useCase.requests[0].Query != "E.T. opening box office" || useCase.requests[0].QueryCount != nil {
		t.Errorf("requests = %#v", useCase.requests)
	}
	var result domain.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !reflect.DeepEqual(result, useCase.result) {
		t.Errorf("result = %#v", result)
	}
}

func TestPostHandlerRejectsMalformedAndInvalidRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	useCase := &recordingUseCase{}
	engine := gin.New()
	engine.POST("/research", NewPost(useCase).Handle)

	for _, body := range []string{"", `{`, `{"query":"topic","unknown":true}`, `{"query":" "}`, `{"query":"topic","query_count":0}`, `{"query":"topic"}{"query":"other"}`} {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/research", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("body %q status = %d, want 400; response = %s", body, recorder.Code, recorder.Body.String())
		}
	}
	if len(useCase.requests) != 0 {
		t.Errorf("use-case calls = %d, want 0", len(useCase.requests))
	}
}

func TestPostHandlerMapsResearchErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "rate limit", err: extractAIDomain.ErrRateLimited, want: http.StatusTooManyRequests},
		{name: "unavailable", err: domain.ErrUnavailable, want: http.StatusServiceUnavailable},
		{name: "timeout", err: context.DeadlineExceeded, want: http.StatusGatewayTimeout},
		{name: "failure", err: errors.New("provider failed"), want: http.StatusBadGateway},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			engine := gin.New()
			engine.POST("/research", NewPost(&recordingUseCase{err: testCase.err}).Handle)
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/research", strings.NewReader(`{"query":"topic"}`)))
			if recorder.Code != testCase.want {
				t.Errorf("status = %d, want %d", recorder.Code, testCase.want)
			}
		})
	}
}

type recordingUseCase struct {
	requests []domain.Request
	result   domain.Result
	err      error
}

func (u *recordingUseCase) Research(_ context.Context, request domain.Request) (domain.Result, error) {
	u.requests = append(u.requests, request)
	return u.result, u.err
}
