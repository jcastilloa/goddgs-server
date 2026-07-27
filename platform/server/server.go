package server

import (
	"context"
	"errors"
	"log"
	"net/http"
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

func New(container di.Container, apiPrefix, version, authToken string, requestTimeout, researchTimeout time.Duration) *Server {
	engine := gin.New()
	engine.Use(gin.Recovery(), gin.Logger(), errorLogger(log.Default()))
	engine.Use(authentication(authToken))
	engine.Use(requestTimeoutMiddleware(requestTimeout, researchTimeout, normalizePrefix(apiPrefix)+"/research", normalizePrefix(apiPrefix)+"/extract"))

	s := &Server{engine: engine, container: container}
	s.registerRoutes(apiPrefix)
	s.registerDocumentation(normalizePrefix(apiPrefix), version, strings.TrimSpace(authToken) != "")
	return s
}

func requestTimeoutMiddleware(timeout, researchTimeout time.Duration, researchPath, extractPath string) gin.HandlerFunc {
	if timeout <= 0 && researchTimeout <= 0 {
		return func(*gin.Context) {}
	}

	return func(ginContext *gin.Context) {
		requestTimeout := selectedRequestTimeout(ginContext.Request, timeout, researchTimeout, researchPath, extractPath)
		if requestTimeout <= 0 {
			ginContext.Next()
			return
		}
		requestContext, cancel := context.WithTimeout(ginContext.Request.Context(), requestTimeout)
		defer cancel()
		ginContext.Request = ginContext.Request.WithContext(requestContext)
		ginContext.Next()
	}
}

func selectedRequestTimeout(request *http.Request, timeout, researchTimeout time.Duration, researchPath, extractPath string) time.Duration {
	if request.Method == http.MethodPost && request.URL.Path == researchPath && researchTimeout > 0 {
		return researchTimeout
	}
	if request.Method == http.MethodGet && request.URL.Path == extractPath && strings.EqualFold(strings.TrimSpace(request.URL.Query().Get("mode")), "ai") {
		return 0
	}
	return timeout
}

func (s *Server) Run(ctx context.Context, address string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	server := &http.Server{Addr: address, Handler: s.engine}
	shutdown := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = server.Shutdown(context.Background())
		case <-shutdown:
		}
	}()
	err := server.ListenAndServe()
	close(shutdown)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) registerRoutes(apiPrefix string) {
	v1 := s.engine.Group(normalizePrefix(apiPrefix))

	routes.AddSystemRoutes(v1, s.container)
	routes.AddSearchRoutes(v1, s.container)
	routes.AddResearchRoutes(v1, s.container)
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
