package application

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	operations "github.com/jcastilloa/goddgs-server/operations/domain"
	proxyApplication "github.com/jcastilloa/goddgs-server/proxy/application"
	searchApplication "github.com/jcastilloa/goddgs-server/search/application"
	searchDomain "github.com/jcastilloa/goddgs-server/search/domain"
	extractAIDomain "github.com/jcastilloa/goddgs-server/shared/extractai/domain"
)

const MaxTextLength = 512

var sensitiveKey = regexp.MustCompile(`(?i)(authorization|api[_-]?key|token|secret|password|response|prompt|body)`)

type operationContext struct {
	id        string
	startedAt time.Time
}

type operationContextKey struct{}

type EventRecorder struct {
	repository operations.Repository
	now        func() time.Time
	newID      func() string
}

func NewEventRecorder(repository operations.Repository, now func() time.Time, newID func() string) EventRecorder {
	if now == nil {
		now = time.Now
	}
	if newID == nil {
		newID = func() string { return fmt.Sprintf("operation-%d", time.Now().UnixNano()) }
	}
	return EventRecorder{repository: repository, now: now, newID: newID}
}

func (r EventRecorder) StartOperation(ctx context.Context, start operations.OperationStart) (context.Context, error) {
	if r.repository == nil {
		return ctx, nil
	}
	startedAt := r.now().UTC()
	id := r.newID()
	operation := operations.Operation{ID: id, Type: start.Type, Status: operations.StatusRunning, StartedAt: startedAt, HTTPMethod: start.Method, HTTPPath: start.Path, Metadata: SanitizeMetadata(start.Metadata)}
	if err := r.repository.CreateOperation(ctx, operation); err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, operationContextKey{}, operationContext{id: id, startedAt: startedAt}), nil
}

func (r EventRecorder) FinishOperation(ctx context.Context, finish operations.OperationFinish) error {
	if r.repository == nil {
		return nil
	}
	operation, ok := operationFromContext(ctx)
	if !ok {
		return nil
	}
	finishedAt := r.now().UTC()
	result := resultFor(finish.Err)
	if finish.Err == nil && finish.HTTPStatus >= 400 {
		result = operations.ResultFailed
	}
	stored := operations.Operation{
		ID:         operation.id,
		Status:     statusFor(result),
		Result:     result,
		FinishedAt: finishedAt,
		DurationMS: finishedAt.Sub(operation.startedAt).Milliseconds(),
		HTTPStatus: finish.HTTPStatus,
	}
	writeCtx := writeContext(ctx)
	if err := r.repository.FinishOperation(writeCtx, stored); err != nil {
		return err
	}
	return r.recordError(writeCtx, operation.id, "", finish.Err, finishedAt)
}

func (r EventRecorder) StartStep(ctx context.Context, start operations.StepStart) (operations.Step, error) {
	if r.repository == nil {
		return operations.Step{}, nil
	}
	operation, ok := operationFromContext(ctx)
	if !ok {
		return operations.Step{}, nil
	}
	step := operations.Step{ID: r.newID(), OperationID: operation.id, Type: start.Type, Status: operations.StatusRunning, StartedAt: r.now().UTC(), Provider: SanitizeText(start.Provider), Backend: SanitizeText(start.Backend), Proxy: SanitizeText(start.Proxy), Metadata: SanitizeMetadata(start.Metadata)}
	if err := r.repository.AddStep(ctx, step); err != nil {
		return operations.Step{}, err
	}
	return step, nil
}

func (r EventRecorder) FinishStep(ctx context.Context, step operations.Step, stepErr error) error {
	if r.repository == nil || step.ID == "" {
		return nil
	}
	finishedAt := r.now().UTC()
	result := resultFor(stepErr)
	step.Status = statusFor(result)
	step.Result = result
	step.FinishedAt = finishedAt
	step.DurationMS = finishedAt.Sub(step.StartedAt).Milliseconds()
	writeCtx := writeContext(ctx)
	if err := r.repository.FinishStep(writeCtx, step); err != nil {
		return err
	}
	return r.recordError(writeCtx, step.OperationID, step.ID, stepErr, finishedAt)
}

func (r EventRecorder) recordError(ctx context.Context, operationID, stepID string, err error, occurredAt time.Time) error {
	if err == nil {
		return nil
	}
	return r.repository.AddError(ctx, operations.OperationError{OperationID: operationID, StepID: stepID, Category: ClassifyError(err), Message: SanitizeText(err.Error()), OccurredAt: occurredAt})
}

func OperationID(ctx context.Context) string {
	operation, _ := operationFromContext(ctx)
	return operation.id
}

func operationFromContext(ctx context.Context) (operationContext, bool) {
	operation, ok := ctx.Value(operationContextKey{}).(operationContext)
	return operation, ok
}

func resultFor(err error) operations.Result {
	switch ClassifyError(err) {
	case "":
		return operations.ResultSucceeded
	case operations.ErrorCanceled:
		return operations.ResultCanceled
	case operations.ErrorTimeout:
		return operations.ResultTimeout
	default:
		return operations.ResultFailed
	}
}

func writeContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}

func statusFor(result operations.Result) operations.Status {
	if result == operations.ResultSucceeded {
		return operations.StatusSucceeded
	}
	return operations.StatusFailed
}

func ClassifyError(err error) operations.ErrorCategory {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, context.Canceled):
		return operations.ErrorCanceled
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, searchDomain.ErrSearchTimeout):
		return operations.ErrorTimeout
	case errors.Is(err, searchDomain.ErrRateLimited), errors.Is(err, extractAIDomain.ErrRateLimited):
		return operations.ErrorRateLimited
	case errors.Is(err, extractAIDomain.ErrInvalidResponse):
		return operations.ErrorInvalidResponse
	case errors.Is(err, extractAIDomain.ErrUnavailable), errors.Is(err, searchApplication.ErrGatewayUnavailable), errors.Is(err, proxyApplication.ErrNoHealthyProxy):
		return operations.ErrorConfiguration
	}
	var netError net.Error
	var urlError *url.Error
	if errors.As(err, &netError) || errors.As(err, &urlError) {
		return operations.ErrorTransport
	}
	if strings.Contains(strings.ToLower(err.Error()), "status 5") {
		return operations.ErrorUpstream5xx
	}
	return operations.ErrorUnknown
}

func SanitizeText(value string) string {
	value = redactURLs(value)
	value = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*(?:bearer\s+)?)([^\s,;]+)`).ReplaceAllString(value, "$1[redacted]")
	value = regexp.MustCompile(`(?i)((?:api[_-]?key|token|secret|password)\s*[:=]\s*)([^\s,;]+)`).ReplaceAllString(value, "$1[redacted]")
	value = strings.TrimSpace(value)
	if len(value) > MaxTextLength {
		return value[:MaxTextLength]
	}
	return value
}

func SanitizeURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return truncate(scrubSecretTokens(strings.TrimSpace(rawURL)))
	}
	parsed.User = nil
	parsed.Fragment = ""
	query := parsed.Query()
	for key := range query {
		if sensitiveKey.MatchString(key) {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	return truncate(parsed.String())
}

func SanitizeMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	clean := make(map[string]string, len(metadata))
	for key, value := range metadata {
		if sensitiveKey.MatchString(key) {
			continue
		}
		if strings.EqualFold(key, "url") {
			clean[key] = SanitizeURL(value)
			continue
		}
		clean[key] = SanitizeText(value)
	}
	return clean
}

func redactURLs(value string) string {
	return regexp.MustCompile(`https?://[^\s,]+`).ReplaceAllStringFunc(value, SanitizeURL)
}

func truncate(value string) string {
	if len(value) > MaxTextLength {
		return value[:MaxTextLength]
	}
	return value
}

func scrubSecretTokens(value string) string {
	value = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*(?:bearer\s+)?)([^\s,;]+)`).ReplaceAllString(value, "$1[redacted]")
	return regexp.MustCompile(`(?i)((?:api[_-]?key|token|secret|password)\s*[:=]\s*)([^\s,;]+)`).ReplaceAllString(value, "$1[redacted]")
}
