package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	operations "github.com/jcastilloa/goddgs-server/operations/domain"
	searchDomain "github.com/jcastilloa/goddgs-server/search/domain"
	extractAIDomain "github.com/jcastilloa/goddgs-server/shared/extractai/domain"
)

func TestSanitizeTextRemovesSecretsAndBoundsItsSize(t *testing.T) {
	input := "Authorization: Bearer super-secret-token api_key=also-secret https://user:password@example.com/path?token=query-secret"
	got := SanitizeText(input)
	for _, secret := range []string{"super-secret-token", "also-secret", "password", "query-secret"} {
		if strings.Contains(got, secret) {
			t.Errorf("SanitizeText() = %q, contains %q", got, secret)
		}
	}

	if got := SanitizeText(strings.Repeat("x", MaxTextLength+1)); len(got) > MaxTextLength {
		t.Errorf("SanitizeText() length = %d, want at most %d", len(got), MaxTextLength)
	}
}

func TestSanitizeURLRemovesCredentialsFragmentsAndSensitiveQueryValues(t *testing.T) {
	got := SanitizeURL("https://user:password@example.com/path?query=topic&api_key=secret#private")
	if got != "https://example.com/path?query=topic" {
		t.Errorf("SanitizeURL() = %q", got)
	}
}

func TestSanitizeMetadataDoesNotPersistSensitiveOrOversizedValues(t *testing.T) {
	got := SanitizeMetadata(map[string]string{
		"query":         "useful query",
		"authorization": "Bearer secret",
		"response":      "full response",
		"url":           "https://user:password@example.com/path",
		"long":          strings.Repeat("x", MaxTextLength+1),
	})
	if got["query"] != "useful query" || got["url"] != "https://example.com/path" {
		t.Errorf("SanitizeMetadata() = %#v", got)
	}
	for _, key := range []string{"authorization", "response"} {
		if _, exists := got[key]; exists {
			t.Errorf("SanitizeMetadata() persisted %q", key)
		}
	}
	if len(got["long"]) > MaxTextLength {
		t.Errorf("SanitizeMetadata() long value length = %d", len(got["long"]))
	}
}

func TestClassifyErrorUsesStableCategories(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want operations.ErrorCategory
	}{
		{name: "canceled", err: context.Canceled, want: operations.ErrorCanceled},
		{name: "timeout", err: context.DeadlineExceeded, want: operations.ErrorTimeout},
		{name: "rate limited", err: extractAIDomain.ErrRateLimited, want: operations.ErrorRateLimited},
		{name: "search timeout", err: searchDomain.ErrSearchTimeout, want: operations.ErrorTimeout},
		{name: "invalid response", err: extractAIDomain.ErrInvalidResponse, want: operations.ErrorInvalidResponse},
		{name: "configuration", err: extractAIDomain.ErrUnavailable, want: operations.ErrorConfiguration},
		{name: "unknown", err: errors.New("provider broke"), want: operations.ErrorUnknown},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := ClassifyError(testCase.err); got != testCase.want {
				t.Errorf("ClassifyError() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestEventRecorderCorrelatesOperationsAndSteps(t *testing.T) {
	repository := &recordingRepository{}
	recorder := NewEventRecorder(repository, func() time.Time { return time.Date(2026, time.July, 28, 10, 0, 0, 0, time.UTC) }, func() string { return "operation-1" })

	ctx, err := recorder.StartOperation(context.Background(), operations.OperationStart{Type: operations.OperationSearch, Method: "GET", Path: "/v1/text"})
	if err != nil {
		t.Fatalf("StartOperation() error = %v", err)
	}
	stepID, err := recorder.StartStep(ctx, operations.StepStart{Type: operations.StepSearch, Metadata: map[string]string{"query": "topic"}})
	if err != nil {
		t.Fatalf("StartStep() error = %v", err)
	}
	if err := recorder.FinishStep(ctx, stepID, nil); err != nil {
		t.Fatalf("FinishStep() error = %v", err)
	}
	if err := recorder.FinishOperation(ctx, httpSuccess(200)); err != nil {
		t.Fatalf("FinishOperation() error = %v", err)
	}

	if len(repository.operations) != 1 || repository.operations[0].ID != "operation-1" || repository.operations[0].Status != operations.StatusSucceeded {
		t.Errorf("operations = %#v", repository.operations)
	}
	if len(repository.steps) != 1 || repository.steps[0].OperationID != "operation-1" || repository.steps[0].Status != operations.StatusSucceeded {
		t.Errorf("steps = %#v", repository.steps)
	}
}

func TestEventRecorderSanitizesMetadataAddedBeforeFinishingStep(t *testing.T) {
	repository := &recordingRepository{}
	recorder := NewEventRecorder(repository, time.Now, func() string { return "operation-1" })
	ctx, err := recorder.StartOperation(context.Background(), operations.OperationStart{Type: operations.OperationResearch})
	if err != nil {
		t.Fatalf("StartOperation() error = %v", err)
	}
	step, err := recorder.StartStep(ctx, operations.StepStart{Type: operations.StepResearchSelection, Metadata: map[string]string{"candidates_found": "2"}})
	if err != nil {
		t.Fatalf("StartStep() error = %v", err)
	}
	step.Metadata = map[string]string{
		"candidates_found":    "2",
		"candidates_selected": "1",
		"url":                 "https://user:password@example.com/source",
		"response":            "full completion response",
	}
	if err := recorder.FinishStep(ctx, step, errors.New("selector response was invalid")); err != nil {
		t.Fatalf("FinishStep() error = %v", err)
	}

	got := repository.steps[0].Metadata
	if got["candidates_found"] != "2" || got["candidates_selected"] != "1" || got["url"] != "https://example.com/source" {
		t.Errorf("metadata = %#v", got)
	}
	if _, exists := got["response"]; exists {
		t.Errorf("metadata = %#v, persisted completion response", got)
	}
}

func httpSuccess(status int) operations.OperationFinish {
	return operations.OperationFinish{HTTPStatus: status}
}

type recordingRepository struct {
	operations []operations.Operation
	steps      []operations.Step
}

func (r *recordingRepository) CreateOperation(_ context.Context, operation operations.Operation) error {
	r.operations = append(r.operations, operation)
	return nil
}
func (r *recordingRepository) FinishOperation(_ context.Context, operation operations.Operation) error {
	r.operations[0] = operation
	return nil
}
func (r *recordingRepository) AddStep(_ context.Context, step operations.Step) error {
	r.steps = append(r.steps, step)
	return nil
}
func (r *recordingRepository) FinishStep(_ context.Context, step operations.Step) error {
	r.steps[0] = step
	return nil
}
func (r *recordingRepository) AddError(context.Context, operations.OperationError) error { return nil }
func (r *recordingRepository) RecordProbe(context.Context, operations.ProxyProbe) error  { return nil }
func (r *recordingRepository) RecordHealthTransition(context.Context, operations.ProxyHealthTransition) error {
	return nil
}
func (r *recordingRepository) ListOperations(context.Context, operations.OperationQuery) ([]operations.Operation, error) {
	return nil, nil
}
