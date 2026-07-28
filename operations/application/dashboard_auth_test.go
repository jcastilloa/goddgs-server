package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	operations "github.com/jcastilloa/goddgs-server/operations/domain"
)

func TestDashboardAuthSetupLoginAndPasswordChange(t *testing.T) {
	repository := &dashboardAuthRepository{}
	service := newDashboardAuthServiceForTest(repository)

	credentials, err := service.Setup(context.Background(), "  operator  ", "a secure password")
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	if credentials.Username != "operator" || credentials.Token == "" || credentials.CSRFToken == "" {
		t.Errorf("Setup() credentials = %#v", credentials)
	}
	if repository.user.PasswordHash == "a secure password" {
		t.Fatal("Setup() persisted plaintext password")
	}

	if _, err := service.Setup(context.Background(), "second", "another secure password"); !errors.Is(err, operations.ErrDashboardSetupCompleted) {
		t.Errorf("second Setup() error = %v, want ErrDashboardSetupCompleted", err)
	}
	if _, err := service.Login(context.Background(), "operator", "wrong password"); !errors.Is(err, operations.ErrInvalidDashboardCredentials) {
		t.Errorf("Login() invalid password error = %v, want ErrInvalidDashboardCredentials", err)
	}

	login, err := service.Login(context.Background(), "operator", "a secure password")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	session, err := service.Authenticate(context.Background(), login.Token)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if session.Username != "operator" {
		t.Errorf("Authenticate() username = %q, want operator", session.Username)
	}
	if !service.ValidCSRF(session, login.CSRFToken) {
		t.Error("ValidCSRF() = false, want true")
	}

	changed, err := service.ChangePassword(context.Background(), session.UserID, "a secure password", "a different secure password")
	if err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}
	if _, err := service.Authenticate(context.Background(), login.Token); !errors.Is(err, operations.ErrDashboardSessionNotFound) {
		t.Errorf("Authenticate() old session error = %v, want ErrDashboardSessionNotFound", err)
	}
	if _, err := service.Login(context.Background(), "operator", "a secure password"); !errors.Is(err, operations.ErrInvalidDashboardCredentials) {
		t.Errorf("Login() old password error = %v, want ErrInvalidDashboardCredentials", err)
	}
	if _, err := service.Authenticate(context.Background(), changed.Token); err != nil {
		t.Errorf("Authenticate() replacement session error = %v", err)
	}
}

func TestDashboardAuthRejectsInvalidCredentialsAndExpiredSessions(t *testing.T) {
	repository := &dashboardAuthRepository{}
	service := newDashboardAuthServiceForTest(repository)

	for _, testCase := range []struct {
		username string
		password string
	}{
		{username: "no spaces", password: "a secure password"},
		{username: "ok", password: "a secure password"},
		{username: "operator", password: "short"},
		{username: "operator", password: strings.Repeat("á", 6)},
	} {
		if _, err := service.Setup(context.Background(), testCase.username, testCase.password); !errors.Is(err, operations.ErrInvalidDashboardInput) {
			t.Errorf("Setup(%q) error = %v, want ErrInvalidDashboardInput", testCase.username, err)
		}
	}

	credentials, err := service.Setup(context.Background(), "operator", "a secure password")
	if err != nil {
		t.Fatal(err)
	}
	repository.session.ExpiresAt = time.Date(2026, time.July, 28, 11, 59, 59, 0, time.UTC)
	if _, err := service.Authenticate(context.Background(), credentials.Token); !errors.Is(err, operations.ErrDashboardSessionExpired) {
		t.Errorf("Authenticate() expired error = %v, want ErrDashboardSessionExpired", err)
	}
}

func TestDashboardAuthRejectsAnUnchangedPasswordWithoutRevokingTheSession(t *testing.T) {
	repository := &dashboardAuthRepository{}
	service := newDashboardAuthServiceForTest(repository)

	credentials, err := service.Setup(context.Background(), "operator", "a secure password")
	if err != nil {
		t.Fatal(err)
	}
	session, err := service.Authenticate(context.Background(), credentials.Token)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.ChangePassword(context.Background(), session.UserID, "a secure password", "a secure password")
	if !errors.Is(err, operations.ErrDashboardPasswordUnchanged) {
		t.Errorf("ChangePassword() error = %v, want ErrDashboardPasswordUnchanged", err)
	}
	if _, err := service.Authenticate(context.Background(), credentials.Token); err != nil {
		t.Errorf("Authenticate() after rejected password change error = %v", err)
	}
}

func newDashboardAuthServiceForTest(repository *dashboardAuthRepository) *DashboardAuthService {
	var sequence byte
	return NewDashboardAuthService(repository, DashboardAuthConfig{
		SessionTTL: time.Hour,
		Now:        func() time.Time { return time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC) },
		Random: func(size int) ([]byte, error) {
			sequence++
			return bytesOf(size, sequence), nil
		},
	})
}

func bytesOf(size int, value byte) []byte {
	result := make([]byte, size)
	for index := range result {
		result[index] = value
	}
	return result
}

type dashboardAuthRepository struct {
	user    operations.DashboardUser
	session operations.DashboardSession
}

func (r *dashboardAuthRepository) HasDashboardUser(context.Context) (bool, error) {
	return r.user.ID != 0, nil
}

func (r *dashboardAuthRepository) FindDashboardUserByID(_ context.Context, id int64) (operations.DashboardUser, bool, error) {
	return r.user, r.user.ID == id, nil
}

func (r *dashboardAuthRepository) CreateDashboardUser(_ context.Context, user operations.DashboardUser) (operations.DashboardUser, error) {
	if r.user.ID != 0 {
		return operations.DashboardUser{}, operations.ErrDashboardSetupCompleted
	}
	user.ID = 1
	r.user = user
	return user, nil
}

func (r *dashboardAuthRepository) FindDashboardUserByUsername(_ context.Context, username string) (operations.DashboardUser, bool, error) {
	return r.user, r.user.ID != 0 && r.user.Username == username, nil
}

func (r *dashboardAuthRepository) CreateDashboardSession(_ context.Context, session operations.DashboardSession) error {
	r.session = session
	return nil
}

func (r *dashboardAuthRepository) FindDashboardSession(_ context.Context, tokenHash string) (operations.DashboardSession, bool, error) {
	return r.session, r.session.TokenHash == tokenHash, nil
}

func (r *dashboardAuthRepository) DeleteDashboardSession(_ context.Context, tokenHash string) error {
	if r.session.TokenHash == tokenHash {
		r.session = operations.DashboardSession{}
	}
	return nil
}

func (r *dashboardAuthRepository) ReplaceDashboardPasswordAndSession(_ context.Context, userID int64, passwordHash string, session operations.DashboardSession) error {
	if r.user.ID != userID {
		return operations.ErrDashboardSessionNotFound
	}
	r.user.PasswordHash = passwordHash
	r.session = session
	return nil
}

var _ operations.DashboardAuthRepository = (*dashboardAuthRepository)(nil)
