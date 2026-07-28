package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	operations "github.com/jcastilloa/goddgs-server/operations/domain"

	"golang.org/x/crypto/argon2"
)

const (
	minDashboardUsernameLength = 3
	maxDashboardUsernameLength = 64
	minDashboardPasswordLength = 12
	maxDashboardPasswordLength = 128
	randomTokenBytes           = 32
	argonMemory                = 19 * 1024
	argonIterations            = 2
	argonParallelism           = 1
	argonSaltBytes             = 16
	argonKeyBytes              = 32
	unknownPasswordSalt        = "dashboard-auth00"
)

type DashboardAuthConfig struct {
	SessionTTL time.Duration
	Now        func() time.Time
	Random     func(int) ([]byte, error)
}

type DashboardCredentials struct {
	Username  string
	Token     string
	CSRFToken string
	ExpiresAt time.Time
}

type DashboardSessionIdentity struct {
	UserID    int64
	Username  string
	TokenHash string
	CSRFHash  string
	ExpiresAt time.Time
}

type DashboardAuthService struct {
	repository operations.DashboardAuthRepository
	ttl        time.Duration
	now        func() time.Time
	random     func(int) ([]byte, error)
}

func NewDashboardAuthService(repository operations.DashboardAuthRepository, config DashboardAuthConfig) *DashboardAuthService {
	if config.SessionTTL <= 0 {
		config.SessionTTL = 12 * time.Hour
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Random == nil {
		config.Random = randomBytes
	}
	return &DashboardAuthService{repository: repository, ttl: config.SessionTTL, now: config.Now, random: config.Random}
}

func (s *DashboardAuthService) NeedsSetup(ctx context.Context) (bool, error) {
	exists, err := s.repository.HasDashboardUser(ctx)
	return !exists, err
}

func (s *DashboardAuthService) Setup(ctx context.Context, username, password string) (DashboardCredentials, error) {
	username, password, err := validateDashboardCredentials(username, password)
	if err != nil {
		return DashboardCredentials{}, err
	}
	hash, err := hashPassword(password, s.random)
	if err != nil {
		return DashboardCredentials{}, err
	}
	now := s.now().UTC()
	user, err := s.repository.CreateDashboardUser(ctx, operations.DashboardUser{Username: username, PasswordHash: hash, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return DashboardCredentials{}, err
	}
	return s.newSession(ctx, user, now)
}

func (s *DashboardAuthService) Login(ctx context.Context, username, password string) (DashboardCredentials, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	user, found, err := s.repository.FindDashboardUserByUsername(ctx, username)
	if err != nil {
		return DashboardCredentials{}, err
	}
	if !found {
		consumePasswordVerification(password)
		return DashboardCredentials{}, operations.ErrInvalidDashboardCredentials
	}
	if !verifyPassword(user.PasswordHash, password) {
		return DashboardCredentials{}, operations.ErrInvalidDashboardCredentials
	}
	return s.newSession(ctx, user, s.now().UTC())
}

func (s *DashboardAuthService) Authenticate(ctx context.Context, token string) (DashboardSessionIdentity, error) {
	tokenHash := tokenDigest(token)
	if tokenHash == "" {
		return DashboardSessionIdentity{}, operations.ErrDashboardSessionNotFound
	}
	session, found, err := s.repository.FindDashboardSession(ctx, tokenHash)
	if err != nil {
		return DashboardSessionIdentity{}, err
	}
	if !found {
		return DashboardSessionIdentity{}, operations.ErrDashboardSessionNotFound
	}
	if !session.ExpiresAt.After(s.now().UTC()) {
		_ = s.repository.DeleteDashboardSession(ctx, tokenHash)
		return DashboardSessionIdentity{}, operations.ErrDashboardSessionExpired
	}
	return DashboardSessionIdentity{UserID: session.UserID, Username: session.Username, TokenHash: session.TokenHash, CSRFHash: session.CSRFHash, ExpiresAt: session.ExpiresAt}, nil
}

func (s *DashboardAuthService) ValidCSRF(session DashboardSessionIdentity, token string) bool {
	digest := tokenDigest(token)
	return digest != "" && subtle.ConstantTimeCompare([]byte(session.CSRFHash), []byte(digest)) == 1
}

func (s *DashboardAuthService) Logout(ctx context.Context, session DashboardSessionIdentity) error {
	return s.repository.DeleteDashboardSession(ctx, session.TokenHash)
}

func (s *DashboardAuthService) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) (DashboardCredentials, error) {
	user, found, err := s.findUser(ctx, userID)
	if err != nil {
		return DashboardCredentials{}, err
	}
	if !found || !verifyPassword(user.PasswordHash, strings.TrimSpace(currentPassword)) {
		return DashboardCredentials{}, operations.ErrInvalidDashboardCredentials
	}
	_, newPassword, err = validateDashboardCredentials(user.Username, newPassword)
	if err != nil {
		return DashboardCredentials{}, err
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(currentPassword)), []byte(newPassword)) == 1 {
		return DashboardCredentials{}, operations.ErrDashboardPasswordUnchanged
	}
	passwordHash, err := hashPassword(newPassword, s.random)
	if err != nil {
		return DashboardCredentials{}, err
	}
	credentials, session, err := s.sessionFor(user, s.now().UTC())
	if err != nil {
		return DashboardCredentials{}, err
	}
	if err := s.repository.ReplaceDashboardPasswordAndSession(ctx, user.ID, passwordHash, session); err != nil {
		return DashboardCredentials{}, err
	}
	return credentials, nil
}

func (s *DashboardAuthService) newSession(ctx context.Context, user operations.DashboardUser, now time.Time) (DashboardCredentials, error) {
	credentials, session, err := s.sessionFor(user, now)
	if err != nil {
		return DashboardCredentials{}, err
	}
	if err := s.repository.CreateDashboardSession(ctx, session); err != nil {
		return DashboardCredentials{}, err
	}
	return credentials, nil
}

func (s *DashboardAuthService) sessionFor(user operations.DashboardUser, now time.Time) (DashboardCredentials, operations.DashboardSession, error) {
	token, err := newToken(s.random)
	if err != nil {
		return DashboardCredentials{}, operations.DashboardSession{}, err
	}
	csrfToken, err := newToken(s.random)
	if err != nil {
		return DashboardCredentials{}, operations.DashboardSession{}, err
	}
	expiresAt := now.Add(s.ttl)
	credentials := DashboardCredentials{Username: user.Username, Token: token, CSRFToken: csrfToken, ExpiresAt: expiresAt}
	session := operations.DashboardSession{TokenHash: tokenDigest(token), CSRFHash: tokenDigest(csrfToken), UserID: user.ID, Username: user.Username, CreatedAt: now, ExpiresAt: expiresAt}
	return credentials, session, nil
}

func (s *DashboardAuthService) findUser(ctx context.Context, id int64) (operations.DashboardUser, bool, error) {
	return s.repository.FindDashboardUserByID(ctx, id)
}

func validateDashboardCredentials(username, password string) (string, string, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	passwordLength := utf8.RuneCountInString(password)
	if !validUsername(username) || passwordLength < minDashboardPasswordLength || passwordLength > maxDashboardPasswordLength {
		return "", "", operations.ErrInvalidDashboardInput
	}
	return username, password, nil
}

func validUsername(username string) bool {
	if len(username) < minDashboardUsernameLength || len(username) > maxDashboardUsernameLength {
		return false
	}
	for _, value := range username {
		if isASCIIAlphaNumeric(value) || value == '.' || value == '_' || value == '-' {
			continue
		}
		return false
	}
	return true
}

func isASCIIAlphaNumeric(value rune) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func hashPassword(password string, random func(int) ([]byte, error)) (string, error) {
	salt, err := random(argonSaltBytes)
	if err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyBytes)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonIterations, argonParallelism, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func consumePasswordVerification(password string) {
	argon2.IDKey([]byte(password), []byte(unknownPasswordSalt), argonIterations, argonMemory, argonParallelism, argonKeyBytes)
}

func newToken(random func(int) ([]byte, error)) (string, error) {
	value, err := random(randomTokenBytes)
	if err != nil {
		return "", fmt.Errorf("generate dashboard token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func tokenDigest(token string) string {
	if strings.TrimSpace(token) == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(token))
	return base64.RawStdEncoding.EncodeToString(digest[:])
}

func randomBytes(size int) ([]byte, error) {
	value := make([]byte, size)
	_, err := rand.Read(value)
	return value, err
}
