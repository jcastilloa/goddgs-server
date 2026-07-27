package goddgs

import (
	"context"
	"errors"

	proxyApplication "github.com/jcastilloa/goddgs-server/proxy/application"
	"github.com/jcastilloa/goddgs-server/search/domain"
)

var ErrTransport = errors.New("proxy transport failure")

type Client interface {
	Search(context.Context, domain.SearchRequest) ([]domain.RawResult, error)
	Extract(context.Context, domain.ExtractRequest) (domain.ExtractResult, error)
}

type client = Client

type Gateway struct {
	clients    *proxyApplication.Pool[Client]
	maxRetries int
}

func NewGateway(entries []proxyApplication.Entry[Client], maxRetries int) (*Gateway, error) {
	clients, err := proxyApplication.NewPool(entries)
	if err != nil {
		return nil, err
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	return &Gateway{clients: clients, maxRetries: maxRetries}, nil
}

func (g *Gateway) MarkHealthy(key string) {
	g.clients.MarkHealthy(key)
}

func (g *Gateway) MarkUnhealthy(key string) {
	g.clients.MarkUnhealthy(key)
}

func (g *Gateway) Search(ctx context.Context, request domain.SearchRequest) ([]domain.RawResult, error) {
	var lastErr error
	for attempts := 0; attempts <= g.maxRetries; attempts++ {
		lease, err := g.clients.Select(ctx)
		if err != nil {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, err
		}

		results, err := lease.Value.Search(ctx, request)
		if !errors.Is(err, ErrTransport) {
			return results, err
		}

		g.clients.MarkUnhealthy(lease.Key)
		lastErr = err
	}
	return nil, lastErr
}

func (g *Gateway) Extract(ctx context.Context, request domain.ExtractRequest) (domain.ExtractResult, error) {
	var lastErr error
	for attempts := 0; attempts <= g.maxRetries; attempts++ {
		lease, err := g.clients.Select(ctx)
		if err != nil {
			if lastErr != nil {
				return domain.ExtractResult{}, lastErr
			}
			return domain.ExtractResult{}, err
		}

		result, err := lease.Value.Extract(ctx, request)
		if !errors.Is(err, ErrTransport) {
			return result, err
		}

		g.clients.MarkUnhealthy(lease.Key)
		lastErr = err
	}
	return domain.ExtractResult{}, lastErr
}
