package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	operationsApplication "github.com/jcastilloa/goddgs-server/operations/application"
	operations "github.com/jcastilloa/goddgs-server/operations/domain"
	containerdi "github.com/jcastilloa/goddgs-server/platform/di"
	operationsHandler "github.com/jcastilloa/goddgs-server/platform/handlers/operations"
	searchApplication "github.com/jcastilloa/goddgs-server/search/application"

	"github.com/gin-gonic/gin"
)

func TestDashboardAuthenticationFlowProtectsHTMLAndAPI(t *testing.T) {
	auth := newDashboardAuthForServerTest()
	container, err := containerdi.New("test", searchApplication.NewService(&serverGateway{}), nil, nil, operationsHandler.EmptyUseCase{}, containerdi.WithDashboardAuth(auth, true)).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer (*container).Delete()
	server := New(*container, "/v1", "test", "bearer", time.Second, time.Second, WithDashboardAuthentication(auth, true))

	first := httptest.NewRecorder()
	server.engine.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/operations", nil))
	if first.Code != http.StatusSeeOther || first.Header().Get("Location") != "/operations/setup" {
		t.Fatalf("GET /operations = %d %q, want 303 setup", first.Code, first.Header().Get("Location"))
	}

	setup := httptest.NewRecorder()
	request := jsonRequest(t, http.MethodPost, "/operations/api/auth/setup", `{"username":"operator","password":"a secure password"}`)
	server.engine.ServeHTTP(setup, request)
	if setup.Code != http.StatusCreated {
		t.Fatalf("setup status = %d, body = %s", setup.Code, setup.Body.String())
	}
	cookies := cookieValues(setup.Result().Cookies())
	if cookies["operations_session"] == "" || cookies["operations_csrf"] == "" {
		t.Fatalf("setup cookies = %#v", cookies)
	}
	for _, cookie := range setup.Result().Cookies() {
		if cookie.Name == "operations_session" && (!cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/operations") {
			t.Errorf("session cookie = %#v", cookie)
		}
	}

	setupPage := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/operations/setup", nil)
	addCookies(request, cookies)
	server.engine.ServeHTTP(setupPage, request)
	if setupPage.Code != http.StatusSeeOther || setupPage.Header().Get("Location") != "/operations" {
		t.Errorf("authenticated setup page = %d %q, want dashboard redirect", setupPage.Code, setupPage.Header().Get("Location"))
	}
	loginPage := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/operations/login", nil)
	addCookies(request, cookies)
	server.engine.ServeHTTP(loginPage, request)
	if loginPage.Code != http.StatusSeeOther || loginPage.Header().Get("Location") != "/operations" {
		t.Errorf("authenticated login page = %d %q, want dashboard redirect", loginPage.Code, loginPage.Header().Get("Location"))
	}

	dashboard := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/operations", nil)
	addCookies(request, cookies)
	server.engine.ServeHTTP(dashboard, request)
	if dashboard.Code != http.StatusOK || !strings.Contains(dashboard.Body.String(), "account-username") {
		t.Errorf("dashboard = %d %s", dashboard.Code, dashboard.Body.String())
	}

	csrfFailure := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/operations/api/auth/logout", nil)
	addCookies(request, cookies)
	server.engine.ServeHTTP(csrfFailure, request)
	if csrfFailure.Code != http.StatusForbidden {
		t.Errorf("logout without CSRF = %d, want 403", csrfFailure.Code)
	}

	logout := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/operations/api/auth/logout", nil)
	request.Header.Set("X-Operations-CSRF", cookies["operations_csrf"])
	addCookies(request, cookies)
	server.engine.ServeHTTP(logout, request)
	if logout.Code != http.StatusNoContent {
		t.Errorf("logout = %d, body = %s", logout.Code, logout.Body.String())
	}

	api := httptest.NewRecorder()
	server.engine.ServeHTTP(api, httptest.NewRequest(http.MethodGet, "/operations/api/summary", nil))
	if api.Code != http.StatusUnauthorized || !strings.Contains(api.Body.String(), "dashboard authentication required") {
		t.Errorf("unauthenticated API = %d %s", api.Code, api.Body.String())
	}

	bearer := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/operations", nil)
	request.Header.Set("Authorization", "Bearer bearer")
	server.engine.ServeHTTP(bearer, request)
	if bearer.Code != http.StatusSeeOther || bearer.Header().Get("Location") != "/operations/login" {
		t.Errorf("bearer dashboard = %d %q, want login redirect", bearer.Code, bearer.Header().Get("Location"))
	}
}

func TestDashboardAuthenticationClearsExpiredSessionCookies(t *testing.T) {
	currentTime := time.Now()
	auth := newDashboardAuthForServerTestWithClock(func() time.Time { return currentTime })
	container, err := containerdi.New("test", searchApplication.NewService(&serverGateway{}), nil, nil, operationsHandler.EmptyUseCase{}, containerdi.WithDashboardAuth(auth, false)).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer (*container).Delete()
	server := New(*container, "/v1", "test", "", time.Second, time.Second, WithDashboardAuthentication(auth, false))

	setup := httptest.NewRecorder()
	server.engine.ServeHTTP(setup, jsonRequest(t, http.MethodPost, "/operations/api/auth/setup", `{"username":"operator","password":"a secure password"}`))
	cookies := cookieValues(setup.Result().Cookies())
	currentTime = currentTime.Add(2 * time.Hour)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/operations/api/auth/session", nil)
	addCookies(request, cookies)
	server.engine.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expired session = %d %s, want 401", response.Code, response.Body.String())
	}
	for _, cookie := range response.Result().Cookies() {
		if (cookie.Name == "operations_session" || cookie.Name == "operations_csrf") && cookie.MaxAge >= 0 {
			t.Errorf("expired-session cookie = %#v, want cleared cookie", cookie)
		}
	}
}

func TestDashboardAuthenticationChangesPasswordAndInvalidatesPreviousSession(t *testing.T) {
	auth := newDashboardAuthForServerTest()
	container, err := containerdi.New("test", searchApplication.NewService(&serverGateway{}), nil, nil, operationsHandler.EmptyUseCase{}, containerdi.WithDashboardAuth(auth, false)).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer (*container).Delete()
	server := New(*container, "/v1", "test", "", time.Second, time.Second, WithDashboardAuthentication(auth, false))

	setup := httptest.NewRecorder()
	server.engine.ServeHTTP(setup, jsonRequest(t, http.MethodPost, "/operations/api/auth/setup", `{"username":"operator","password":"a secure password"}`))
	cookies := cookieValues(setup.Result().Cookies())

	unchanged := httptest.NewRecorder()
	request := jsonRequest(t, http.MethodPost, "/operations/api/auth/password", `{"current_password":"a secure password","new_password":"a secure password"}`)
	request.Header.Set("X-Operations-CSRF", cookies["operations_csrf"])
	addCookies(request, cookies)
	server.engine.ServeHTTP(unchanged, request)
	if unchanged.Code != http.StatusBadRequest {
		t.Fatalf("unchanged password = %d %s, want 400", unchanged.Code, unchanged.Body.String())
	}

	incorrectCurrent := httptest.NewRecorder()
	request = jsonRequest(t, http.MethodPost, "/operations/api/auth/password", `{"current_password":"wrong password","new_password":"a different secure password"}`)
	request.Header.Set("X-Operations-CSRF", cookies["operations_csrf"])
	addCookies(request, cookies)
	server.engine.ServeHTTP(incorrectCurrent, request)
	if incorrectCurrent.Code != http.StatusUnauthorized {
		t.Fatalf("incorrect current password = %d %s, want 401", incorrectCurrent.Code, incorrectCurrent.Body.String())
	}

	currentSession := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/operations/api/auth/session", nil)
	addCookies(request, cookies)
	server.engine.ServeHTTP(currentSession, request)
	if currentSession.Code != http.StatusOK || !strings.Contains(currentSession.Body.String(), `"username":"operator"`) {
		t.Fatalf("session after rejected password change = %d %s", currentSession.Code, currentSession.Body.String())
	}

	change := httptest.NewRecorder()
	request = jsonRequest(t, http.MethodPost, "/operations/api/auth/password", `{"current_password":"a secure password","new_password":"a different secure password"}`)
	request.Header.Set("X-Operations-CSRF", cookies["operations_csrf"])
	addCookies(request, cookies)
	server.engine.ServeHTTP(change, request)
	if change.Code != http.StatusNoContent {
		t.Fatalf("password change = %d %s", change.Code, change.Body.String())
	}

	oldSession := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/operations/api/auth/session", nil)
	addCookies(request, cookies)
	server.engine.ServeHTTP(oldSession, request)
	if oldSession.Code != http.StatusUnauthorized {
		t.Errorf("old session = %d, want 401", oldSession.Code)
	}
}

func TestDashboardAuthenticationRejectsOversizedJSON(t *testing.T) {
	auth := newDashboardAuthForServerTest()
	container, err := containerdi.New("test", searchApplication.NewService(&serverGateway{}), nil, nil, operationsHandler.EmptyUseCase{}, containerdi.WithDashboardAuth(auth, false)).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer (*container).Delete()
	server := New(*container, "/v1", "test", "", time.Second, time.Second, WithDashboardAuthentication(auth, false))

	body := `{"username":"operator","password":"` + strings.Repeat("a", 4<<10) + `"}`
	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, jsonRequest(t, http.MethodPost, "/operations/api/auth/setup", body))
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("oversized setup = %d %s, want 400", recorder.Code, recorder.Body.String())
	}
}

func TestDashboardAuthenticationReturnsUnavailableWhenSessionValidationFails(t *testing.T) {
	engine := gin.New()
	engine.GET("/operations/api/summary", dashboardAuthentication(unavailableDashboardAuthenticator{}, false), func(ctx *gin.Context) {
		ctx.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodGet, "/operations/api/summary", nil)
	request.AddCookie(&http.Cookie{Name: "operations_session", Value: "session"})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "operations dashboard authentication is unavailable") {
		t.Errorf("unavailable dashboard authentication = %d %s", recorder.Code, recorder.Body.String())
	}
}

func newDashboardAuthForServerTest() *operationsApplication.DashboardAuthService {
	return newDashboardAuthForServerTestWithClock(time.Now)
}

func newDashboardAuthForServerTestWithClock(now func() time.Time) *operationsApplication.DashboardAuthService {
	return operationsApplication.NewDashboardAuthService(&serverDashboardRepository{}, operationsApplication.DashboardAuthConfig{
		SessionTTL: time.Hour,
		Now:        now,
		Random:     func(size int) ([]byte, error) { return randomServerBytes(size), nil },
	})
}

func randomServerBytes(size int) []byte {
	result := make([]byte, size)
	for index := range result {
		result[index] = byte(time.Now().UnixNano() >> (index % 8))
	}
	return result
}

func jsonRequest(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func cookieValues(cookies []*http.Cookie) map[string]string {
	result := make(map[string]string, len(cookies))
	for _, cookie := range cookies {
		result[cookie.Name] = cookie.Value
	}
	return result
}

func addCookies(request *http.Request, values map[string]string) {
	for name, value := range values {
		if value != "" {
			request.AddCookie(&http.Cookie{Name: name, Value: value})
		}
	}
}

type serverDashboardRepository struct {
	user     operations.DashboardUser
	sessions map[string]operations.DashboardSession
}

func (r *serverDashboardRepository) HasDashboardUser(context.Context) (bool, error) {
	return r.user.ID != 0, nil
}
func (r *serverDashboardRepository) CreateDashboardUser(_ context.Context, user operations.DashboardUser) (operations.DashboardUser, error) {
	if r.user.ID != 0 {
		return operations.DashboardUser{}, operations.ErrDashboardSetupCompleted
	}
	user.ID = 1
	r.user = user
	return user, nil
}
func (r *serverDashboardRepository) FindDashboardUserByID(_ context.Context, id int64) (operations.DashboardUser, bool, error) {
	return r.user, r.user.ID == id, nil
}
func (r *serverDashboardRepository) FindDashboardUserByUsername(_ context.Context, username string) (operations.DashboardUser, bool, error) {
	return r.user, r.user.ID != 0 && r.user.Username == username, nil
}
func (r *serverDashboardRepository) CreateDashboardSession(_ context.Context, session operations.DashboardSession) error {
	if r.sessions == nil {
		r.sessions = map[string]operations.DashboardSession{}
	}
	r.sessions[session.TokenHash] = session
	return nil
}
func (r *serverDashboardRepository) FindDashboardSession(_ context.Context, hash string) (operations.DashboardSession, bool, error) {
	session, found := r.sessions[hash]
	return session, found, nil
}
func (r *serverDashboardRepository) DeleteDashboardSession(_ context.Context, hash string) error {
	delete(r.sessions, hash)
	return nil
}
func (r *serverDashboardRepository) ReplaceDashboardPasswordAndSession(_ context.Context, id int64, hash string, session operations.DashboardSession) error {
	if r.user.ID != id {
		return operations.ErrDashboardSessionNotFound
	}
	r.user.PasswordHash = hash
	r.sessions = map[string]operations.DashboardSession{session.TokenHash: session}
	return nil
}

var _ operations.DashboardAuthRepository = (*serverDashboardRepository)(nil)

type unavailableDashboardAuthenticator struct{}

func (unavailableDashboardAuthenticator) NeedsSetup(context.Context) (bool, error) {
	return false, errors.New("store unavailable")
}

func (unavailableDashboardAuthenticator) Authenticate(context.Context, string) (operationsApplication.DashboardSessionIdentity, error) {
	return operationsApplication.DashboardSessionIdentity{}, errors.New("store unavailable")
}
