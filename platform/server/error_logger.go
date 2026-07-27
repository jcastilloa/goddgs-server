package server

import (
	"log"

	"github.com/gin-gonic/gin"
)

func errorLogger(logger *log.Logger) gin.HandlerFunc {
	return func(context *gin.Context) {
		context.Next()
		for _, entry := range context.Errors {
			logger.Printf("request failed method=%s path=%s status=%d error=%v", context.Request.Method, context.Request.URL.Path, context.Writer.Status(), entry.Err)
		}
	}
}
