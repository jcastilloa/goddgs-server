package domain

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrDashboardSetupCompleted     = errors.New("dashboard setup completed")
	ErrInvalidDashboardInput       = errors.New("invalid dashboard input")
	ErrDashboardPasswordUnchanged  = errors.New("dashboard password unchanged")
	ErrInvalidDashboardCredentials = errors.New("invalid dashboard credentials")
	ErrDashboardSessionNotFound    = errors.New("dashboard session not found")
	ErrDashboardSessionExpired     = errors.New("dashboard session expired")
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

type Result string

const (
	ResultSucceeded Result = "succeeded"
	ResultFailed    Result = "failed"
	ResultCanceled  Result = "canceled"
	ResultTimeout   Result = "timeout"
)

type ProxyHealth string

const (
	ProxyHealthUnknown   ProxyHealth = "unknown"
	ProxyHealthHealthy   ProxyHealth = "healthy"
	ProxyHealthDegraded  ProxyHealth = "degraded"
	ProxyHealthUnhealthy ProxyHealth = "unhealthy"
)

type OperationType string

const (
	OperationSearch   OperationType = "search"
	OperationExtract  OperationType = "extract"
	OperationResearch OperationType = "research"
)

type StepType string

const (
	StepSearch            StepType = "search"
	StepExtractHeuristic  StepType = "extract_heuristic"
	StepExtractAI         StepType = "extract_ai"
	StepLLMPlanning       StepType = "llm_planning"
	StepLLMSelection      StepType = "llm_selection"
	StepLLMReport         StepType = "llm_report"
	StepResearchPlanning  StepType = "research_planning"
	StepResearchSearch    StepType = "research_search"
	StepResearchSelection StepType = "research_selection"
	StepResearchExtract   StepType = "research_extract"
	StepResearchReport    StepType = "research_report"
)

type ErrorCategory string

const (
	ErrorCanceled        ErrorCategory = "canceled"
	ErrorTimeout         ErrorCategory = "timeout"
	ErrorRateLimited     ErrorCategory = "rate_limited"
	ErrorTransport       ErrorCategory = "transport"
	ErrorUpstream5xx     ErrorCategory = "upstream_5xx"
	ErrorInvalidResponse ErrorCategory = "invalid_response"
	ErrorConfiguration   ErrorCategory = "configuration"
	ErrorUnknown         ErrorCategory = "unknown"
)

type Operation struct {
	ID         string            `json:"id"`
	Type       OperationType     `json:"type"`
	Status     Status            `json:"status"`
	StartedAt  time.Time         `json:"started_at"`
	FinishedAt time.Time         `json:"finished_at,omitzero"`
	DurationMS int64             `json:"duration_ms"`
	Result     Result            `json:"result,omitempty"`
	HTTPMethod string            `json:"http_method,omitempty"`
	HTTPPath   string            `json:"http_path,omitempty"`
	HTTPStatus int               `json:"http_status,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Details    json.RawMessage   `json:"details,omitempty"`
}

type Step struct {
	ID          string            `json:"id"`
	OperationID string            `json:"operation_id"`
	Name        string            `json:"name"`
	Type        StepType          `json:"type"`
	Status      Status            `json:"status"`
	StartedAt   time.Time         `json:"started_at"`
	FinishedAt  time.Time         `json:"finished_at,omitzero"`
	DurationMS  int64             `json:"duration_ms"`
	Result      Result            `json:"result,omitempty"`
	Provider    string            `json:"provider,omitempty"`
	Backend     string            `json:"backend,omitempty"`
	Proxy       string            `json:"proxy,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Details     json.RawMessage   `json:"details,omitempty"`
}

type OperationError struct {
	OperationID string        `json:"operation_id"`
	StepID      string        `json:"step_id,omitempty"`
	Category    ErrorCategory `json:"category,omitempty"`
	Message     string        `json:"message"`
	OccurredAt  time.Time     `json:"occurred_at"`
}

type OperationStart struct {
	Type     OperationType
	Method   string
	Path     string
	Metadata map[string]string
	Details  json.RawMessage
}

type OperationFinish struct {
	HTTPStatus int
	Err        error
}

type StepStart struct {
	Type     StepType
	Provider string
	Backend  string
	Proxy    string
	Metadata map[string]string
	Details  json.RawMessage
}

type ProxyProbe struct {
	ProxyName     string
	Healthy       bool
	Status        ProxyHealth
	Result        Result
	HTTPStatus    int
	ErrorCategory ErrorCategory
	Duration      time.Duration
	ObservedAt    time.Time
}

type ProxyHealthTransition struct {
	ProxyName  string
	Healthy    bool
	Status     ProxyHealth
	OccurredAt time.Time
}

type OperationQuery struct {
	From   time.Time
	To     time.Time
	Status Status
	Type   OperationType
	Limit  int
	Offset int
}

type DashboardRange struct {
	From time.Time
	To   time.Time
}

type DashboardSummary struct {
	Active    int   `json:"active"`
	Succeeded int   `json:"succeeded"`
	Failed    int   `json:"failed"`
	P50MS     int64 `json:"p50_ms"`
	P95MS     int64 `json:"p95_ms"`
}

type TimeSeriesQuery struct {
	DashboardRange
	Interval time.Duration
}

type TimeSeriesBucket struct {
	StartedAt time.Time `json:"started_at"`
	Succeeded int       `json:"succeeded"`
	Failed    int       `json:"failed"`
	P50MS     int64     `json:"p50_ms"`
	P95MS     int64     `json:"p95_ms"`
}

type OperationsPage struct {
	Operations []Operation `json:"operations"`
	Total      int         `json:"total"`
}

type OperationDetail struct {
	Operation Operation        `json:"operation"`
	Steps     []Step           `json:"steps"`
	Errors    []OperationError `json:"errors"`
}

type ProxyPoint struct {
	ObservedAt time.Time   `json:"observed_at"`
	Healthy    bool        `json:"healthy"`
	Status     ProxyHealth `json:"status"`
	Result     Result      `json:"result,omitempty"`
	DurationMS int64       `json:"duration_ms"`
}

type ProxyDashboard struct {
	Name       string       `json:"name"`
	Healthy    bool         `json:"healthy"`
	Status     ProxyHealth  `json:"status,omitempty"`
	ObservedAt time.Time    `json:"observed_at,omitzero"`
	DurationMS int64        `json:"duration_ms"`
	Points     []ProxyPoint `json:"points"`
}

type Repository interface {
	CreateOperation(context.Context, Operation) error
	FinishOperation(context.Context, Operation) error
	AddStep(context.Context, Step) error
	FinishStep(context.Context, Step) error
	AddError(context.Context, OperationError) error
	RecordProbe(context.Context, ProxyProbe) error
	RecordHealthTransition(context.Context, ProxyHealthTransition) error
	ListOperations(context.Context, OperationQuery) ([]Operation, error)
}

type RetentionRepository interface {
	DeleteExpired(context.Context, time.Time) error
}

type DashboardRepository interface {
	ListOperations(context.Context, OperationQuery) ([]Operation, error)
	CountOperations(context.Context, OperationQuery) (int, error)
	GetOperation(context.Context, string) (OperationDetail, bool, error)
	Summary(context.Context, DashboardRange) (DashboardSummary, error)
	TimeSeries(context.Context, TimeSeriesQuery) ([]TimeSeriesBucket, error)
	ListProxies(context.Context, DashboardRange) ([]ProxyDashboard, error)
}

type DashboardUser struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type DashboardSession struct {
	TokenHash string
	CSRFHash  string
	UserID    int64
	Username  string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type DashboardAuthRepository interface {
	HasDashboardUser(context.Context) (bool, error)
	CreateDashboardUser(context.Context, DashboardUser) (DashboardUser, error)
	FindDashboardUserByID(context.Context, int64) (DashboardUser, bool, error)
	FindDashboardUserByUsername(context.Context, string) (DashboardUser, bool, error)
	CreateDashboardSession(context.Context, DashboardSession) error
	FindDashboardSession(context.Context, string) (DashboardSession, bool, error)
	DeleteDashboardSession(context.Context, string) error
	ReplaceDashboardPasswordAndSession(context.Context, int64, string, DashboardSession) error
}
