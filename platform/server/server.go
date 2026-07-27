package server

import (
	"context"
	"strings"
	"time"

	"github.com/jcastilloa/goddgs-server/platform/routes"

	"github.com/gin-gonic/gin"
	"github.com/sarulabs/di"
)

type Server struct {
	engine    *gin.Engine
	container di.Container
}

func New(container di.Container, apiPrefix, authToken string, requestTimeout time.Duration) *Server {
	engine := gin.New()
	engine.Use(gin.Recovery(), gin.Logger())
	engine.Use(authentication(authToken))
	engine.Use(requestTimeoutMiddleware(requestTimeout))

	s := &Server{engine: engine, container: container}
	s.registerRoutes(apiPrefix)
	s.registerDocumentation(normalizePrefix(apiPrefix))
	return s
}

func requestTimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	if timeout <= 0 {
		return func(*gin.Context) {}
	}

	return func(ginContext *gin.Context) {
		requestContext, cancel := context.WithTimeout(ginContext.Request.Context(), timeout)
		defer cancel()
		ginContext.Request = ginContext.Request.WithContext(requestContext)
		ginContext.Next()
	}
}

func (s *Server) Run(address string) error {
	return s.engine.Run(address)
}

func (s *Server) registerRoutes(apiPrefix string) {
	v1 := s.engine.Group(normalizePrefix(apiPrefix))

	routes.AddHelloRoutes(v1, s.container)
	routes.AddSystemRoutes(v1, s.container)
	routes.AddSearchRoutes(v1, s.container)
}

func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "/v1"
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	prefix = strings.TrimRight(prefix, "/")
	if prefix == "" {
		return "/v1"
	}
	return prefix
}
