package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	operationsApplication "github.com/jcastilloa/goddgs-server/operations/application"
	operationsDomain "github.com/jcastilloa/goddgs-server/operations/domain"
	"github.com/jcastilloa/goddgs-server/research/domain"
	searchDomain "github.com/jcastilloa/goddgs-server/search/domain"
	configDomain "github.com/jcastilloa/goddgs-server/shared/config/domain"
	extractAIDomain "github.com/jcastilloa/goddgs-server/shared/extractai/domain"
)

func TestNewResearchUseCaseUsesIndependentSelectionAIConfiguration(t *testing.T) {
	var models []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode completion request: %v", err)
		}
		models = append(models, body.Model)
		content := map[string]string{
			"query-model":     `{"queries":[{"language":"en","query":"research query"}]}`,
			"selection-model": `{"candidate_ids":["candidate-1"]}`,
			"report-model":    `{"html":"<p>Report</p>","source_ids":["source-1"]}`,
		}[body.Model]
		if content == "" {
			t.Fatalf("unexpected model %q", body.Model)
		}
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":` + mustJSON(t, content) + `}}]}`))
	}))
	defer server.Close()

	repository := &compositionRepository{}
	identifier := 0
	recorder := operationsApplication.NewEventRecorder(repository, time.Now, func() string {
		identifier++
		return fmt.Sprintf("event-%d", identifier)
	})
	ctx, err := recorder.StartOperation(context.Background(), operationsDomain.OperationStart{Type: operationsDomain.OperationResearch})
	if err != nil {
		t.Fatalf("StartOperation() error = %v", err)
	}
	service := newResearchUseCase(
		researchServerConfig(server.URL),
		compositionSearcher{},
		compositionExtractor{},
		recorder,
	)
	one := 1
	_, err = service.Research(ctx, domain.Request{Query: "topic", QueryCount: &one, ResultsPerQuery: &one})
	if err != nil {
		t.Fatalf("Research() error = %v", err)
	}
	if want := []string{"query-model", "selection-model", "report-model"}; !reflect.DeepEqual(models, want) {
		t.Errorf("completion models = %#v, want %#v", models, want)
	}
	selectionStep, found := findStep(repository.steps, operationsDomain.StepLLMSelection)
	if !found || selectionStep.Provider != "openai-compatible" || selectionStep.Metadata["model"] != "selection-model" {
		t.Errorf("selection step = %#v, found = %v", selectionStep, found)
	}
}

func researchServerConfig(baseURL string) configDomain.ServerConfig {
	return configDomain.ServerConfig{
		LLM: configDomain.LLMConfig{BaseURL: baseURL, APIKey: "test-key"},
		ExtractAI: configDomain.ExtractAIConfig{
			Model: "extract-model", Timeout: time.Second,
		},
		Research: configDomain.ResearchConfig{
			Timeout:                  time.Minute,
			MaxSelectionCandidates:   2,
			MaxSelectedSources:       1,
			MaxConcurrentExtractions: 1,
			QueryAI:                  configDomain.ResearchAIConfig{Model: "query-model", Timeout: time.Second},
			SelectionAI:              configDomain.ResearchAIConfig{Model: "selection-model", Timeout: time.Second},
			ReportAI:                 configDomain.ResearchAIConfig{Model: "report-model", Timeout: time.Second},
		},
	}
}

func mustJSON(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON value: %v", err)
	}
	return string(encoded)
}

type compositionSearcher struct{}

func (compositionSearcher) Search(context.Context, searchDomain.SearchRequest) ([]searchDomain.RawResult, error) {
	return []searchDomain.RawResult{{"href": "https://example.com/source", "title": "Source", "description": "Evidence"}}, nil
}

type compositionExtractor struct{}

func (compositionExtractor) Extract(context.Context, extractAIDomain.Request) (extractAIDomain.Result, error) {
	return extractAIDomain.Result{URL: "https://example.com/source", Content: "<p>Evidence</p>"}, nil
}

func findStep(steps []operationsDomain.Step, stepType operationsDomain.StepType) (operationsDomain.Step, bool) {
	for _, step := range steps {
		if step.Type == stepType {
			return step, true
		}
	}
	return operationsDomain.Step{}, false
}

type compositionRepository struct {
	steps []operationsDomain.Step
}

func (r *compositionRepository) CreateOperation(context.Context, operationsDomain.Operation) error {
	return nil
}
func (r *compositionRepository) FinishOperation(context.Context, operationsDomain.Operation) error {
	return nil
}
func (r *compositionRepository) AddStep(_ context.Context, step operationsDomain.Step) error {
	r.steps = append(r.steps, step)
	return nil
}
func (r *compositionRepository) FinishStep(_ context.Context, step operationsDomain.Step) error {
	for index := range r.steps {
		if r.steps[index].ID == step.ID {
			r.steps[index] = step
		}
	}
	return nil
}
func (r *compositionRepository) AddError(context.Context, operationsDomain.OperationError) error {
	return nil
}
func (r *compositionRepository) RecordProbe(context.Context, operationsDomain.ProxyProbe) error {
	return nil
}
func (r *compositionRepository) RecordHealthTransition(context.Context, operationsDomain.ProxyHealthTransition) error {
	return nil
}
func (r *compositionRepository) ListOperations(context.Context, operationsDomain.OperationQuery) ([]operationsDomain.Operation, error) {
	return nil, nil
}
