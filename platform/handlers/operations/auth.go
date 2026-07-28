package operations

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	operationsApplication "github.com/jcastilloa/goddgs-server/operations/application"
	operationsDomain "github.com/jcastilloa/goddgs-server/operations/domain"

	"github.com/gin-gonic/gin"
)

const (
	SetupPageHandlerLabel   = "handler.operations.setup-page"
	SetupHandlerLabel       = "handler.operations.setup"
	LoginPageHandlerLabel   = "handler.operations.login-page"
	LoginHandlerLabel       = "handler.operations.login"
	SessionHandlerLabel     = "handler.operations.session"
	LogoutHandlerLabel      = "handler.operations.logout"
	PasswordHandlerLabel    = "handler.operations.password"
	dashboardSessionCookie  = "operations_session"
	dashboardCSRFCookie     = "operations_csrf"
	csrfHeader              = "X-Operations-CSRF"
	maxAuthRequestBodyBytes = 1 << 10
)

type DashboardAuthUseCase interface {
	NeedsSetup(context.Context) (bool, error)
	Setup(context.Context, string, string) (operationsApplication.DashboardCredentials, error)
	Login(context.Context, string, string) (operationsApplication.DashboardCredentials, error)
	Authenticate(context.Context, string) (operationsApplication.DashboardSessionIdentity, error)
	ValidCSRF(operationsApplication.DashboardSessionIdentity, string) bool
	Logout(context.Context, operationsApplication.DashboardSessionIdentity) error
	ChangePassword(context.Context, int64, string, string) (operationsApplication.DashboardCredentials, error)
}

type authHandler struct {
	useCase      DashboardAuthUseCase
	cookieSecure bool
}

type credentialsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type passwordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type Setup struct{ authHandler }

func NewSetup(useCase DashboardAuthUseCase, cookieSecure bool) Setup {
	return Setup{authHandler{useCase: useCase, cookieSecure: cookieSecure}}
}

func (h Setup) Handle(ctx *gin.Context) {
	if h.redirectAuthenticated(ctx) {
		return
	}
	needsSetup, err := h.useCase.NeedsSetup(ctx.Request.Context())
	if err != nil {
		writeAuthError(ctx, err)
		return
	}
	if !needsSetup {
		ctx.Redirect(http.StatusSeeOther, "/operations/login")
		return
	}
	ctx.Data(http.StatusOK, "text/html; charset=utf-8", authPageHTML("Create dashboard access", "Create the local operator account that protects this dashboard.", "Create account", "setup"))
}

type LoginPage struct{ authHandler }

func NewLoginPage(useCase DashboardAuthUseCase, cookieSecure bool) LoginPage {
	return LoginPage{authHandler{useCase: useCase, cookieSecure: cookieSecure}}
}

func (h LoginPage) Handle(ctx *gin.Context) {
	if h.redirectAuthenticated(ctx) {
		return
	}
	needsSetup, err := h.useCase.NeedsSetup(ctx.Request.Context())
	if err != nil {
		writeAuthError(ctx, err)
		return
	}
	if needsSetup {
		ctx.Redirect(http.StatusSeeOther, "/operations/setup")
		return
	}
	ctx.Data(http.StatusOK, "text/html; charset=utf-8", authPageHTML("Welcome back", "Sign in to view protected operational telemetry.", "Sign in", "login"))
}

func (h authHandler) redirectAuthenticated(ctx *gin.Context) bool {
	session, err := h.session(ctx)
	if err != nil {
		if errors.Is(err, operationsDomain.ErrDashboardSessionNotFound) || errors.Is(err, operationsDomain.ErrDashboardSessionExpired) {
			h.clearSessionCookies(ctx)
			return false
		}
		writeAuthError(ctx, err)
		return true
	}
	if session.Username == "" {
		return false
	}
	ctx.Redirect(http.StatusSeeOther, "/operations")
	return true
}

type Login struct{ authHandler }

func NewLogin(useCase DashboardAuthUseCase, cookieSecure bool) Login {
	return Login{authHandler{useCase: useCase, cookieSecure: cookieSecure}}
}

func (h Login) Handle(ctx *gin.Context) {
	var request credentialsRequest
	if !bindJSON(ctx, &request) {
		return
	}
	credentials, err := h.useCase.Login(ctx.Request.Context(), request.Username, request.Password)
	if err != nil {
		writeAuthError(ctx, err)
		return
	}
	h.setSessionCookies(ctx, credentials)
	ctx.JSON(http.StatusOK, gin.H{"username": credentials.Username})
}

type SetupAccount struct{ authHandler }

func NewSetupAccount(useCase DashboardAuthUseCase, cookieSecure bool) SetupAccount {
	return SetupAccount{authHandler{useCase: useCase, cookieSecure: cookieSecure}}
}

func (h SetupAccount) Handle(ctx *gin.Context) {
	var request credentialsRequest
	if !bindJSON(ctx, &request) {
		return
	}
	credentials, err := h.useCase.Setup(ctx.Request.Context(), request.Username, request.Password)
	if err != nil {
		writeAuthError(ctx, err)
		return
	}
	h.setSessionCookies(ctx, credentials)
	ctx.JSON(http.StatusCreated, gin.H{"username": credentials.Username})
}

type Session struct{ authHandler }

func NewSession(useCase DashboardAuthUseCase, cookieSecure bool) Session {
	return Session{authHandler{useCase: useCase, cookieSecure: cookieSecure}}
}

func (h Session) Handle(ctx *gin.Context) {
	session, ok := h.requireSession(ctx)
	if !ok {
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"username": session.Username})
}

type Logout struct{ authHandler }

func NewLogout(useCase DashboardAuthUseCase, cookieSecure bool) Logout {
	return Logout{authHandler{useCase: useCase, cookieSecure: cookieSecure}}
}

func (h Logout) Handle(ctx *gin.Context) {
	session, ok := h.requireSession(ctx)
	if !ok {
		return
	}
	if !h.validCSRF(ctx, session) {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "invalid dashboard CSRF token"})
		return
	}
	if err := h.useCase.Logout(ctx.Request.Context(), session); err != nil {
		writeAuthError(ctx, err)
		return
	}
	h.clearSessionCookies(ctx)
	ctx.Status(http.StatusNoContent)
}

type Password struct{ authHandler }

func NewPassword(useCase DashboardAuthUseCase, cookieSecure bool) Password {
	return Password{authHandler{useCase: useCase, cookieSecure: cookieSecure}}
}

func (h Password) Handle(ctx *gin.Context) {
	session, ok := h.requireSession(ctx)
	if !ok {
		return
	}
	if !h.validCSRF(ctx, session) {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "invalid dashboard CSRF token"})
		return
	}
	var request passwordRequest
	if !bindJSON(ctx, &request) {
		return
	}
	if strings.TrimSpace(request.CurrentPassword) == "" || strings.TrimSpace(request.NewPassword) == "" {
		writeAuthError(ctx, operationsDomain.ErrInvalidDashboardInput)
		return
	}
	credentials, err := h.useCase.ChangePassword(ctx.Request.Context(), session.UserID, request.CurrentPassword, request.NewPassword)
	if err != nil {
		writeAuthError(ctx, err)
		return
	}
	h.setSessionCookies(ctx, credentials)
	ctx.Status(http.StatusNoContent)
}

func (h authHandler) session(ctx *gin.Context) (operationsApplication.DashboardSessionIdentity, error) {
	token, err := ctx.Cookie(dashboardSessionCookie)
	if err != nil {
		return operationsApplication.DashboardSessionIdentity{}, operationsDomain.ErrDashboardSessionNotFound
	}
	return h.useCase.Authenticate(ctx.Request.Context(), token)
}

func (h authHandler) requireSession(ctx *gin.Context) (operationsApplication.DashboardSessionIdentity, bool) {
	session, err := h.session(ctx)
	if err == nil {
		return session, true
	}
	if errors.Is(err, operationsDomain.ErrDashboardSessionNotFound) || errors.Is(err, operationsDomain.ErrDashboardSessionExpired) {
		h.clearSessionCookies(ctx)
		writeAuthenticationRequired(ctx)
		return operationsApplication.DashboardSessionIdentity{}, false
	}
	writeAuthError(ctx, err)
	return operationsApplication.DashboardSessionIdentity{}, false
}

func (h authHandler) validCSRF(ctx *gin.Context, session operationsApplication.DashboardSessionIdentity) bool {
	cookie, err := ctx.Cookie(dashboardCSRFCookie)
	if err != nil || !h.useCase.ValidCSRF(session, cookie) {
		return false
	}
	return h.useCase.ValidCSRF(session, ctx.GetHeader(csrfHeader)) && strings.TrimSpace(cookie) == strings.TrimSpace(ctx.GetHeader(csrfHeader))
}

func (h authHandler) setSessionCookies(ctx *gin.Context, credentials operationsApplication.DashboardCredentials) {
	maxAge := int(time.Until(credentials.ExpiresAt).Seconds())
	ctx.SetSameSite(http.SameSiteStrictMode)
	ctx.SetCookie(dashboardSessionCookie, credentials.Token, maxAge, "/operations", "", h.cookieSecure, true)
	ctx.SetCookie(dashboardCSRFCookie, credentials.CSRFToken, maxAge, "/operations", "", h.cookieSecure, false)
}

func (h authHandler) clearSessionCookies(ctx *gin.Context) {
	ctx.SetSameSite(http.SameSiteStrictMode)
	ctx.SetCookie(dashboardSessionCookie, "", -1, "/operations", "", h.cookieSecure, true)
	ctx.SetCookie(dashboardCSRFCookie, "", -1, "/operations", "", h.cookieSecure, false)
}

func bindJSON(ctx *gin.Context, target any) bool {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxAuthRequestBodyBytes)
	if err := ctx.ShouldBindJSON(target); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid dashboard authentication request"})
		return false
	}
	return true
}

func writeAuthError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, operationsDomain.ErrDashboardSetupCompleted):
		ctx.JSON(http.StatusConflict, gin.H{"error": "setup_completed"})
	case errors.Is(err, operationsDomain.ErrInvalidDashboardInput):
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "username must be 3-64 ASCII letters, digits, dots, underscores, or hyphens; password must be 12-128 characters"})
	case errors.Is(err, operationsDomain.ErrDashboardPasswordUnchanged):
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "new dashboard password must be different from the current password"})
	case errors.Is(err, operationsDomain.ErrInvalidDashboardCredentials):
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid dashboard credentials"})
	default:
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "operations dashboard authentication is unavailable"})
	}
}

func writeAuthenticationRequired(ctx *gin.Context) {
	ctx.JSON(http.StatusUnauthorized, gin.H{"error": "dashboard authentication required"})
}

type EmptyDashboardAuthUseCase struct{}

func (EmptyDashboardAuthUseCase) NeedsSetup(context.Context) (bool, error) {
	return true, nil
}
func (EmptyDashboardAuthUseCase) Setup(context.Context, string, string) (operationsApplication.DashboardCredentials, error) {
	return operationsApplication.DashboardCredentials{}, errors.New("dashboard auth unavailable")
}
func (EmptyDashboardAuthUseCase) Login(context.Context, string, string) (operationsApplication.DashboardCredentials, error) {
	return operationsApplication.DashboardCredentials{}, errors.New("dashboard auth unavailable")
}
func (EmptyDashboardAuthUseCase) Authenticate(context.Context, string) (operationsApplication.DashboardSessionIdentity, error) {
	return operationsApplication.DashboardSessionIdentity{}, operationsDomain.ErrDashboardSessionNotFound
}
func (EmptyDashboardAuthUseCase) ValidCSRF(operationsApplication.DashboardSessionIdentity, string) bool {
	return false
}
func (EmptyDashboardAuthUseCase) Logout(context.Context, operationsApplication.DashboardSessionIdentity) error {
	return nil
}
func (EmptyDashboardAuthUseCase) ChangePassword(context.Context, int64, string, string) (operationsApplication.DashboardCredentials, error) {
	return operationsApplication.DashboardCredentials{}, errors.New("dashboard auth unavailable")
}

var _ DashboardAuthUseCase = EmptyDashboardAuthUseCase{}
