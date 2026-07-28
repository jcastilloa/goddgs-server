package routes

import (
	operationsHandler "github.com/jcastilloa/goddgs-server/platform/handlers/operations"

	"github.com/gin-gonic/gin"
	"github.com/sarulabs/di"
)

func AddOperationsRoutes(group *gin.RouterGroup, container di.Container) {
	group.GET("", buildEndpoint(container, operationsHandler.DashboardHandlerLabel))
	group.GET("/api/summary", buildEndpoint(container, operationsHandler.SummaryHandlerLabel))
	group.GET("/api/timeseries", buildEndpoint(container, operationsHandler.TimeSeriesHandlerLabel))
	group.GET("/api/operations", buildEndpoint(container, operationsHandler.ListHandlerLabel))
	group.GET("/api/operations/:id", buildEndpoint(container, operationsHandler.DetailHandlerLabel))
	group.GET("/api/proxies", buildEndpoint(container, operationsHandler.ProxiesHandlerLabel))
}
