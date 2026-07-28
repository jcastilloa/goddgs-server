package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	operationsApplication "github.com/jcastilloa/goddgs-server/operations/application"
	operations "github.com/jcastilloa/goddgs-server/operations/domain"
	operationsHandler "github.com/jcastilloa/goddgs-server/platform/handlers/operations"
	"github.com/jcastilloa/goddgs-server/platform/routes"

	"github.com/gin-gonic/gin"
	"github.com/sarulabs/di"
)

type Server struct {
	engine        *gin.Engine
	container     di.Container
	dashboardAuth dashboardAuthenticator
	cookieSecure  bool
}

type DashboardAuthOption func(*Server)

func WithDashboardAuthentication(authenticator dashboardAuthenticator, cookieSecure bool) DashboardAuthOption {
	return func(server *Server) {
		server.dashboardAuth = authenticator
		server.cookieSecure = cookieSecure
	}
}

func New(container di.Container, apiPrefix, version, authToken string, requestTimeout, researchTimeout time.Duration, options ...DashboardAuthOption) *Server {
	engine := gin.New()
	engine.Use(gin.Recovery(), gin.Logger(), errorLogger(log.Default()))
	engine.Use(selectiveAuthentication(authToken, apiPrefix))
	engine.Use(requestTimeoutMiddleware(requestTimeout, researchTimeout, normalizePrefix(apiPrefix)+"/research", normalizePrefix(apiPrefix)+"/extract"))

	s := &Server{engine: engine, container: container, dashboardAuth: operationsHandler.EmptyDashboardAuthUseCase{}}
	for _, option := range options {
		option(s)
	}
	s.registerRoutes(engine.Group(""), apiPrefix)
	s.registerDocumentation(engine.Group(""), normalizePrefix(apiPrefix), version, strings.TrimSpace(authToken) != "")
	s.registerOperationsRoutes()
	return s
}

func NewWithRecorder(container di.Container, apiPrefix, version, authToken string, requestTimeout, researchTimeout time.Duration, recorder operationsApplication.EventRecorder, options ...DashboardAuthOption) *Server {
	engine := gin.New()
	engine.Use(gin.Recovery(), gin.Logger(), errorLogger(log.Default()))
	engine.Use(selectiveAuthentication(authToken, apiPrefix))
	engine.Use(requestTimeoutMiddleware(requestTimeout, researchTimeout, normalizePrefix(apiPrefix)+"/research", normalizePrefix(apiPrefix)+"/extract"))

	s := &Server{engine: engine, container: container, dashboardAuth: operationsHandler.EmptyDashboardAuthUseCase{}}
	for _, option := range options {
		option(s)
	}
	s.registerRoutesWithRecorder(engine.Group(""), apiPrefix, recorder)
	s.registerDocumentation(engine.Group(""), normalizePrefix(apiPrefix), version, strings.TrimSpace(authToken) != "")
	s.registerOperationsRoutes()
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

func (s *Server) registerRoutes(protected *gin.RouterGroup, apiPrefix string) {
	v1 := protected.Group(normalizePrefix(apiPrefix))

	routes.AddSystemRoutes(v1, s.container)
	routes.AddSearchRoutes(v1, s.container)
	routes.AddResearchRoutes(v1, s.container)
}

func (s *Server) registerRoutesWithRecorder(protected *gin.RouterGroup, apiPrefix string, recorder operationsApplication.EventRecorder) {
	v1 := protected.Group(normalizePrefix(apiPrefix))
	instrumented := protected.Group(normalizePrefix(apiPrefix))
	instrumented.Use(operationRecorderMiddleware(recorder))

	routes.AddSystemRoutes(v1, s.container)
	routes.AddSearchRoutes(instrumented, s.container)
	routes.AddResearchRoutes(instrumented, s.container)
}

func (s *Server) registerOperationsRoutes() {
	public := s.engine.Group("/operations")
	protected := s.engine.Group("/operations")
	protected.Use(dashboardAuthentication(s.dashboardAuth, s.cookieSecure))
	routes.AddOperationsRoutes(public, protected, s.container)
}

func NewWithDashboardAuth(container di.Container, apiPrefix, version, authToken string, requestTimeout, researchTimeout time.Duration, dashboardAuth dashboardAuthenticator, cookieSecure bool) *Server {
	return New(container, apiPrefix, version, authToken, requestTimeout, researchTimeout, WithDashboardAuthentication(dashboardAuth, cookieSecure))
}

func NewWithRecorderAndDashboardAuth(container di.Container, apiPrefix, version, authToken string, requestTimeout, researchTimeout time.Duration, recorder operationsApplication.EventRecorder, dashboardAuth dashboardAuthenticator, cookieSecure bool) *Server {
	return NewWithRecorder(container, apiPrefix, version, authToken, requestTimeout, researchTimeout, recorder, WithDashboardAuthentication(dashboardAuth, cookieSecure))
}

func operationRecorderMiddleware(recorder operationsApplication.EventRecorder) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		start, ok := operationStart(ginContext)
		if !ok {
			ginContext.Next()
			return
		}
		requestContext, err := recorder.StartOperation(ginContext.Request.Context(), start)
		if err == nil {
			ginContext.Request = ginContext.Request.WithContext(requestContext)
		}
		ginContext.Next()

		finishErr := ginContext.Request.Context().Err()
		if finishErr == nil && len(ginContext.Errors) > 0 {
			finishErr = ginContext.Errors.Last().Err
		}
		_ = recorder.FinishOperation(ginContext.Request.Context(), operations.OperationFinish{
			HTTPStatus: ginContext.Writer.Status(),
			Err:        finishErr,
		})
	}
}

func operationStart(ginContext *gin.Context) (operations.OperationStart, bool) {
	path := ginContext.Request.URL.Path
	switch {
	case ginContext.Request.Method == http.MethodGet && isSearchPath(path):
		category := strings.TrimPrefix(path, normalizePrefix(""))
		return operations.OperationStart{
			Type:   operations.OperationSearch,
			Method: ginContext.Request.Method,
			Path:   path,
			Metadata: map[string]string{
				"query":    searchQuery(ginContext),
				"backend":  ginContext.Query("backend"),
				"category": strings.TrimPrefix(category, "/"),
			},
		}, true
	case ginContext.Request.Method == http.MethodGet && strings.HasSuffix(path, "/extract"):
		return operations.OperationStart{
			Type:   operations.OperationExtract,
			Method: ginContext.Request.Method,
			Path:   path,
			Metadata: map[string]string{
				"url":  ginContext.Query("url"),
				"mode": ginContext.Query("mode"),
			},
		}, true
	case ginContext.Request.Method == http.MethodPost && strings.HasSuffix(path, "/research"):
		return operations.OperationStart{
			Type:   operations.OperationResearch,
			Method: ginContext.Request.Method,
			Path:   path,
		}, true
	default:
		return operations.OperationStart{}, false
	}
}

func isSearchPath(path string) bool {
	for _, suffix := range []string{"/text", "/images", "/news", "/videos", "/books"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

func searchQuery(ginContext *gin.Context) string {
	if value := ginContext.Query("q"); strings.TrimSpace(value) != "" {
		return value
	}
	return ginContext.Query("query")
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
