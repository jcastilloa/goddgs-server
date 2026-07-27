package research

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jcastilloa/goddgs-server/research/domain"
	extractAIDomain "github.com/jcastilloa/goddgs-server/shared/extractai/domain"
)

const PostHandlerLabel = "handler.research.post"

type UseCase interface {
	Research(context.Context, domain.Request) (domain.Result, error)
}

type Post struct {
	useCase UseCase
}

func NewPost(useCase UseCase) Post {
	return Post{useCase: useCase}
}

func (h Post) Handle(ginContext *gin.Context) {
	request, err := decodeRequest(ginContext)
	if err != nil {
		ginContext.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if h.useCase == nil {
		ginContext.JSON(http.StatusServiceUnavailable, gin.H{"error": domain.ErrUnavailable.Error()})
		return
	}
	result, err := h.useCase.Research(ginContext.Request.Context(), request)
	if err != nil {
		writeError(ginContext, err)
		return
	}
	ginContext.JSON(http.StatusOK, result)
}

func decodeRequest(ginContext *gin.Context) (domain.Request, error) {
	var request domain.Request
	decoder := json.NewDecoder(ginContext.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return domain.Request{}, errors.New("invalid research request: request body must be valid JSON")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return domain.Request{}, errors.New("invalid research request: request body must contain one JSON object")
	}
	if _, err := request.Normalize(); err != nil {
		return domain.Request{}, err
	}
	return request, nil
}

func writeError(ginContext *gin.Context, err error) {
	ginContext.Error(err)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		ginContext.JSON(http.StatusGatewayTimeout, gin.H{"error": "research timed out"})
	case errors.Is(err, context.Canceled):
		ginContext.JSON(499, gin.H{"error": "request canceled"})
	case errors.Is(err, domain.ErrInvalidRequest):
		ginContext.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, domain.ErrUnavailable):
		ginContext.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	case errors.Is(err, extractAIDomain.ErrRateLimited):
		ginContext.JSON(http.StatusTooManyRequests, gin.H{"error": "research rate limited"})
	default:
		ginContext.JSON(http.StatusBadGateway, gin.H{"error": "research failed"})
	}
}
