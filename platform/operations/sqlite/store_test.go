package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	operationsApplication "github.com/jcastilloa/goddgs-server/operations/application"
	operations "github.com/jcastilloa/goddgs-server/operations/domain"
)

func TestResolveDatabasePathUsesExplicitPathAndResolvedExecutableDirectory(t *testing.T) {
	explicit, err := ResolveDatabasePath(filepath.Join("data", "operations.sqlite"))
	if err != nil {
		t.Fatalf("ResolveDatabasePath() error = %v", err)
	}
	if !filepath.IsAbs(explicit) || filepath.Base(explicit) != "operations.sqlite" {
		t.Errorf("explicit path = %q", explicit)
	}

	temporaryDirectory := t.TempDir()
	target := filepath.Join(temporaryDirectory, "release", "goddgs-server")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(temporaryDirectory, "bin", "goddgs-server")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveDatabasePath("", func() (string, error) { return link, nil })
	if err != nil {
		t.Fatalf("resolveDatabasePath() error = %v", err)
	}
	want := filepath.Join(filepath.Dir(target), "operations.sqlite")
	if resolved != want {
		t.Errorf("default path = %q, want %q", resolved, want)
	}
}

func TestOpenReturnsPathInErrorWhenParentCannotContainDatabase(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parent, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "operations.sqlite")

	_, err := Open(context.Background(), Config{DatabasePath: path})
	if err == nil || !errors.Is(err, ErrOpen) || !contains(err.Error(), path) {
		t.Errorf("Open() error = %v, want path-aware ErrOpen", err)
	}
}

func TestOpenMigratesSchemaEnablesPragmasAndSupportsReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operations.sqlite")
	store := openStore(t, path)
	defer store.Close()

	for _, table := range []string{"schema_migrations", "operations", "operation_steps", "operation_errors", "proxy_probes", "proxy_health_transitions", "operations_dashboard_users", "operations_dashboard_sessions"} {
		var name string
		if err := store.db.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&name); err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
	for _, index := range []string{"operations_started_at_idx", "operations_finished_at_idx", "operations_status_idx", "operations_type_idx", "proxy_probes_name_observed_at_idx", "proxy_probes_observed_at_idx", "proxy_health_transitions_name_occurred_at_idx", "operations_dashboard_sessions_expires_at_idx"} {
		var name string
		if err := store.db.QueryRow("SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?", index).Scan(&name); err != nil {
			t.Errorf("index %q not found: %v", index, err)
		}
	}
	var foreignKeys int
	if err := store.db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, error = %v", foreignKeys, err)
	}
	var journalMode string
	if err := store.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil || journalMode != "wal" {
		t.Errorf("journal_mode = %q, error = %v", journalMode, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened := openStore(t, path); reopened == nil {
		t.Fatal("reopened store is nil")
	} else if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStorePersistsRecordsQueriesByTimeAndCascadesExpiredDependencies(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "operations.sqlite"))
	defer store.Close()

	now := time.Now().UTC().Truncate(time.Millisecond)
	expired := operations.Operation{ID: "expired", Type: "search", Status: operations.StatusSucceeded, StartedAt: now.Add(-48 * time.Hour), FinishedAt: now.Add(-47 * time.Hour)}
	recent := operations.Operation{ID: "recent", Type: "search", Status: operations.StatusFailed, StartedAt: now.Add(-time.Hour), FinishedAt: now.Add(-30 * time.Minute)}
	for _, operation := range []operations.Operation{expired, recent} {
		if err := store.CreateOperation(context.Background(), operation); err != nil {
			t.Fatalf("CreateOperation(%q) error = %v", operation.ID, err)
		}
	}
	if err := store.AddStep(context.Background(), operations.Step{OperationID: expired.ID, Name: "gateway", Status: operations.StatusSucceeded, StartedAt: expired.StartedAt, FinishedAt: expired.FinishedAt}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddError(context.Background(), operations.OperationError{OperationID: expired.ID, Message: "failed", OccurredAt: expired.FinishedAt}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordProbe(context.Background(), operations.ProxyProbe{ProxyName: "direct", Healthy: true, ObservedAt: now.Add(-48 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordHealthTransition(context.Background(), operations.ProxyHealthTransition{ProxyName: "direct", Healthy: false, OccurredAt: now.Add(-48 * time.Hour)}); err != nil {
		t.Fatal(err)
	}

	got, err := store.ListOperations(context.Background(), operations.OperationQuery{From: now.Add(-2 * time.Hour), To: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != recent.ID {
		t.Errorf("ListOperations() = %#v, want only %q", got, recent.ID)
	}
	if err := store.DeleteExpired(context.Background(), now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM operations", 1)
	assertCount(t, store.db, "SELECT COUNT(*) FROM operation_steps", 0)
	assertCount(t, store.db, "SELECT COUNT(*) FROM operation_errors", 0)
	assertCount(t, store.db, "SELECT COUNT(*) FROM proxy_probes", 0)
	assertCount(t, store.db, "SELECT COUNT(*) FROM proxy_health_transitions", 0)
}

func TestStorePersistsRecordedOperationDurationAndErrors(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "operations.sqlite"))
	defer store.Close()

	recorder := operationsApplication.NewEventRecorder(store, func() time.Time {
		return time.Date(2026, time.July, 28, 10, 0, 0, 0, time.UTC)
	}, func() string { return "operation-1" })
	ctx, err := recorder.StartOperation(context.Background(), operations.OperationStart{
		Type:   operations.OperationSearch,
		Method: "GET",
		Path:   "/v1/text",
		Metadata: map[string]string{
			"query": "topic",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.FinishOperation(ctx, operations.OperationFinish{HTTPStatus: http.StatusGatewayTimeout, Err: context.DeadlineExceeded}); err != nil {
		t.Fatal(err)
	}

	var status, result string
	var duration int64
	if err := store.db.QueryRow(`SELECT status, result, duration_ms FROM operations WHERE id = ?`, "operation-1").Scan(&status, &result, &duration); err != nil {
		t.Fatal(err)
	}
	if status != string(operations.StatusFailed) || result != string(operations.ResultTimeout) || duration != 0 {
		t.Errorf("persisted operation = status %q result %q duration %d", status, result, duration)
	}
}

func TestStoreUpdatesStepMetadataWhenSelectionFinishes(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "operations.sqlite"))
	defer store.Close()

	if err := store.CreateOperation(context.Background(), operations.Operation{ID: "research-1", Type: operations.OperationResearch, Status: operations.StatusRunning, StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddStep(context.Background(), operations.Step{
		ID:          "selection-1",
		OperationID: "research-1",
		Type:        operations.StepResearchSelection,
		Status:      operations.StatusRunning,
		StartedAt:   time.Now().UTC(),
		Metadata:    map[string]string{"candidates_found": "8", "candidates_submitted": "3", "candidates_selected": "0"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishStep(context.Background(), operations.Step{
		ID:         "selection-1",
		Status:     operations.StatusSucceeded,
		Result:     operations.ResultSucceeded,
		FinishedAt: time.Now().UTC(),
		Metadata:   map[string]string{"candidates_found": "8", "candidates_submitted": "3", "candidates_selected": "2"},
	}); err != nil {
		t.Fatal(err)
	}

	var metadataJSON string
	if err := store.db.QueryRow(`SELECT metadata FROM operation_steps WHERE step_id = ?`, "selection-1").Scan(&metadataJSON); err != nil {
		t.Fatal(err)
	}
	var metadata map[string]string
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if want := map[string]string{"candidates_found": "8", "candidates_submitted": "3", "candidates_selected": "2"}; !reflect.DeepEqual(metadata, want) {
		t.Errorf("metadata = %#v, want %#v", metadata, want)
	}
}

func TestStorePreservesMetadataWhenFinishingAnUnmodifiedStep(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "operations.sqlite"))
	defer store.Close()

	if err := store.CreateOperation(context.Background(), operations.Operation{ID: "research-1", Type: operations.OperationResearch, Status: operations.StatusRunning, StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddStep(context.Background(), operations.Step{
		ID:          "planning-1",
		OperationID: "research-1",
		Type:        operations.StepResearchPlanning,
		Status:      operations.StatusRunning,
		StartedAt:   time.Now().UTC(),
		Metadata:    map[string]string{"query": "research topic"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishStep(context.Background(), operations.Step{
		ID:         "planning-1",
		Status:     operations.StatusSucceeded,
		Result:     operations.ResultSucceeded,
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	var metadataJSON string
	if err := store.db.QueryRow(`SELECT metadata FROM operation_steps WHERE step_id = ?`, "planning-1").Scan(&metadataJSON); err != nil {
		t.Fatal(err)
	}
	var metadata map[string]string
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if want := map[string]string{"query": "research topic"}; !reflect.DeepEqual(metadata, want) {
		t.Errorf("metadata = %#v, want %#v", metadata, want)
	}
}

func TestStorePersistsCanceledOperationWithWithoutCancelContext(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "operations.sqlite"))
	defer store.Close()

	recorder := operationsApplication.NewEventRecorder(store, time.Now, func() string { return "operation-2" })
	ctx, cancel := context.WithCancel(context.Background())
	started, err := recorder.StartOperation(ctx, operations.OperationStart{Type: operations.OperationResearch, Method: "POST", Path: "/v1/research"})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := recorder.FinishOperation(started, operations.OperationFinish{HTTPStatus: 499, Err: context.Canceled}); err != nil {
		t.Fatal(err)
	}

	var status, result string
	if err := store.db.QueryRow(`SELECT status, result FROM operations WHERE id = ?`, "operation-2").Scan(&status, &result); err != nil {
		t.Fatal(err)
	}
	if status != string(operations.StatusFailed) || result != string(operations.ResultCanceled) {
		t.Errorf("persisted canceled operation = status %q result %q", status, result)
	}
}

func TestDeleteExpiredRollsBackTheBatchWhenProxyCleanupFails(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "operations.sqlite"))
	defer store.Close()

	expiredAt := time.Now().UTC().Add(-48 * time.Hour)
	if err := store.CreateOperation(context.Background(), operations.Operation{
		ID:         "expired",
		Type:       "search",
		Status:     operations.StatusSucceeded,
		StartedAt:  expiredAt,
		FinishedAt: expiredAt,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordProbe(context.Background(), operations.ProxyProbe{ProxyName: "direct", Healthy: true, ObservedAt: expiredAt}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
CREATE TRIGGER reject_probe_cleanup
BEFORE DELETE ON proxy_probes
BEGIN
  SELECT RAISE(ABORT, 'probe cleanup rejected');
END`); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteExpired(context.Background(), time.Now().UTC().Add(-24*time.Hour)); err == nil {
		t.Fatal("DeleteExpired() error = nil, want cleanup error")
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM operations", 1)
}

func TestDeleteExpiredRemovesEveryExpiredRecordAcrossBoundedBatches(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "operations.sqlite"))
	defer store.Close()

	if _, err := store.db.Exec(`
INSERT INTO proxy_probes (proxy_name, healthy, observed_at)
WITH RECURSIVE sequence(value) AS (
  VALUES(1)
  UNION ALL
  SELECT value + 1 FROM sequence WHERE value < 1001
)
SELECT 'direct', 1, ? FROM sequence`, timestamp(time.Now().UTC().Add(-48*time.Hour))); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteExpired(context.Background(), time.Now().UTC().Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM proxy_probes", 0)
}

func TestDashboardQueriesReturnOperationsSummarySeriesDetailAndProxies(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "operations.sqlite"))
	defer store.Close()

	from := time.Date(2026, time.July, 28, 8, 0, 0, 0, time.UTC)
	operationsToCreate := []operations.Operation{
		{ID: "succeeded", Type: operations.OperationSearch, Status: operations.StatusSucceeded, Result: operations.ResultSucceeded, StartedAt: from, FinishedAt: from.Add(100 * time.Millisecond), DurationMS: 100, HTTPMethod: "GET", HTTPPath: "/v1/text", Metadata: map[string]string{"query": "Go"}},
		{ID: "failed", Type: operations.OperationExtract, Status: operations.StatusFailed, Result: operations.ResultFailed, StartedAt: from.Add(time.Hour), FinishedAt: from.Add(time.Hour + 300*time.Millisecond), DurationMS: 300},
		{ID: "running", Type: operations.OperationResearch, Status: operations.StatusRunning, StartedAt: from.Add(2 * time.Hour)},
	}
	for _, operation := range operationsToCreate {
		if err := store.CreateOperation(context.Background(), operation); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.AddStep(context.Background(), operations.Step{ID: "step-1", OperationID: "failed", Type: operations.StepExtractAI, Status: operations.StatusFailed, StartedAt: from.Add(time.Hour), FinishedAt: from.Add(time.Hour + 300*time.Millisecond), DurationMS: 300, Provider: "openai-compatible"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddError(context.Background(), operations.OperationError{OperationID: "failed", StepID: "step-1", Category: operations.ErrorTimeout, Message: "request timed out", OccurredAt: from.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	for _, probe := range []operations.ProxyProbe{
		{ProxyName: "direct", Healthy: true, Status: operations.ProxyHealthHealthy, Result: operations.ResultSucceeded, Duration: 80 * time.Millisecond, ObservedAt: from},
		{ProxyName: "direct", Healthy: false, Status: operations.ProxyHealthUnhealthy, Result: operations.ResultFailed, Duration: 150 * time.Millisecond, ObservedAt: from.Add(time.Hour)},
	} {
		if err := store.RecordProbe(context.Background(), probe); err != nil {
			t.Fatal(err)
		}
	}

	dateRange := operations.DashboardRange{From: from.Add(-time.Minute), To: from.Add(3 * time.Hour)}
	summary, err := store.Summary(context.Background(), dateRange)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Active != 1 || summary.Succeeded != 1 || summary.Failed != 1 || summary.P50MS != 100 || summary.P95MS != 300 {
		t.Errorf("Summary() = %#v", summary)
	}

	page, err := operationsApplication.NewDashboardService(store).ListOperations(context.Background(), operations.OperationQuery{From: dateRange.From, To: dateRange.To, Limit: 2, Offset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Operations) != 2 || page.Operations[0].ID != "failed" || page.Operations[0].Metadata != nil || page.Operations[1].Metadata["query"] != "Go" {
		t.Errorf("ListOperations() = %#v", page)
	}

	detail, found, err := store.GetOperation(context.Background(), "failed")
	if err != nil {
		t.Fatal(err)
	}
	if !found || len(detail.Steps) != 1 || detail.Steps[0].ID != "step-1" || len(detail.Errors) != 1 || detail.Errors[0].Category != operations.ErrorTimeout {
		t.Errorf("GetOperation() = %#v, found = %t", detail, found)
	}

	series, err := store.TimeSeries(context.Background(), operations.TimeSeriesQuery{DashboardRange: dateRange, Interval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 5 || series[1].Succeeded != 1 || series[1].P50MS != 100 || series[2].Failed != 1 || series[2].P95MS != 300 || series[4].P50MS != 0 {
		t.Errorf("TimeSeries() = %#v", series)
	}

	proxies, err := store.ListProxies(context.Background(), dateRange)
	if err != nil {
		t.Fatal(err)
	}
	if len(proxies) != 1 || proxies[0].Name != "direct" || len(proxies[0].Points) != 2 || proxies[0].Status != operations.ProxyHealthUnhealthy {
		t.Errorf("ListProxies() = %#v", proxies)
	}
}

func TestDashboardQueriesReturnEmptyArraysAndExcludeRunningDurations(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "operations.sqlite"))
	defer store.Close()

	from := time.Date(2026, time.July, 28, 8, 0, 0, 0, time.UTC)
	dateRange := operations.DashboardRange{From: from, To: from.Add(2 * time.Hour)}
	query := operations.OperationQuery{From: dateRange.From, To: dateRange.To, Limit: 50}

	list, err := store.ListOperations(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if list == nil || len(list) != 0 {
		t.Errorf("ListOperations() = %#v, want non-nil empty list", list)
	}

	series, err := store.TimeSeries(context.Background(), operations.TimeSeriesQuery{DashboardRange: dateRange, Interval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if series == nil || len(series) != 0 {
		t.Errorf("TimeSeries() = %#v, want non-nil empty list", series)
	}

	proxies, err := store.ListProxies(context.Background(), dateRange)
	if err != nil {
		t.Fatal(err)
	}
	if proxies == nil || len(proxies) != 0 {
		t.Errorf("ListProxies() = %#v, want non-nil empty list", proxies)
	}

	for _, operation := range []operations.Operation{
		{ID: "running", Type: operations.OperationResearch, Status: operations.StatusRunning, StartedAt: from},
		{ID: "finished", Type: operations.OperationSearch, Status: operations.StatusSucceeded, Result: operations.ResultSucceeded, StartedAt: from.Add(time.Hour), FinishedAt: from.Add(time.Hour).Add(250 * time.Millisecond), DurationMS: 250},
		{ID: "without-details", Type: operations.OperationSearch, Status: operations.StatusSucceeded, Result: operations.ResultSucceeded, StartedAt: from.Add(2 * time.Hour), FinishedAt: from.Add(2*time.Hour + 50*time.Millisecond), DurationMS: 50},
	} {
		if err := store.CreateOperation(context.Background(), operation); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.AddStep(context.Background(), operations.Step{ID: "step-without-result", OperationID: "finished", Name: "gateway", Status: operations.StatusSucceeded, StartedAt: from.Add(time.Hour), FinishedAt: from.Add(time.Hour).Add(120 * time.Millisecond), DurationMS: 120}); err != nil {
		t.Fatal(err)
	}

	detail, found, err := store.GetOperation(context.Background(), "without-details")
	if err != nil {
		t.Fatal(err)
	}
	if !found || detail.Steps == nil || detail.Errors == nil || len(detail.Steps) != 0 || len(detail.Errors) != 0 {
		t.Errorf("GetOperation() = %#v, found = %t, want non-nil empty detail arrays", detail, found)
	}

	detail, found, err = store.GetOperation(context.Background(), "finished")
	if err != nil {
		t.Fatal(err)
	}
	if !found || len(detail.Steps) != 1 || detail.Steps[0].DurationMS != 120 {
		t.Errorf("GetOperation() = %#v, found = %t, want persisted step duration", detail, found)
	}

	series, err = store.TimeSeries(context.Background(), operations.TimeSeriesQuery{DashboardRange: dateRange, Interval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 3 || series[1].Succeeded != 1 || series[1].P50MS != 250 || series[1].P95MS != 250 {
		t.Errorf("TimeSeries() = %#v, want only the completed operation in percentile values", series)
	}
}

func openStore(t *testing.T, path string) *Store {
	t.Helper()
	store, err := Open(context.Background(), Config{DatabasePath: path})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	return store
}

func assertCount(t *testing.T, database *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := database.QueryRow(query).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("%s = %d, want %d", query, got, want)
	}
}

func contains(value, part string) bool { return strings.Contains(value, part) }
