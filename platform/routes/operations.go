package routes

import (
	operationsHandler "github.com/jcastilloa/goddgs-server/platform/handlers/operations"

	"github.com/gin-gonic/gin"
	"github.com/sarulabs/di"
)

func AddOperationsRoutes(public, protected *gin.RouterGroup, container di.Container) {
	public.GET("/setup", buildEndpoint(container, operationsHandler.SetupPageHandlerLabel))
	public.GET("/login", buildEndpoint(container, operationsHandler.LoginPageHandlerLabel))
	public.POST("/api/auth/setup", buildEndpoint(container, operationsHandler.SetupHandlerLabel))
	public.POST("/api/auth/login", buildEndpoint(container, operationsHandler.LoginHandlerLabel))

	protected.GET("", buildEndpoint(container, operationsHandler.DashboardHandlerLabel))
	protected.GET("/api/summary", buildEndpoint(container, operationsHandler.SummaryHandlerLabel))
	protected.GET("/api/timeseries", buildEndpoint(container, operationsHandler.TimeSeriesHandlerLabel))
	protected.GET("/api/operations", buildEndpoint(container, operationsHandler.ListHandlerLabel))
	protected.GET("/api/operations/:id", buildEndpoint(container, operationsHandler.DetailHandlerLabel))
	protected.GET("/api/proxies", buildEndpoint(container, operationsHandler.ProxiesHandlerLabel))
	protected.GET("/api/auth/session", buildEndpoint(container, operationsHandler.SessionHandlerLabel))
	protected.POST("/api/auth/logout", buildEndpoint(container, operationsHandler.LogoutHandlerLabel))
	protected.POST("/api/auth/password", buildEndpoint(container, operationsHandler.PasswordHandlerLabel))
}
