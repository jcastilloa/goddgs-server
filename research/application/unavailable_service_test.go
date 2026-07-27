package application

import (
	"context"
	"errors"
	"testing"

	"github.com/jcastilloa/goddgs-server/research/domain"
)

func TestUnavailableServiceExplainsWhyResearchCannotRun(t *testing.T) {
	service := NewUnavailableService(errors.New("research.query_ai.model is required"))

	_, err := service.Research(context.Background(), domain.Request{Query: "topic"})
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("Research() error = %v, want ErrUnavailable", err)
	}
	if got := err.Error(); got != "research is unavailable: research.query_ai.model is required" {
		t.Errorf("error = %q", got)
	}
}
