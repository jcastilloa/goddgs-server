package routes

import (
	helloHandler "github.com/jcastilloa/goddgs-server/platform/handlers/hello"

	"github.com/gin-gonic/gin"
	"github.com/sarulabs/di"
)

func AddHelloRoutes(group *gin.RouterGroup, container di.Container) {
	group.GET("/hello", buildEndpoint(container, helloHandler.GetHelloHandlerLabel))
}
