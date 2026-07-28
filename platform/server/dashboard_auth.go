package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	operationsApplication "github.com/jcastilloa/goddgs-server/operations/application"
	operations "github.com/jcastilloa/goddgs-server/operations/domain"
)

const (
	dashboardSessionCookie = "operations_session"
	dashboardCSRFCookie    = "operations_csrf"
)

type dashboardAuthenticator interface {
	NeedsSetup(context.Context) (bool, error)
	Authenticate(context.Context, string) (operationsApplication.DashboardSessionIdentity, error)
}

func dashboardAuthentication(authenticator dashboardAuthenticator, cookieSecure bool) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		token, err := ctx.Cookie(dashboardSessionCookie)
		if err == nil {
			if _, authErr := authenticator.Authenticate(ctx.Request.Context(), token); authErr == nil {
				ctx.Next()
				return
			} else if !isInvalidDashboardSession(authErr) {
				abortDashboardAuthenticationUnavailable(ctx)
				return
			}
		}
		clearDashboardCookies(ctx, cookieSecure)
		if strings.HasPrefix(ctx.Request.URL.Path, "/operations/api/") {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "dashboard authentication required"})
			return
		}
		needsSetup, setupErr := authenticator.NeedsSetup(ctx.Request.Context())
		if setupErr != nil {
			abortDashboardAuthenticationUnavailable(ctx)
			return
		}
		if needsSetup {
			ctx.Redirect(http.StatusSeeOther, "/operations/setup")
		} else {
			ctx.Redirect(http.StatusSeeOther, "/operations/login")
		}
		ctx.Abort()
	}
}

func isInvalidDashboardSession(err error) bool {
	return errors.Is(err, operations.ErrDashboardSessionNotFound) || errors.Is(err, operations.ErrDashboardSessionExpired)
}

func abortDashboardAuthenticationUnavailable(ctx *gin.Context) {
	ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "operations dashboard authentication is unavailable"})
}

func clearDashboardCookies(ctx *gin.Context, cookieSecure bool) {
	ctx.SetSameSite(http.SameSiteStrictMode)
	ctx.SetCookie(dashboardSessionCookie, "", -1, "/operations", "", cookieSecure, true)
	ctx.SetCookie(dashboardCSRFCookie, "", -1, "/operations", "", cookieSecure, false)
}
