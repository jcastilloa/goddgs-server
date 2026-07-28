package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

type migration struct {
	version int
	apply   func(context.Context, *sql.Tx) error
}

var migrations = []migration{
	{version: 1, apply: applyInitialSchema},
	{version: 2, apply: applyEventSchema},
	{version: 3, apply: applyProbeHealthSchema},
	{version: 4, apply: applyDashboardIndexes},
	{version: 5, apply: applyDashboardAuthSchema},
}

func applyDashboardIndexes(ctx context.Context, transaction *sql.Tx) error {
	_, err := transaction.ExecContext(ctx, "CREATE INDEX proxy_probes_observed_at_idx ON proxy_probes(observed_at)")
	return err
}

func applyDashboardAuthSchema(ctx context.Context, transaction *sql.Tx) error {
	statements := []string{
		`CREATE TABLE operations_dashboard_users (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
)`,
		`CREATE TABLE operations_dashboard_sessions (
  token_hash TEXT PRIMARY KEY,
  csrf_hash TEXT NOT NULL,
  user_id INTEGER NOT NULL REFERENCES operations_dashboard_users(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL
)`,
		"CREATE INDEX operations_dashboard_sessions_expires_at_idx ON operations_dashboard_sessions(expires_at)",
	}
	for _, statement := range statements {
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func migrate(ctx context.Context, database *sql.DB) error {
	if _, err := database.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY
)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	for _, migration := range migrations {
		var applied bool
		if err := database.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)", migration.version).Scan(&applied); err != nil {
			return fmt.Errorf("read migration %d: %w", migration.version, err)
		}
		if applied {
			continue
		}
		transaction, err := database.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("start migration %d: %w", migration.version, err)
		}
		if err := migration.apply(ctx, transaction); err != nil {
			_ = transaction.Rollback()
			return fmt.Errorf("apply migration %d: %w", migration.version, err)
		}
		if _, err := transaction.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES (?)", migration.version); err != nil {
			_ = transaction.Rollback()
			return fmt.Errorf("record migration %d: %w", migration.version, err)
		}
		if err := transaction.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", migration.version, err)
		}
	}
	return nil
}

func applyEventSchema(ctx context.Context, transaction *sql.Tx) error {
	statements := []string{
		"ALTER TABLE operations ADD COLUMN result TEXT",
		"ALTER TABLE operations ADD COLUMN http_method TEXT",
		"ALTER TABLE operations ADD COLUMN http_path TEXT",
		"ALTER TABLE operations ADD COLUMN http_status INTEGER",
		"ALTER TABLE operations ADD COLUMN duration_ms INTEGER",
		"ALTER TABLE operations ADD COLUMN metadata TEXT",
		"ALTER TABLE operation_steps ADD COLUMN step_id TEXT",
		"ALTER TABLE operation_steps ADD COLUMN type TEXT",
		"ALTER TABLE operation_steps ADD COLUMN result TEXT",
		"ALTER TABLE operation_steps ADD COLUMN provider TEXT",
		"ALTER TABLE operation_steps ADD COLUMN backend TEXT",
		"ALTER TABLE operation_steps ADD COLUMN proxy TEXT",
		"ALTER TABLE operation_steps ADD COLUMN duration_ms INTEGER",
		"ALTER TABLE operation_steps ADD COLUMN metadata TEXT",
		"ALTER TABLE operation_errors ADD COLUMN step_id TEXT",
		"ALTER TABLE operation_errors ADD COLUMN category TEXT",
		"CREATE UNIQUE INDEX operation_steps_step_id_idx ON operation_steps(step_id)",
	}
	for _, statement := range statements {
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func applyProbeHealthSchema(ctx context.Context, transaction *sql.Tx) error {
	statements := []string{
		"ALTER TABLE proxy_probes ADD COLUMN health_status TEXT",
		"ALTER TABLE proxy_probes ADD COLUMN result TEXT",
		"ALTER TABLE proxy_probes ADD COLUMN http_status INTEGER",
		"ALTER TABLE proxy_probes ADD COLUMN error_category TEXT",
		"ALTER TABLE proxy_probes ADD COLUMN duration_ms INTEGER",
		"ALTER TABLE proxy_health_transitions ADD COLUMN health_status TEXT",
	}
	for _, statement := range statements {
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func applyInitialSchema(ctx context.Context, transaction *sql.Tx) error {
	statements := []string{
		`CREATE TABLE operations (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  status TEXT NOT NULL,
  started_at TEXT NOT NULL,
  finished_at TEXT
)`,
		`CREATE TABLE operation_steps (
  id INTEGER PRIMARY KEY,
  operation_id TEXT NOT NULL REFERENCES operations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  status TEXT NOT NULL,
  started_at TEXT NOT NULL,
  finished_at TEXT
)`,
		`CREATE TABLE operation_errors (
  id INTEGER PRIMARY KEY,
  operation_id TEXT NOT NULL REFERENCES operations(id) ON DELETE CASCADE,
  message TEXT NOT NULL,
  occurred_at TEXT NOT NULL
)`,
		`CREATE TABLE proxy_probes (
  id INTEGER PRIMARY KEY,
  proxy_name TEXT NOT NULL,
  healthy INTEGER NOT NULL,
  observed_at TEXT NOT NULL
)`,
		`CREATE TABLE proxy_health_transitions (
  id INTEGER PRIMARY KEY,
  proxy_name TEXT NOT NULL,
  healthy INTEGER NOT NULL,
  occurred_at TEXT NOT NULL
)`,
		"CREATE INDEX operations_started_at_idx ON operations(started_at)",
		"CREATE INDEX operations_finished_at_idx ON operations(finished_at)",
		"CREATE INDEX operations_status_idx ON operations(status)",
		"CREATE INDEX operations_type_idx ON operations(type)",
		"CREATE INDEX operation_steps_operation_id_idx ON operation_steps(operation_id)",
		"CREATE INDEX operation_errors_operation_id_idx ON operation_errors(operation_id)",
		"CREATE INDEX proxy_probes_name_observed_at_idx ON proxy_probes(proxy_name, observed_at)",
		"CREATE INDEX proxy_health_transitions_name_occurred_at_idx ON proxy_health_transitions(proxy_name, occurred_at)",
	}
	for _, statement := range statements {
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
