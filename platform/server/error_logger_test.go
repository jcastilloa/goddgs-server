package server

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestErrorLoggerRecordsRequestContextAndCause(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var output bytes.Buffer
	engine := gin.New()
	engine.Use(errorLogger(log.New(&output, "", 0)))
	engine.GET("/v1/text", func(context *gin.Context) {
		_ = context.Error(errors.New("dial upstream: connection refused"))
		context.JSON(http.StatusBadGateway, gin.H{"error": "upstream connection refused"})
	})

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/text?q=go", nil))

	for _, fragment := range []string{
		"method=GET",
		"path=/v1/text",
		"status=502",
		"error=dial upstream: connection refused",
	} {
		if !strings.Contains(output.String(), fragment) {
			t.Errorf("log = %q, want %q", output.String(), fragment)
		}
	}
}
