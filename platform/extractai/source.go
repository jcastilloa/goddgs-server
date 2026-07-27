package extractai

import (
	"context"
	"fmt"

	searchDomain "github.com/jcastilloa/goddgs-server/search/domain"
	"github.com/jcastilloa/goddgs-server/shared/extractai/domain"
)

type Extractor interface {
	Extract(context.Context, searchDomain.ExtractRequest) (searchDomain.ExtractResult, error)
}

type Source struct {
	extractor Extractor
}

func NewSource(extractor Extractor) Source {
	return Source{extractor: extractor}
}

func (s Source) Fetch(ctx context.Context, request domain.Request) (domain.Page, error) {
	if err := request.Validate(); err != nil {
		return domain.Page{}, err
	}
	if s.extractor == nil {
		return domain.Page{}, domain.ErrUnavailable
	}

	result, err := s.extractor.Extract(ctx, searchDomain.ExtractRequest{URL: request.URL, Format: "content"})
	if err != nil {
		return domain.Page{}, fmt.Errorf("extract source HTML: %w", err)
	}
	html, ok := sourceHTML(result.Content)
	if !ok || html == "" {
		return domain.Page{}, domain.ErrInvalidSource
	}
	return domain.Page{URL: result.URL, HTML: html}, nil
}

func sourceHTML(content any) (string, bool) {
	switch value := content.(type) {
	case []byte:
		return string(value), true
	case string:
		return value, true
	default:
		return "", false
	}
}
