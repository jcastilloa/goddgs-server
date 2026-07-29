package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	operations "github.com/jcastilloa/goddgs-server/operations/domain"
	_ "modernc.org/sqlite"
)

const (
	driverName         = "sqlite"
	busyTimeout        = 5 * time.Second
	defaultDBFile      = "operations.sqlite"
	retentionBatchSize = 1_000
)

var ErrOpen = errors.New("open operations store")

type Config struct {
	DatabasePath string
}

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, config Config) (*Store, error) {
	path, err := ResolveDatabasePath(config.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve database path: %w", ErrOpen, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("%w: create database directory for %q: %w", ErrOpen, path, err)
	}
	database, err := sql.Open(driverName, databaseSource(path))
	if err != nil {
		return nil, fmt.Errorf("%w: open %q: %w", ErrOpen, path, err)
	}
	store := &Store{db: database}
	if err := store.initialize(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("%w: initialize %q: %w", ErrOpen, path, err)
	}
	return store, nil
}

func ResolveDatabasePath(configuredPath string) (string, error) {
	return resolveDatabasePath(configuredPath, os.Executable)
}

func resolveDatabasePath(configuredPath string, executable func() (string, error)) (string, error) {
	if path := strings.TrimSpace(configuredPath); path != "" {
		return filepath.Abs(path)
	}
	path, err := executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(path); resolveErr == nil {
		path = resolved
	}
	return filepath.Join(filepath.Dir(path), defaultDBFile), nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) DeleteExpired(ctx context.Context, cutoff time.Time) error {
	for {
		deleted, err := s.deleteExpiredBatch(ctx, cutoff)
		if err != nil {
			return err
		}
		if deleted < retentionBatchSize {
			return nil
		}
	}
}

func (s *Store) deleteExpiredBatch(ctx context.Context, cutoff time.Time) (int64, error) {
	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("start expired records cleanup: %w", err)
	}
	deleted, err := deleteExpiredRecords(ctx, transaction, cutoff)
	if err != nil {
		_ = transaction.Rollback()
		return 0, err
	}
	if err := transaction.Commit(); err != nil {
		return 0, fmt.Errorf("commit expired records cleanup: %w", err)
	}
	return deleted, nil
}

func deleteExpiredRecords(ctx context.Context, transaction *sql.Tx, cutoff time.Time) (int64, error) {
	value := timestamp(cutoff)
	queries := []struct {
		name  string
		query string
	}{
		{
			name: "operations",
			query: `DELETE FROM operations
WHERE id IN (
  SELECT id FROM operations
  WHERE finished_at IS NOT NULL AND finished_at < ?
  ORDER BY finished_at
  LIMIT ?
)`,
		},
		{name: "proxy probes", query: "DELETE FROM proxy_probes WHERE id IN (SELECT id FROM proxy_probes WHERE observed_at < ? ORDER BY observed_at LIMIT ?)"},
		{name: "proxy health transitions", query: "DELETE FROM proxy_health_transitions WHERE id IN (SELECT id FROM proxy_health_transitions WHERE occurred_at < ? ORDER BY occurred_at LIMIT ?)"},
	}
	var deleted int64
	for _, item := range queries {
		result, err := transaction.ExecContext(ctx, item.query, value, retentionBatchSize)
		if err != nil {
			return 0, fmt.Errorf("delete expired %s: %w", item.name, err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("count deleted %s: %w", item.name, err)
		}
		deleted += count
	}
	return deleted, nil
}

func (s *Store) CreateOperation(ctx context.Context, operation operations.Operation) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO operations (id, type, status, started_at, finished_at, duration_ms, result, http_method, http_path, http_status, metadata, details)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, operation.ID, operation.Type, operation.Status, timestamp(operation.StartedAt), nullableTimestamp(operation.FinishedAt), nullableInt64(operation.DurationMS), nullableString(string(operation.Result)), nullableString(operation.HTTPMethod), nullableString(operation.HTTPPath), nullableInt(operation.HTTPStatus), metadata(operation.Metadata), details(operation.Details))
	return wrapWriteError("create operation", err)
}

func (s *Store) FinishOperation(ctx context.Context, operation operations.Operation) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE operations
SET status = ?, finished_at = ?, duration_ms = ?, result = ?, http_status = ?
WHERE id = ?`, operation.Status, nullableTimestamp(operation.FinishedAt), nullableInt64(operation.DurationMS), nullableString(string(operation.Result)), nullableInt(operation.HTTPStatus), operation.ID)
	return wrapWriteError("finish operation", err)
}

func (s *Store) AddStep(ctx context.Context, step operations.Step) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO operation_steps (step_id, operation_id, name, type, status, started_at, finished_at, duration_ms, result, provider, backend, proxy, metadata, details)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, step.ID, step.OperationID, stepName(step), step.Type, step.Status, timestamp(step.StartedAt), nullableTimestamp(step.FinishedAt), nullableInt64(step.DurationMS), nullableString(string(step.Result)), nullableString(step.Provider), nullableString(step.Backend), nullableString(step.Proxy), metadata(step.Metadata), details(step.Details))
	return wrapWriteError("add operation step", err)
}

func (s *Store) FinishStep(ctx context.Context, step operations.Step) error {
	query := `
UPDATE operation_steps
SET status = ?, finished_at = ?, duration_ms = ?, result = ?`
	arguments := []any{step.Status, nullableTimestamp(step.FinishedAt), nullableInt64(step.DurationMS), nullableString(string(step.Result))}
	if step.Metadata != nil {
		query += `, metadata = ?`
		arguments = append(arguments, metadata(step.Metadata))
	}
	if step.Details != nil {
		query += `, details = ?`
		arguments = append(arguments, details(step.Details))
	}
	query += ` WHERE step_id = ?`
	arguments = append(arguments, step.ID)
	_, err := s.db.ExecContext(ctx, query, arguments...)
	return wrapWriteError("finish operation step", err)
}

func (s *Store) AddError(ctx context.Context, operationError operations.OperationError) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO operation_errors (operation_id, step_id, category, message, occurred_at)
VALUES (?, ?, ?, ?, ?)`, operationError.OperationID, nullableString(operationError.StepID), nullableString(string(operationError.Category)), operationError.Message, timestamp(operationError.OccurredAt))
	return wrapWriteError("add operation error", err)
}

func (s *Store) RecordProbe(ctx context.Context, probe operations.ProxyProbe) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO proxy_probes (proxy_name, healthy, health_status, result, http_status, error_category, duration_ms, observed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, probe.ProxyName, probe.Healthy, nullableString(string(probe.Status)), nullableString(string(probe.Result)), nullableInt(probe.HTTPStatus), nullableString(string(probe.ErrorCategory)), nullableInt64(probe.Duration.Milliseconds()), timestamp(probe.ObservedAt))
	return wrapWriteError("record proxy probe", err)
}

func (s *Store) RecordHealthTransition(ctx context.Context, transition operations.ProxyHealthTransition) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO proxy_health_transitions (proxy_name, healthy, health_status, occurred_at)
VALUES (?, ?, ?, ?)`, transition.ProxyName, transition.Healthy, nullableString(string(transition.Status)), timestamp(transition.OccurredAt))
	return wrapWriteError("record proxy health transition", err)
}

func (s *Store) HasDashboardUser(ctx context.Context) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM operations_dashboard_users)").Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check dashboard user: %w", err)
	}
	return exists, nil
}

func (s *Store) CreateDashboardUser(ctx context.Context, user operations.DashboardUser) (operations.DashboardUser, error) {
	result, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO operations_dashboard_users (id, username, password_hash, created_at, updated_at)
VALUES (1, ?, ?, ?, ?)`, user.Username, user.PasswordHash, timestamp(user.CreatedAt), timestamp(user.UpdatedAt))
	if err != nil {
		return operations.DashboardUser{}, fmt.Errorf("create dashboard user: %w", err)
	}
	created, err := result.RowsAffected()
	if err != nil {
		return operations.DashboardUser{}, fmt.Errorf("count dashboard user creation: %w", err)
	}
	if created == 0 {
		return operations.DashboardUser{}, operations.ErrDashboardSetupCompleted
	}
	user.ID = 1
	return user, nil
}

func (s *Store) FindDashboardUserByID(ctx context.Context, id int64) (operations.DashboardUser, bool, error) {
	return s.findDashboardUser(ctx, "id = ?", id)
}

func (s *Store) FindDashboardUserByUsername(ctx context.Context, username string) (operations.DashboardUser, bool, error) {
	return s.findDashboardUser(ctx, "username = ?", username)
}

func (s *Store) findDashboardUser(ctx context.Context, predicate string, argument any) (operations.DashboardUser, bool, error) {
	var user operations.DashboardUser
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, "SELECT id, username, password_hash, created_at, updated_at FROM operations_dashboard_users WHERE "+predicate, argument).Scan(&user.ID, &user.Username, &user.PasswordHash, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return operations.DashboardUser{}, false, nil
	}
	if err != nil {
		return operations.DashboardUser{}, false, fmt.Errorf("find dashboard user: %w", err)
	}
	var parseErr error
	user.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, createdAt)
	if parseErr != nil {
		return operations.DashboardUser{}, false, fmt.Errorf("parse dashboard user creation: %w", parseErr)
	}
	user.UpdatedAt, parseErr = time.Parse(time.RFC3339Nano, updatedAt)
	if parseErr != nil {
		return operations.DashboardUser{}, false, fmt.Errorf("parse dashboard user update: %w", parseErr)
	}
	return user, true, nil
}

func (s *Store) CreateDashboardSession(ctx context.Context, session operations.DashboardSession) error {
	if err := s.deleteExpiredDashboardSessions(ctx, session.CreatedAt); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO operations_dashboard_sessions (token_hash, csrf_hash, user_id, created_at, expires_at)
VALUES (?, ?, ?, ?, ?)`, session.TokenHash, session.CSRFHash, session.UserID, timestamp(session.CreatedAt), timestamp(session.ExpiresAt))
	if err != nil {
		return fmt.Errorf("create dashboard session: %w", err)
	}
	return nil
}

func (s *Store) FindDashboardSession(ctx context.Context, tokenHash string) (operations.DashboardSession, bool, error) {
	var session operations.DashboardSession
	var createdAt, expiresAt string
	err := s.db.QueryRowContext(ctx, `
SELECT s.token_hash, s.csrf_hash, s.user_id, u.username, s.created_at, s.expires_at
FROM operations_dashboard_sessions s
JOIN operations_dashboard_users u ON u.id = s.user_id
WHERE s.token_hash = ?`, tokenHash).Scan(&session.TokenHash, &session.CSRFHash, &session.UserID, &session.Username, &createdAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return operations.DashboardSession{}, false, nil
	}
	if err != nil {
		return operations.DashboardSession{}, false, fmt.Errorf("find dashboard session: %w", err)
	}
	var parseErr error
	session.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, createdAt)
	if parseErr != nil {
		return operations.DashboardSession{}, false, fmt.Errorf("parse dashboard session creation: %w", parseErr)
	}
	session.ExpiresAt, parseErr = time.Parse(time.RFC3339Nano, expiresAt)
	if parseErr != nil {
		return operations.DashboardSession{}, false, fmt.Errorf("parse dashboard session expiry: %w", parseErr)
	}
	return session, true, nil
}

func (s *Store) DeleteDashboardSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM operations_dashboard_sessions WHERE token_hash = ?", tokenHash)
	if err != nil {
		return fmt.Errorf("delete dashboard session: %w", err)
	}
	return nil
}

func (s *Store) ReplaceDashboardPasswordAndSession(ctx context.Context, userID int64, passwordHash string, session operations.DashboardSession) error {
	transaction, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start dashboard password update: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, "UPDATE operations_dashboard_users SET password_hash = ?, updated_at = ? WHERE id = ?", passwordHash, timestamp(session.CreatedAt), userID); err != nil {
		_ = transaction.Rollback()
		return fmt.Errorf("update dashboard password: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, "DELETE FROM operations_dashboard_sessions WHERE user_id = ?", userID); err != nil {
		_ = transaction.Rollback()
		return fmt.Errorf("revoke dashboard sessions: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
INSERT INTO operations_dashboard_sessions (token_hash, csrf_hash, user_id, created_at, expires_at)
VALUES (?, ?, ?, ?, ?)`, session.TokenHash, session.CSRFHash, userID, timestamp(session.CreatedAt), timestamp(session.ExpiresAt)); err != nil {
		_ = transaction.Rollback()
		return fmt.Errorf("create replacement dashboard session: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit dashboard password update: %w", err)
	}
	return nil
}

func (s *Store) ListOperations(ctx context.Context, query operations.OperationQuery) ([]operations.Operation, error) {
	statement, arguments := operationListQuery(query)
	rows, err := s.db.QueryContext(ctx, statement, arguments...)
	if err != nil {
		return nil, fmt.Errorf("list operations: %w", err)
	}
	defer rows.Close()
	result := make([]operations.Operation, 0)
	for rows.Next() {
		operation, err := scanOperation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, operation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operations: %w", err)
	}
	return result, nil
}

func (s *Store) CountOperations(ctx context.Context, query operations.OperationQuery) (int, error) {
	statement, arguments := operationCountQuery(query)
	var count int
	if err := s.db.QueryRowContext(ctx, statement, arguments...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count operations: %w", err)
	}
	return count, nil
}

func (s *Store) GetOperation(ctx context.Context, id string) (operations.OperationDetail, bool, error) {
	operation, found, err := s.findOperation(ctx, id)
	if err != nil || !found {
		return operations.OperationDetail{}, found, err
	}
	steps, err := s.listSteps(ctx, id)
	if err != nil {
		return operations.OperationDetail{}, false, err
	}
	errors, err := s.listErrors(ctx, id)
	if err != nil {
		return operations.OperationDetail{}, false, err
	}
	return operations.OperationDetail{Operation: operation, Steps: steps, Errors: errors}, true, nil
}

func (s *Store) Summary(ctx context.Context, dateRange operations.DashboardRange) (operations.DashboardSummary, error) {
	var summary operations.DashboardSummary
	if err := s.db.QueryRowContext(ctx, `
SELECT
  COALESCE(SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN result = 'succeeded' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN result IS NOT NULL AND result != 'succeeded' THEN 1 ELSE 0 END), 0)
FROM operations
WHERE started_at >= ? AND started_at <= ?`, timestamp(dateRange.From), timestamp(dateRange.To)).Scan(&summary.Active, &summary.Succeeded, &summary.Failed); err != nil {
		return operations.DashboardSummary{}, fmt.Errorf("summarize operations: %w", err)
	}
	durations, err := s.operationDurations(ctx, dateRange)
	if err != nil {
		return operations.DashboardSummary{}, err
	}
	summary.P50MS = percentile(durations, 50)
	summary.P95MS = percentile(durations, 95)
	return summary, nil
}

func (s *Store) TimeSeries(ctx context.Context, query operations.TimeSeriesQuery) ([]operations.TimeSeriesBucket, error) {
	if query.Interval <= 0 {
		return nil, errors.New("operation time series interval must be positive")
	}
	buckets, err := s.timeSeriesBuckets(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(buckets) == 0 {
		return []operations.TimeSeriesBucket{}, nil
	}
	if err := s.addTimeSeriesDurations(ctx, query, buckets); err != nil {
		return nil, err
	}
	result := make([]operations.TimeSeriesBucket, 0, len(buckets))
	for bucketTime := query.From.UTC().Truncate(query.Interval); !bucketTime.After(query.To); bucketTime = bucketTime.Add(query.Interval) {
		bucket := buckets[bucketTime]
		if bucket == nil {
			result = append(result, operations.TimeSeriesBucket{StartedAt: bucketTime})
			continue
		}
		result = append(result, operations.TimeSeriesBucket{
			StartedAt: bucketTime,
			Succeeded: bucket.succeeded,
			Failed:    bucket.failed,
			P50MS:     percentile(bucket.durations, 50),
			P95MS:     percentile(bucket.durations, 95),
		})
	}
	return result, nil
}

func (s *Store) timeSeriesBuckets(ctx context.Context, query operations.TimeSeriesQuery) (map[time.Time]*timeSeriesAccumulator, error) {
	intervalSeconds := int64(query.Interval / time.Second)
	rows, err := s.db.QueryContext(ctx, `
SELECT
  (CAST(strftime('%s', started_at) AS INTEGER) / ?) * ?,
  COALESCE(SUM(CASE WHEN result = ? THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN result IS NOT NULL AND result != ? THEN 1 ELSE 0 END), 0)
FROM operations
WHERE started_at >= ? AND started_at <= ?
GROUP BY 1
ORDER BY 1`, intervalSeconds, intervalSeconds, operations.ResultSucceeded, operations.ResultSucceeded, timestamp(query.From), timestamp(query.To))
	if err != nil {
		return nil, fmt.Errorf("aggregate operation time series: %w", err)
	}
	defer rows.Close()

	buckets := make(map[time.Time]*timeSeriesAccumulator)
	for rows.Next() {
		var bucketUnix int64
		bucket := &timeSeriesAccumulator{}
		if err := rows.Scan(&bucketUnix, &bucket.succeeded, &bucket.failed); err != nil {
			return nil, fmt.Errorf("scan operation time series aggregate: %w", err)
		}
		buckets[time.Unix(bucketUnix, 0).UTC()] = bucket
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operation time series aggregate: %w", err)
	}
	return buckets, nil
}

func (s *Store) addTimeSeriesDurations(ctx context.Context, query operations.TimeSeriesQuery, buckets map[time.Time]*timeSeriesAccumulator) error {
	intervalSeconds := int64(query.Interval / time.Second)
	rows, err := s.db.QueryContext(ctx, `
SELECT (CAST(strftime('%s', started_at) AS INTEGER) / ?) * ?, duration_ms
FROM operations
WHERE started_at >= ? AND started_at <= ?
  AND finished_at IS NOT NULL
  AND duration_ms IS NOT NULL`, intervalSeconds, intervalSeconds, timestamp(query.From), timestamp(query.To))
	if err != nil {
		return fmt.Errorf("list operation time series durations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var bucketUnix, duration int64
		if err := rows.Scan(&bucketUnix, &duration); err != nil {
			return fmt.Errorf("scan operation time series duration: %w", err)
		}
		bucketTime := time.Unix(bucketUnix, 0).UTC()
		if bucket := buckets[bucketTime]; bucket != nil {
			bucket.durations = append(bucket.durations, duration)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate operation time series durations: %w", err)
	}
	return nil
}

func (s *Store) ListProxies(ctx context.Context, dateRange operations.DashboardRange) ([]operations.ProxyDashboard, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT proxy_name, healthy, health_status, result, duration_ms, observed_at
FROM proxy_probes
WHERE observed_at >= ? AND observed_at <= ?
ORDER BY proxy_name, observed_at`, timestamp(dateRange.From), timestamp(dateRange.To))
	if err != nil {
		return nil, fmt.Errorf("list proxy dashboard: %w", err)
	}
	defer rows.Close()

	proxies := make(map[string]*operations.ProxyDashboard)
	var names []string
	for rows.Next() {
		var name, observedAt string
		var healthy bool
		var status, result sql.NullString
		var duration sql.NullInt64
		if err := rows.Scan(&name, &healthy, &status, &result, &duration, &observedAt); err != nil {
			return nil, fmt.Errorf("scan proxy dashboard: %w", err)
		}
		occurredAt, err := time.Parse(time.RFC3339Nano, observedAt)
		if err != nil {
			return nil, fmt.Errorf("parse proxy dashboard timestamp: %w", err)
		}
		proxy := proxies[name]
		if proxy == nil {
			proxy = &operations.ProxyDashboard{Name: name}
			proxies[name] = proxy
			names = append(names, name)
		}
		point := operations.ProxyPoint{ObservedAt: occurredAt, Healthy: healthy, Status: operations.ProxyHealth(status.String), Result: operations.Result(result.String), DurationMS: duration.Int64}
		proxy.Points = append(proxy.Points, point)
		proxy.Healthy, proxy.Status, proxy.ObservedAt, proxy.DurationMS = point.Healthy, point.Status, point.ObservedAt, point.DurationMS
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate proxy dashboard: %w", err)
	}
	result := make([]operations.ProxyDashboard, 0, len(names))
	for _, name := range names {
		result = append(result, *proxies[name])
	}
	return result, nil
}

type timeSeriesAccumulator struct {
	succeeded int
	failed    int
	durations []int64
}

func operationListQuery(query operations.OperationQuery) (string, []any) {
	statement, arguments := operationQuery(`
SELECT id, type, status, started_at, finished_at, duration_ms, result, http_method, http_path, http_status, metadata, NULL AS details
FROM operations`, query)
	statement += " ORDER BY started_at DESC"
	if query.Limit > 0 {
		statement += " LIMIT ? OFFSET ?"
		arguments = append(arguments, query.Limit, query.Offset)
	}
	return statement, arguments
}

func operationCountQuery(query operations.OperationQuery) (string, []any) {
	return operationQuery("SELECT COUNT(*) FROM operations", query)
}

func operationQuery(statement string, query operations.OperationQuery) (string, []any) {
	filters := []string{"started_at >= ?", "started_at <= ?"}
	arguments := []any{timestamp(query.From), timestamp(query.To)}
	if query.Status != "" {
		filters = append(filters, "status = ?")
		arguments = append(arguments, query.Status)
	}
	if query.Type != "" {
		filters = append(filters, "type = ?")
		arguments = append(arguments, query.Type)
	}
	return statement + " WHERE " + strings.Join(filters, " AND "), arguments
}

func (s *Store) findOperation(ctx context.Context, id string) (operations.Operation, bool, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, type, status, started_at, finished_at, duration_ms, result, http_method, http_path, http_status, metadata, details
FROM operations
WHERE id = ?`, id)
	operation, err := scanOperation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return operations.Operation{}, false, nil
	}
	if err != nil {
		return operations.Operation{}, false, err
	}
	return operation, true, nil
}

func (s *Store) listSteps(ctx context.Context, operationID string) ([]operations.Step, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT step_id, operation_id, name, type, status, started_at, finished_at, duration_ms, result, provider, backend, proxy, metadata, details
FROM operation_steps
WHERE operation_id = ?
ORDER BY started_at`, operationID)
	if err != nil {
		return nil, fmt.Errorf("list operation steps: %w", err)
	}
	defer rows.Close()
	result := make([]operations.Step, 0)
	for rows.Next() {
		step, err := scanStep(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operation steps: %w", err)
	}
	return result, nil
}

func (s *Store) listErrors(ctx context.Context, operationID string) ([]operations.OperationError, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT operation_id, step_id, category, message, occurred_at
FROM operation_errors
WHERE operation_id = ?
ORDER BY occurred_at`, operationID)
	if err != nil {
		return nil, fmt.Errorf("list operation errors: %w", err)
	}
	defer rows.Close()
	result := make([]operations.OperationError, 0)
	for rows.Next() {
		operationError, err := scanOperationError(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, operationError)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operation errors: %w", err)
	}
	return result, nil
}

func (s *Store) operationDurations(ctx context.Context, dateRange operations.DashboardRange) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT duration_ms
FROM operations
WHERE started_at >= ? AND started_at <= ? AND finished_at IS NOT NULL AND duration_ms IS NOT NULL
ORDER BY duration_ms`, timestamp(dateRange.From), timestamp(dateRange.To))
	if err != nil {
		return nil, fmt.Errorf("list operation durations: %w", err)
	}
	defer rows.Close()
	var durations []int64
	for rows.Next() {
		var duration int64
		if err := rows.Scan(&duration); err != nil {
			return nil, fmt.Errorf("scan operation duration: %w", err)
		}
		durations = append(durations, duration)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operation durations: %w", err)
	}
	return durations, nil
}

func percentile(sorted []int64, value int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	sort.Slice(sorted, func(left, right int) bool { return sorted[left] < sorted[right] })
	index := (len(sorted)*value + 99) / 100
	if index == 0 {
		index = 1
	}
	return sorted[index-1]
}

func (s *Store) initialize(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, "PRAGMA journal_mode = WAL"); err != nil {
		return err
	}
	if err := migrate(ctx, s.db); err != nil {
		return err
	}
	return s.deleteExpiredDashboardSessions(ctx, time.Now())
}

func (s *Store) deleteExpiredDashboardSessions(ctx context.Context, cutoff time.Time) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM operations_dashboard_sessions WHERE expires_at <= ?", timestamp(cutoff))
	if err != nil {
		return fmt.Errorf("delete expired dashboard sessions: %w", err)
	}
	return nil
}

func databaseSource(path string) string {
	return (&url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: fmt.Sprintf("_pragma=busy_timeout(%d)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", busyTimeout.Milliseconds()),
	}).String()
}

func wrapWriteError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}

func timestamp(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func nullableTimestamp(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return timestamp(value)
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullableInt64(value int64) any {
	return value
}

func metadata(value map[string]string) any {
	if len(value) == 0 {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return string(encoded)
}

func details(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}

func stepName(step operations.Step) string {
	if step.Name != "" {
		return step.Name
	}
	return string(step.Type)
}

type scanner interface{ Scan(...any) error }

func scanOperation(row scanner) (operations.Operation, error) {
	var operation operations.Operation
	var startedAt string
	var finishedAt sql.NullString
	var durationMS sql.NullInt64
	var result, method, path, metadataValue, detailsValue sql.NullString
	var statusCode sql.NullInt64
	if err := row.Scan(&operation.ID, &operation.Type, &operation.Status, &startedAt, &finishedAt, &durationMS, &result, &method, &path, &statusCode, &metadataValue, &detailsValue); err != nil {
		return operations.Operation{}, fmt.Errorf("scan operation: %w", err)
	}
	var err error
	operation.StartedAt, err = time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return operations.Operation{}, fmt.Errorf("parse operation start: %w", err)
	}
	if finishedAt.Valid {
		operation.FinishedAt, err = time.Parse(time.RFC3339Nano, finishedAt.String)
		if err != nil {
			return operations.Operation{}, fmt.Errorf("parse operation finish: %w", err)
		}
	}
	if durationMS.Valid {
		operation.DurationMS = durationMS.Int64
	}
	operation.Result = operations.Result(result.String)
	operation.HTTPMethod = method.String
	operation.HTTPPath = path.String
	if statusCode.Valid {
		operation.HTTPStatus = int(statusCode.Int64)
	}
	operation.Metadata = parseMetadata(metadataValue)
	operation.Details = parseDetails(detailsValue)
	return operation, nil
}

func scanStep(row scanner) (operations.Step, error) {
	var step operations.Step
	var startedAt string
	var finishedAt, result, provider, backend, proxy, metadataValue, detailsValue sql.NullString
	var duration sql.NullInt64
	if err := row.Scan(&step.ID, &step.OperationID, &step.Name, &step.Type, &step.Status, &startedAt, &finishedAt, &duration, &result, &provider, &backend, &proxy, &metadataValue, &detailsValue); err != nil {
		return operations.Step{}, fmt.Errorf("scan operation step: %w", err)
	}
	parsedStartedAt, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return operations.Step{}, fmt.Errorf("parse operation step start: %w", err)
	}
	step.StartedAt = parsedStartedAt
	if finishedAt.Valid {
		step.FinishedAt, err = time.Parse(time.RFC3339Nano, finishedAt.String)
		if err != nil {
			return operations.Step{}, fmt.Errorf("parse operation step finish: %w", err)
		}
	}
	if duration.Valid {
		step.DurationMS = duration.Int64
	}
	step.Result = operations.Result(result.String)
	step.Provider, step.Backend, step.Proxy = provider.String, backend.String, proxy.String
	step.Metadata = parseMetadata(metadataValue)
	step.Details = parseDetails(detailsValue)
	return step, nil
}

func scanOperationError(row scanner) (operations.OperationError, error) {
	var operationError operations.OperationError
	var stepID, category sql.NullString
	var occurredAt string
	if err := row.Scan(&operationError.OperationID, &stepID, &category, &operationError.Message, &occurredAt); err != nil {
		return operations.OperationError{}, fmt.Errorf("scan operation error: %w", err)
	}
	parsedOccurredAt, err := time.Parse(time.RFC3339Nano, occurredAt)
	if err != nil {
		return operations.OperationError{}, fmt.Errorf("parse operation error time: %w", err)
	}
	operationError.StepID, operationError.Category, operationError.OccurredAt = stepID.String, operations.ErrorCategory(category.String), parsedOccurredAt
	return operationError, nil
}

func parseMetadata(value sql.NullString) map[string]string {
	if !value.Valid || value.String == "" {
		return nil
	}
	var metadata map[string]string
	if json.Unmarshal([]byte(value.String), &metadata) != nil {
		return nil
	}
	return metadata
}

func parseDetails(value sql.NullString) json.RawMessage {
	if !value.Valid || value.String == "" {
		return nil
	}
	return json.RawMessage(value.String)
}
