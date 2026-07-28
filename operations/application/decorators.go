package application

import (
	"context"
	"fmt"

	operations "github.com/jcastilloa/goddgs-server/operations/domain"
	extractAIDomain "github.com/jcastilloa/goddgs-server/shared/extractai/domain"
)

type completionModel interface {
	Complete(context.Context, string, string) (string, error)
}

type CompletionModelRecorder struct {
	next     completionModel
	recorder EventRecorder
	stepType operations.StepType
	provider string
	model    string
}

func NewCompletionModelRecorder(next completionModel, recorder EventRecorder, stepType operations.StepType, provider, model string) CompletionModelRecorder {
	return CompletionModelRecorder{
		next:     next,
		recorder: recorder,
		stepType: stepType,
		provider: provider,
		model:    model,
	}
}

func (r CompletionModelRecorder) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if r.next == nil {
		return "", fmt.Errorf("%w: completion model is unavailable", extractAIDomain.ErrUnavailable)
	}
	step, _ := r.recorder.StartStep(ctx, operations.StepStart{
		Type:     r.stepType,
		Provider: r.provider,
		Metadata: map[string]string{"model": r.model},
	})
	content, err := r.next.Complete(ctx, systemPrompt, userPrompt)
	_ = r.recorder.FinishStep(ctx, step, err)
	return content, err
}

type sourceFetcher interface {
	Fetch(context.Context, extractAIDomain.Request) (extractAIDomain.Page, error)
}

type SourceRecorder struct {
	next     sourceFetcher
	recorder EventRecorder
}

func NewSourceRecorder(next sourceFetcher, recorder EventRecorder) SourceRecorder {
	return SourceRecorder{next: next, recorder: recorder}
}

func (r SourceRecorder) Fetch(ctx context.Context, request extractAIDomain.Request) (extractAIDomain.Page, error) {
	if r.next == nil {
		return extractAIDomain.Page{}, fmt.Errorf("%w: source is unavailable", extractAIDomain.ErrUnavailable)
	}
	step, _ := r.recorder.StartStep(ctx, operations.StepStart{
		Type:     operations.StepExtractHeuristic,
		Metadata: map[string]string{"url": request.URL},
	})
	page, err := r.next.Fetch(ctx, request)
	_ = r.recorder.FinishStep(ctx, step, err)
	return page, err
}

type extractor interface {
	Extract(context.Context, extractAIDomain.Request) (extractAIDomain.Result, error)
}

type ExtractorRecorder struct {
	next     extractor
	recorder EventRecorder
}

func NewExtractorRecorder(next extractor, recorder EventRecorder) ExtractorRecorder {
	return ExtractorRecorder{next: next, recorder: recorder}
}

func (r ExtractorRecorder) Extract(ctx context.Context, request extractAIDomain.Request) (extractAIDomain.Result, error) {
	if r.next == nil {
		return extractAIDomain.Result{}, fmt.Errorf("%w: extractor is unavailable", extractAIDomain.ErrUnavailable)
	}
	step, _ := r.recorder.StartStep(ctx, operations.StepStart{
		Type:     operations.StepExtractAI,
		Metadata: map[string]string{"url": request.URL},
	})
	result, err := r.next.Extract(ctx, request)
	_ = r.recorder.FinishStep(ctx, step, err)
	return result, err
}
