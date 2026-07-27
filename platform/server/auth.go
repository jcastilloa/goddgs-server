package server

import (
	"crypto/sha256"
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
		providedDigest := sha256.Sum256([]byte(provided))
		tokenDigest := sha256.Sum256([]byte(token))
		if !ok || subtle.ConstantTimeCompare(providedDigest[:], tokenDigest[:]) != 1 {
			context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		context.Next()
	}
}
