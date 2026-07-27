package routes

import (
	systemHandler "github.com/jcastilloa/goddgs-server/platform/handlers/system"

	"github.com/gin-gonic/gin"
	"github.com/sarulabs/di"
)

func AddSystemRoutes(group *gin.RouterGroup, container di.Container) {
	group.GET("/version", buildEndpoint(container, systemHandler.GetVersionHandlerLabel))
}
