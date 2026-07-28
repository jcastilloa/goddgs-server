package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	operationsApplication "github.com/jcastilloa/goddgs-server/operations/application"
	operations "github.com/jcastilloa/goddgs-server/operations/domain"
)

func TestStorePersistsProbeResultsTransitionsAndLatency(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "operations.sqlite"))
	defer store.Close()
	monitor := operationsApplication.NewHealthMonitor(
		[]operationsApplication.ProbeTarget{{Name: "direct"}},
		operationsApplication.HealthMonitorConfig{SuccessThreshold: 1, FailureThreshold: 2},
		&sqliteHealthPool{},
		store,
		func() time.Time { return time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC) },
	)
	if err := monitor.ApplyProbe(context.Background(), "direct", operationsApplication.ProbeObservation{
		Success:    true,
		HTTPStatus: 204,
		Duration:   37 * time.Millisecond,
		ObservedAt: time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("ApplyProbe(success) error = %v", err)
	}
	if err := monitor.ApplyProbe(context.Background(), "direct", operationsApplication.ProbeObservation{
		HTTPStatus:    502,
		ErrorCategory: operations.ErrorUpstream5xx,
		Duration:      51 * time.Millisecond,
		ObservedAt:    time.Date(2026, time.July, 28, 12, 1, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("ApplyProbe(failure) error = %v", err)
	}

	var probeCount, transitionCount int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM proxy_probes").Scan(&probeCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow("SELECT COUNT(*) FROM proxy_health_transitions").Scan(&transitionCount); err != nil {
		t.Fatal(err)
	}
	if probeCount != 2 || transitionCount != 2 {
		t.Fatalf("counts = probes %d transitions %d, want 2 each", probeCount, transitionCount)
	}

	var status, result, category string
	var httpStatus int
	var durationMS int64
	if err := store.db.QueryRow(`
SELECT health_status, result, COALESCE(error_category, ''), http_status, duration_ms
FROM proxy_probes
ORDER BY id DESC
LIMIT 1`).Scan(&status, &result, &category, &httpStatus, &durationMS); err != nil {
		t.Fatal(err)
	}
	if status != string(operations.ProxyHealthDegraded) || result != string(operations.ResultFailed) || category != string(operations.ErrorUpstream5xx) || httpStatus != 502 || durationMS != 51 {
		t.Errorf("last probe = status %q result %q category %q HTTP %d duration %d", status, result, category, httpStatus, durationMS)
	}
}

type sqliteHealthPool struct{}

func (*sqliteHealthPool) MarkHealthy(string)   {}
func (*sqliteHealthPool) MarkUnhealthy(string) {}
