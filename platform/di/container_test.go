package di

import (
	"testing"

	searchApplication "github.com/jcastilloa/goddgs-server/search/application"
)

func TestBuildRequiresDashboardUseCase(t *testing.T) {
	container := New("test", searchApplication.Service{}, nil, nil, nil)

	if _, err := container.Build(); err == nil {
		t.Fatal("Build() error = nil, want missing dashboard use case error")
	}
}
