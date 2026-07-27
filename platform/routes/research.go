package routes

import (
	researchHandler "github.com/jcastilloa/goddgs-server/platform/handlers/research"

	"github.com/gin-gonic/gin"
	"github.com/sarulabs/di"
)

func AddResearchRoutes(group *gin.RouterGroup, container di.Container) {
	group.POST("/research", buildEndpoint(container, researchHandler.PostHandlerLabel))
}
