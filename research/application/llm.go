package application

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jcastilloa/goddgs-server/research/domain"
)

type CompletionModel interface {
	Complete(context.Context, string, string) (string, error)
}

type LLMPlanner struct {
	model   CompletionModel
	retries int
	now     func() time.Time
}

func NewLLMPlanner(model CompletionModel, retries int, now ...func() time.Time) LLMPlanner {
	currentTime := time.Now
	if len(now) > 0 && now[0] != nil {
		currentTime = now[0]
	}
	if retries < 0 {
		retries = 0
	}
	return LLMPlanner{model: model, retries: retries, now: currentTime}
}

func (p LLMPlanner) Plan(ctx context.Context, request domain.NormalizedRequest) ([]domain.GeneratedQuery, error) {
	if p.model == nil {
		return nil, domain.ErrUnavailable
	}
	var lastErr error
	for attempt := 0; attempt <= p.retries; attempt++ {
		response, err := p.model.Complete(ctx, plannerSystemPrompt, plannerUserPrompt(request, p.now()))
		if err != nil {
			lastErr = err
			continue
		}
		var result struct {
			Queries []domain.GeneratedQuery `json:"queries"`
		}
		if err := decodeJSON(response, &result); err != nil {
			lastErr = fmt.Errorf("%w: decode generated queries: %v", domain.ErrInvalidResponse, err)
			continue
		}
		return result.Queries, nil
	}
	return nil, lastErr
}

type LLMReporter struct {
	model CompletionModel
	now   func() time.Time
}

func NewLLMReporter(model CompletionModel, now ...func() time.Time) LLMReporter {
	currentTime := time.Now
	if len(now) > 0 && now[0] != nil {
		currentTime = now[0]
	}
	return LLMReporter{model: model, now: currentTime}
}

func (r LLMReporter) Write(ctx context.Context, request ReportRequest) (Report, error) {
	if r.model == nil {
		return Report{}, domain.ErrUnavailable
	}
	response, err := r.model.Complete(ctx, reporterSystemPrompt, reporterUserPrompt(request, r.now()))
	if err != nil {
		return Report{}, err
	}
	var report Report
	if err := decodeJSON(response, &report); err != nil {
		return Report{}, fmt.Errorf("%w: decode research report: %v", domain.ErrInvalidResponse, err)
	}
	return report, nil
}

func decodeJSON(input string, output any) error {
	decoder := json.NewDecoder(strings.NewReader(stripCodeFence(input)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("unexpected JSON values")
	}
	return nil
}

func stripCodeFence(content string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "```") {
		return content
	}
	firstLineEnd := strings.IndexByte(content, '\n')
	if firstLineEnd == -1 {
		return content
	}
	content = content[firstLineEnd+1:]
	if end := strings.LastIndex(content, "```"); end >= 0 {
		content = content[:end]
	}
	return strings.TrimSpace(content)
}
