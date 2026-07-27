package application

import (
	"context"
	"fmt"

	"github.com/jcastilloa/goddgs-server/shared/extractai/domain"
)

type Source interface {
	Fetch(context.Context, domain.Request) (domain.Page, error)
}

type Model interface {
	Complete(context.Context, string, string) (string, error)
}

type Service struct {
	source Source
	model  Model
}

func NewService(source Source, model Model) Service {
	return Service{source: source, model: model}
}

func (s Service) Extract(ctx context.Context, request domain.Request) (domain.Result, error) {
	if err := ctx.Err(); err != nil {
		return domain.Result{}, err
	}
	if err := request.Validate(); err != nil {
		return domain.Result{}, err
	}
	if s.source == nil || s.model == nil {
		return domain.Result{}, domain.ErrUnavailable
	}

	page, err := s.source.Fetch(ctx, request)
	if err != nil {
		return domain.Result{}, fmt.Errorf("fetch source page: %w", err)
	}
	sourceURL := page.URL
	if sourceURL == "" {
		sourceURL = request.URL
	}
	content, err := s.model.Complete(ctx, systemPrompt, userPrompt(page.HTML, sourceURL))
	if err != nil {
		return domain.Result{}, fmt.Errorf("extract primary content with AI: %w", err)
	}
	cleanHTML, err := sanitizeHTML(content)
	if err != nil {
		return domain.Result{}, err
	}

	return domain.Result{URL: sourceURL, Content: cleanHTML}, nil
}
