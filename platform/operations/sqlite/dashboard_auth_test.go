package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	operations "github.com/jcastilloa/goddgs-server/operations/domain"
)

func TestStorePersistsDashboardUserAndRevocableSession(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "operations.sqlite"))
	defer store.Close()

	user, err := store.CreateDashboardUser(context.Background(), operations.DashboardUser{
		Username:     "operator",
		PasswordHash: "$argon2id$example",
		CreatedAt:    time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateDashboardUser() error = %v", err)
	}
	if user.ID == 0 {
		t.Fatal("CreateDashboardUser() did not assign an ID")
	}
	if _, err := store.CreateDashboardUser(context.Background(), operations.DashboardUser{Username: "other", PasswordHash: "hash"}); !errors.Is(err, operations.ErrDashboardSetupCompleted) {
		t.Errorf("second CreateDashboardUser() error = %v, want ErrDashboardSetupCompleted", err)
	}

	session := operations.DashboardSession{
		TokenHash: "token-hash", CSRFHash: "csrf-hash", UserID: user.ID, Username: user.Username,
		CreatedAt: time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC), ExpiresAt: time.Date(2026, time.July, 28, 13, 0, 0, 0, time.UTC),
	}
	if err := store.CreateDashboardSession(context.Background(), session); err != nil {
		t.Fatalf("CreateDashboardSession() error = %v", err)
	}
	got, found, err := store.FindDashboardSession(context.Background(), session.TokenHash)
	if err != nil || !found || got.Username != user.Username || got.CSRFHash != session.CSRFHash {
		t.Errorf("FindDashboardSession() = %#v, %t, %v", got, found, err)
	}
	if err := store.DeleteDashboardSession(context.Background(), session.TokenHash); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.FindDashboardSession(context.Background(), session.TokenHash); err != nil || found {
		t.Errorf("session after deletion found = %t, error = %v", found, err)
	}
}

func TestStoreCreatesOnlyOneDashboardUserDuringConcurrentSetup(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "operations.sqlite"))
	defer store.Close()

	start := make(chan struct{})
	errorsByAttempt := make(chan error, 2)
	var group sync.WaitGroup
	for _, username := range []string{"operator-one", "operator-two"} {
		group.Add(1)
		go func(username string) {
			defer group.Done()
			<-start
			_, err := store.CreateDashboardUser(context.Background(), operations.DashboardUser{
				Username:     username,
				PasswordHash: "$argon2id$example",
				CreatedAt:    time.Now().UTC(),
				UpdatedAt:    time.Now().UTC(),
			})
			errorsByAttempt <- err
		}(username)
	}
	close(start)
	group.Wait()
	close(errorsByAttempt)

	var created, rejected int
	for err := range errorsByAttempt {
		switch {
		case err == nil:
			created++
		case errors.Is(err, operations.ErrDashboardSetupCompleted):
			rejected++
		default:
			t.Fatalf("CreateDashboardUser() error = %v", err)
		}
	}
	if created != 1 || rejected != 1 {
		t.Errorf("created = %d rejected = %d, want one of each", created, rejected)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM operations_dashboard_users", 1)
}

func TestStoreCleansExpiredDashboardSessionsOnSessionCreationAndOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operations.sqlite")
	store := openStore(t, path)

	now := time.Now().UTC()
	user, err := store.CreateDashboardUser(context.Background(), operations.DashboardUser{
		Username: "operator", PasswordHash: "$argon2id$example", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateDashboardSession(context.Background(), operations.DashboardSession{
		TokenHash: "expired", CSRFHash: "csrf-expired", UserID: user.ID, CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateDashboardSession(context.Background(), operations.DashboardSession{
		TokenHash: "active", CSRFHash: "csrf-active", UserID: user.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	assertCount(t, store.db, "SELECT COUNT(*) FROM operations_dashboard_sessions", 1)

	if _, err := store.db.Exec("INSERT INTO operations_dashboard_sessions (token_hash, csrf_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?, ?)", "expired-on-open", "csrf-expired-on-open", user.ID, timestamp(now.Add(-2*time.Hour)), timestamp(now.Add(-time.Hour))); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openStore(t, path)
	defer store.Close()
	assertCount(t, store.db, "SELECT COUNT(*) FROM operations_dashboard_sessions", 1)
}
