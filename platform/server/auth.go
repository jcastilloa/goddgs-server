package server

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func authentication(token string) gin.HandlerFunc {
	token = strings.TrimSpace(token)
	if token == "" {
		return func(*gin.Context) {}
	}

	return func(context *gin.Context) {
		provided, ok := strings.CutPrefix(context.GetHeader("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		context.Next()
	}
}
