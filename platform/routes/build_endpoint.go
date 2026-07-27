package routes

import (
	"net/http"

	"github.com/jcastilloa/goddgs-server/platform/handlers"

	"github.com/gin-gonic/gin"
	"github.com/sarulabs/di"
)

func buildEndpoint(container di.Container, handlerLabel string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		resolved := container.Get(handlerLabel)
		handler, ok := resolved.(handlers.Handler)
		if !ok {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "invalid handler wiring: " + handlerLabel})
			return
		}
		handler.Handle(ctx)
	}
}
