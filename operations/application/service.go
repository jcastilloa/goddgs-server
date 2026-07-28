package application

import (
	"context"

	operations "github.com/jcastilloa/goddgs-server/operations/domain"
)

type Recorder struct {
	repository operations.Repository
}

func NewRecorder(repository operations.Repository) Recorder {
	return Recorder{repository: repository}
}

func (r Recorder) RecordOperation(ctx context.Context, operation operations.Operation) error {
	return r.repository.CreateOperation(ctx, operation)
}

func (r Recorder) RecordStep(ctx context.Context, step operations.Step) error {
	return r.repository.AddStep(ctx, step)
}

func (r Recorder) RecordError(ctx context.Context, operationError operations.OperationError) error {
	return r.repository.AddError(ctx, operationError)
}

func (r Recorder) RecordProbe(ctx context.Context, probe operations.ProxyProbe) error {
	return r.repository.RecordProbe(ctx, probe)
}

func (r Recorder) RecordHealthTransition(ctx context.Context, transition operations.ProxyHealthTransition) error {
	return r.repository.RecordHealthTransition(ctx, transition)
}

type QueryService struct {
	repository operations.Repository
}

func NewQueryService(repository operations.Repository) QueryService {
	return QueryService{repository: repository}
}

func (s QueryService) ListOperations(ctx context.Context, query operations.OperationQuery) ([]operations.Operation, error) {
	return s.repository.ListOperations(ctx, query)
}

type DashboardService struct {
	repository operations.DashboardRepository
}

func NewDashboardService(repository operations.DashboardRepository) *DashboardService {
	return &DashboardService{repository: repository}
}

func (s *DashboardService) Summary(ctx context.Context, dateRange operations.DashboardRange) (operations.DashboardSummary, error) {
	return s.repository.Summary(ctx, dateRange)
}

func (s *DashboardService) TimeSeries(ctx context.Context, query operations.TimeSeriesQuery) ([]operations.TimeSeriesBucket, error) {
	return s.repository.TimeSeries(ctx, query)
}

func (s *DashboardService) ListOperations(ctx context.Context, query operations.OperationQuery) (operations.OperationsPage, error) {
	operationsList, err := s.repository.ListOperations(ctx, query)
	if err != nil {
		return operations.OperationsPage{}, err
	}
	total, err := s.repository.CountOperations(ctx, query)
	if err != nil {
		return operations.OperationsPage{}, err
	}
	return operations.OperationsPage{Operations: operationsList, Total: total}, nil
}

func (s *DashboardService) GetOperation(ctx context.Context, id string) (operations.OperationDetail, bool, error) {
	return s.repository.GetOperation(ctx, id)
}

func (s *DashboardService) ListProxies(ctx context.Context, dateRange operations.DashboardRange) ([]operations.ProxyDashboard, error) {
	return s.repository.ListProxies(ctx, dateRange)
}
