package application

import (
	"context"
	"errors"
	"testing"

	"github.com/jcastilloa/goddgs-server/shared/extractai/domain"
)

func TestUnavailableServiceExplainsWhyAIExtractionCannotRun(t *testing.T) {
	service := NewUnavailableService(errors.New("llm.api_key is required"))

	_, err := service.Extract(context.Background(), domain.Request{URL: "https://example.com/article"})
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("Extract() error = %v, want ErrUnavailable", err)
	}
	if got := err.Error(); got != "AI extraction is unavailable: llm.api_key is required" {
		t.Errorf("error = %q", got)
	}
}
