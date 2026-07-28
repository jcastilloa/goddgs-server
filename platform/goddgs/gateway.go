package goddgs

import (
	"context"
	"errors"
	"fmt"

	operationsApplication "github.com/jcastilloa/goddgs-server/operations/application"
	operations "github.com/jcastilloa/goddgs-server/operations/domain"
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
	recorder   *operationsApplication.EventRecorder
}

func NewGateway(entries []proxyApplication.Entry[Client], maxRetries int, recorders ...operationsApplication.EventRecorder) (*Gateway, error) {
	clients, err := proxyApplication.NewPool(entries)
	if err != nil {
		return nil, err
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	var recorder *operationsApplication.EventRecorder
	if len(recorders) > 0 {
		recorder = &recorders[0]
	}
	return &Gateway{clients: clients, maxRetries: maxRetries, recorder: recorder}, nil
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

		step := g.startSearchStep(ctx, request, lease.Key)
		results, err := lease.Value.Search(ctx, request)
		if step.ID != "" {
			step.Metadata = operationsApplication.SanitizeMetadata(map[string]string{
				"query":        request.Query,
				"result_count": fmt.Sprintf("%d", len(results)),
			})
			_ = g.recorder.FinishStep(ctx, step, err)
		}
		if !errors.Is(err, ErrTransport) {
			return results, err
		}

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

		step := g.startExtractStep(ctx, request, lease.Key)
		result, err := lease.Value.Extract(ctx, request)
		if step.ID != "" {
			step.Metadata = operationsApplication.SanitizeMetadata(map[string]string{
				"url":    request.URL,
				"format": request.Format,
			})
			_ = g.recorder.FinishStep(ctx, step, err)
		}
		if !errors.Is(err, ErrTransport) {
			return result, err
		}

		lastErr = err
	}
	return domain.ExtractResult{}, lastErr
}

func (g *Gateway) startSearchStep(ctx context.Context, request domain.SearchRequest, proxy string) operations.Step {
	if g.recorder == nil {
		return operations.Step{}
	}
	step, _ := g.recorder.StartStep(ctx, operations.StepStart{
		Type:    operations.StepSearch,
		Backend: request.Backend,
		Proxy:   proxy,
		Metadata: map[string]string{
			"query":    request.Query,
			"category": string(request.Category),
			"region":   request.Region,
		},
	})
	return step
}

func (g *Gateway) startExtractStep(ctx context.Context, request domain.ExtractRequest, proxy string) operations.Step {
	if g.recorder == nil {
		return operations.Step{}
	}
	step, _ := g.recorder.StartStep(ctx, operations.StepStart{
		Type:  operations.StepExtractHeuristic,
		Proxy: proxy,
		Metadata: map[string]string{
			"url":    request.URL,
			"format": request.Format,
			"mode":   string(request.Mode.Normalize()),
		},
	})
	return step
}
