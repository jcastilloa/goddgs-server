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

func selectiveAuthentication(token, apiPrefix string) gin.HandlerFunc {
	authenticate := authentication(token)
	return func(context *gin.Context) {
		if !requiresAuthentication(context.Request.URL.Path, apiPrefix) {
			context.Next()
			return
		}
		authenticate(context)
	}
}

func requiresAuthentication(path, apiPrefix string) bool {
	normalizedPrefix := normalizePrefix(apiPrefix)
	return path == "/openapi.json" || strings.HasPrefix(path, "/docs") || path == normalizedPrefix || strings.HasPrefix(path, normalizedPrefix+"/")
}
