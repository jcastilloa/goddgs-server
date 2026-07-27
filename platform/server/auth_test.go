package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthenticationAllowsRequestsWhenTokenIsNotConfigured(t *testing.T) {
	engine := authenticatedEngine("")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestAuthenticationAcceptsOnlyMatchingBearerToken(t *testing.T) {
	engine := authenticatedEngine("server-secret")
	tests := []struct {
		name          string
		authorization string
		wantStatus    int
	}{
		{name: "missing token", wantStatus: http.StatusUnauthorized},
		{name: "wrong scheme", authorization: "Basic server-secret", wantStatus: http.StatusUnauthorized},
		{name: "wrong token", authorization: "Bearer different", wantStatus: http.StatusUnauthorized},
		{name: "valid token", authorization: "Bearer server-secret", wantStatus: http.StatusOK},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			if testCase.authorization != "" {
				request.Header.Set("Authorization", testCase.authorization)
			}

			engine.ServeHTTP(recorder, request)
			if recorder.Code != testCase.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, testCase.wantStatus)
			}
		})
	}
}

func authenticatedEngine(token string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(authentication(token))
	engine.GET("/", func(context *gin.Context) { context.Status(http.StatusOK) })
	return engine
}
