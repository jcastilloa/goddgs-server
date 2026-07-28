package operations

import (
	"context"
	_ "embed"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	operations "github.com/jcastilloa/goddgs-server/operations/domain"

	"github.com/gin-gonic/gin"
)

//go:embed assets/dashboard.html
var dashboardHTML []byte

const (
	DashboardHandlerLabel  = "handler.operations.dashboard"
	SummaryHandlerLabel    = "handler.operations.summary"
	TimeSeriesHandlerLabel = "handler.operations.timeseries"
	ListHandlerLabel       = "handler.operations.list"
	DetailHandlerLabel     = "handler.operations.detail"
	ProxiesHandlerLabel    = "handler.operations.proxies"
	defaultLimit           = 50
	maximumLimit           = 100
)

type SummaryUseCase interface {
	Summary(context.Context, operations.DashboardRange) (operations.DashboardSummary, error)
}

type TimeSeriesUseCase interface {
	TimeSeries(context.Context, operations.TimeSeriesQuery) ([]operations.TimeSeriesBucket, error)
}

type OperationsUseCase interface {
	ListOperations(context.Context, operations.OperationQuery) (operations.OperationsPage, error)
}

type DetailUseCase interface {
	GetOperation(context.Context, string) (operations.OperationDetail, bool, error)
}

type ProxiesUseCase interface {
	ListProxies(context.Context, operations.DashboardRange) ([]operations.ProxyDashboard, error)
}

type DashboardUseCase interface {
	SummaryUseCase
	TimeSeriesUseCase
	OperationsUseCase
	DetailUseCase
	ProxiesUseCase
}

type EmptyUseCase struct{}

func (EmptyUseCase) Summary(context.Context, operations.DashboardRange) (operations.DashboardSummary, error) {
	return operations.DashboardSummary{}, nil
}

func (EmptyUseCase) TimeSeries(context.Context, operations.TimeSeriesQuery) ([]operations.TimeSeriesBucket, error) {
	return []operations.TimeSeriesBucket{}, nil
}

func (EmptyUseCase) ListOperations(context.Context, operations.OperationQuery) (operations.OperationsPage, error) {
	return operations.OperationsPage{Operations: []operations.Operation{}}, nil
}

func (EmptyUseCase) GetOperation(context.Context, string) (operations.OperationDetail, bool, error) {
	return operations.OperationDetail{}, false, nil
}

func (EmptyUseCase) ListProxies(context.Context, operations.DashboardRange) ([]operations.ProxyDashboard, error) {
	return []operations.ProxyDashboard{}, nil
}

type Dashboard struct{}

func NewDashboard() Dashboard { return Dashboard{} }

func (h Dashboard) Handle(context *gin.Context) {
	context.Data(http.StatusOK, "text/html; charset=utf-8", dashboardHTML)
}

type Summary struct {
	dashboardHandler
	useCase SummaryUseCase
}

func NewSummary(useCase SummaryUseCase) Summary {
	return Summary{dashboardHandler: newDashboardHandler(), useCase: useCase}
}

func (h Summary) Handle(context *gin.Context) {
	dateRange, err := h.dateRange(context)
	if err != nil {
		writeBadRequest(context, err)
		return
	}
	result, err := h.useCase.Summary(context.Request.Context(), dateRange)
	if err != nil {
		writeDashboardError(context, err)
		return
	}
	context.JSON(http.StatusOK, result)
}

type TimeSeries struct {
	dashboardHandler
	useCase TimeSeriesUseCase
}

func NewTimeSeries(useCase TimeSeriesUseCase) TimeSeries {
	return TimeSeries{dashboardHandler: newDashboardHandler(), useCase: useCase}
}

func (h TimeSeries) Handle(context *gin.Context) {
	dateRange, err := h.dateRange(context)
	if err != nil {
		writeBadRequest(context, err)
		return
	}
	interval, err := parseInterval(context.Query("interval"), dateRange)
	if err != nil {
		writeBadRequest(context, err)
		return
	}
	result, err := h.useCase.TimeSeries(context.Request.Context(), operations.TimeSeriesQuery{DashboardRange: dateRange, Interval: interval})
	if err != nil {
		writeDashboardError(context, err)
		return
	}
	context.JSON(http.StatusOK, result)
}

type List struct {
	dashboardHandler
	useCase OperationsUseCase
}

func NewList(useCase OperationsUseCase) List {
	return List{dashboardHandler: newDashboardHandler(), useCase: useCase}
}

func (h List) Handle(context *gin.Context) {
	query, err := h.operationQuery(context)
	if err != nil {
		writeBadRequest(context, err)
		return
	}
	result, err := h.useCase.ListOperations(context.Request.Context(), query)
	if err != nil {
		writeDashboardError(context, err)
		return
	}
	context.JSON(http.StatusOK, result)
}

type Detail struct {
	dashboardHandler
	useCase DetailUseCase
}

func NewDetail(useCase DetailUseCase) Detail {
	return Detail{dashboardHandler: newDashboardHandler(), useCase: useCase}
}

func (h Detail) Handle(context *gin.Context) {
	id := strings.TrimSpace(context.Param("id"))
	if id == "" {
		writeBadRequest(context, errors.New("operation id is required"))
		return
	}
	result, found, err := h.useCase.GetOperation(context.Request.Context(), id)
	if err != nil {
		writeDashboardError(context, err)
		return
	}
	if !found {
		context.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}
	context.JSON(http.StatusOK, result)
}

type Proxies struct {
	dashboardHandler
	useCase ProxiesUseCase
}

func NewProxies(useCase ProxiesUseCase) Proxies {
	return Proxies{dashboardHandler: newDashboardHandler(), useCase: useCase}
}

func (h Proxies) Handle(context *gin.Context) {
	dateRange, err := h.dateRange(context)
	if err != nil {
		writeBadRequest(context, err)
		return
	}
	result, err := h.useCase.ListProxies(context.Request.Context(), dateRange)
	if err != nil {
		writeDashboardError(context, err)
		return
	}
	context.JSON(http.StatusOK, result)
}

type dashboardHandler struct {
	now func() time.Time
}

func newDashboardHandler() dashboardHandler {
	return dashboardHandler{now: time.Now}
}

func (h dashboardHandler) dateRange(context *gin.Context) (operations.DashboardRange, error) {
	return parseDateRange(context.Query("range"), context.Query("from"), context.Query("to"), h.now().UTC())
}

func (h dashboardHandler) operationQuery(context *gin.Context) (operations.OperationQuery, error) {
	dateRange, err := h.dateRange(context)
	if err != nil {
		return operations.OperationQuery{}, err
	}
	limit, err := boundedInt(context.Query("limit"), defaultLimit, 1, maximumLimit, "limit")
	if err != nil {
		return operations.OperationQuery{}, err
	}
	offset, err := boundedInt(context.Query("offset"), 0, 0, 10_000, "offset")
	if err != nil {
		return operations.OperationQuery{}, err
	}
	status, err := parseStatus(context.Query("status"))
	if err != nil {
		return operations.OperationQuery{}, err
	}
	operationType, err := parseType(context.Query("type"))
	if err != nil {
		return operations.OperationQuery{}, err
	}
	return operations.OperationQuery{From: dateRange.From, To: dateRange.To, Status: status, Type: operationType, Limit: limit, Offset: offset}, nil
}

func parseDateRange(rangeValue, fromValue, toValue string, now time.Time) (operations.DashboardRange, error) {
	rangeValue = strings.TrimSpace(rangeValue)
	fromValue = strings.TrimSpace(fromValue)
	toValue = strings.TrimSpace(toValue)
	if rangeValue != "" && (fromValue != "" || toValue != "") {
		return operations.DashboardRange{}, errors.New("range cannot be combined with from or to")
	}
	if fromValue == "" && toValue == "" {
		duration, ok := ranges[rangeValue]
		if rangeValue == "" {
			duration = 24 * time.Hour
			ok = true
		}
		if !ok {
			return operations.DashboardRange{}, errors.New("range must be one of 24h, 7d, or 30d")
		}
		return operations.DashboardRange{From: now.Add(-duration), To: now}, nil
	}
	if fromValue == "" || toValue == "" {
		return operations.DashboardRange{}, errors.New("from and to must be provided together as ISO-8601 timestamps")
	}
	from, err := time.Parse(time.RFC3339, fromValue)
	if err != nil {
		return operations.DashboardRange{}, errors.New("from must be an ISO-8601 timestamp")
	}
	to, err := time.Parse(time.RFC3339, toValue)
	if err != nil {
		return operations.DashboardRange{}, errors.New("to must be an ISO-8601 timestamp")
	}
	if !from.Before(to) {
		return operations.DashboardRange{}, errors.New("from must be before to")
	}
	if to.Sub(from) > 30*24*time.Hour {
		return operations.DashboardRange{}, errors.New("date range must not exceed 30 days")
	}
	return operations.DashboardRange{From: from.UTC(), To: to.UTC()}, nil
}

var ranges = map[string]time.Duration{
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
}

func parseInterval(value string, dateRange operations.DashboardRange) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		switch duration := dateRange.To.Sub(dateRange.From); {
		case duration <= 24*time.Hour:
			return time.Hour, nil
		case duration <= 7*24*time.Hour:
			return 6 * time.Hour, nil
		default:
			return 24 * time.Hour, nil
		}
	}
	interval, ok := intervals[strings.TrimSpace(value)]
	if !ok || interval > dateRange.To.Sub(dateRange.From) {
		return 0, errors.New("interval must be 1h, 6h, or 24h and no longer than the selected range")
	}
	return interval, nil
}

var intervals = map[string]time.Duration{
	"1h":  time.Hour,
	"6h":  6 * time.Hour,
	"24h": 24 * time.Hour,
}

func boundedInt(value string, defaultValue, minimum, maximum int, name string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, errors.New(name + " must be between " + strconv.Itoa(minimum) + " and " + strconv.Itoa(maximum))
	}
	return parsed, nil
}

func parseStatus(value string) (operations.Status, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	status := operations.Status(value)
	for _, candidate := range []operations.Status{operations.StatusRunning, operations.StatusSucceeded, operations.StatusFailed} {
		if status == candidate {
			return status, nil
		}
	}
	return "", errors.New("status must be running, succeeded, or failed")
}

func parseType(value string) (operations.OperationType, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	operationType := operations.OperationType(value)
	for _, candidate := range []operations.OperationType{operations.OperationSearch, operations.OperationExtract, operations.OperationResearch} {
		if operationType == candidate {
			return operationType, nil
		}
	}
	return "", errors.New("type must be search, extract, or research")
}

func writeBadRequest(context *gin.Context, err error) {
	context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

func writeDashboardError(context *gin.Context, err error) {
	context.Error(err)
	context.JSON(http.StatusInternalServerError, gin.H{"error": "operations dashboard is unavailable"})
}
