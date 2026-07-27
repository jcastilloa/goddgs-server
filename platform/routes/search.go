package routes

import (
	searchHandler "github.com/jcastilloa/goddgs-server/platform/handlers/search"

	"github.com/gin-gonic/gin"
	"github.com/sarulabs/di"
)

func AddSearchRoutes(group *gin.RouterGroup, container di.Container) {
	group.GET("/text", buildEndpoint(container, searchHandler.GetTextHandlerLabel))
	group.GET("/images", buildEndpoint(container, searchHandler.GetImagesHandlerLabel))
	group.GET("/news", buildEndpoint(container, searchHandler.GetNewsHandlerLabel))
	group.GET("/videos", buildEndpoint(container, searchHandler.GetVideosHandlerLabel))
	group.GET("/books", buildEndpoint(container, searchHandler.GetBooksHandlerLabel))
	group.GET("/extract", buildEndpoint(container, searchHandler.GetExtractHandlerLabel))
}
