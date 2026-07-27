package application

import (
	"context"
	"errors"

	"github.com/jcastilloa/goddgs-server/search/domain"
)

var ErrGatewayUnavailable = errors.New("search gateway is unavailable")

type Gateway interface {
	Search(context.Context, domain.SearchRequest) ([]domain.RawResult, error)
	Extract(context.Context, domain.ExtractRequest) (domain.ExtractResult, error)
}

type Service struct {
	gateway Gateway
}

func NewService(gateway Gateway) Service {
	return Service{gateway: gateway}
}

func (s Service) Search(ctx context.Context, request domain.SearchRequest) ([]domain.RawResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if s.gateway == nil {
		return nil, ErrGatewayUnavailable
	}
	return s.gateway.Search(ctx, request)
}

func (s Service) Extract(ctx context.Context, request domain.ExtractRequest) (domain.ExtractResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.ExtractResult{}, err
	}
	if err := request.Validate(); err != nil {
		return domain.ExtractResult{}, err
	}
	if s.gateway == nil {
		return domain.ExtractResult{}, ErrGatewayUnavailable
	}
	return s.gateway.Extract(ctx, request)
}
