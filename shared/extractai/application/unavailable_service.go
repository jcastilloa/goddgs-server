package application

import (
	"context"
	"fmt"

	"github.com/jcastilloa/goddgs-server/shared/extractai/domain"
)

type UnavailableService struct {
	cause error
}

func NewUnavailableService(cause error) UnavailableService {
	return UnavailableService{cause: cause}
}

func (s UnavailableService) Extract(context.Context, domain.Request) (domain.Result, error) {
	if s.cause == nil {
		return domain.Result{}, domain.ErrUnavailable
	}
	return domain.Result{}, fmt.Errorf("%w: %v", domain.ErrUnavailable, s.cause)
}
