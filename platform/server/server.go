package server

import (
	"strings"

	"github.com/jcastilloa/goddgs-server/platform/routes"

	"github.com/gin-gonic/gin"
	"github.com/sarulabs/di"
)

type Server struct {
	engine    *gin.Engine
	container di.Container
}

func New(container di.Container, apiPrefix string) *Server {
	engine := gin.New()
	engine.Use(gin.Recovery(), gin.Logger())

	s := &Server{engine: engine, container: container}
	s.registerRoutes(apiPrefix)
	return s
}

func (s *Server) Run(address string) error {
	return s.engine.Run(address)
}

func (s *Server) registerRoutes(apiPrefix string) {
	v1 := s.engine.Group(normalizePrefix(apiPrefix))

	routes.AddHelloRoutes(v1, s.container)
	routes.AddSystemRoutes(v1, s.container)
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
