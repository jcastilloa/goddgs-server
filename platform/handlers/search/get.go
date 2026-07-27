package search

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jcastilloa/goddgs-server/search/domain"
)

type SearchUseCase interface {
	Search(context.Context, domain.SearchRequest) ([]domain.RawResult, error)
}

const (
	GetTextHandlerLabel   = "handler.search.text.get"
	GetImagesHandlerLabel = "handler.search.images.get"
	GetNewsHandlerLabel   = "handler.search.news.get"
	GetVideosHandlerLabel = "handler.search.videos.get"
	GetBooksHandlerLabel  = "handler.search.books.get"
)

type Get struct {
	category domain.Category
	useCase  SearchUseCase
}

func NewGet(category domain.Category, useCase SearchUseCase) Get {
	return Get{category: category, useCase: useCase}
}

func (h Get) Handle(context *gin.Context) {
	request, err := searchRequest(context, h.category)
	if err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := request.Validate(); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results, err := h.useCase.Search(context.Request.Context(), request)
	if err != nil {
		writeSearchError(context, err)
		return
	}
	context.JSON(http.StatusOK, results)
}

func searchRequest(context *gin.Context, category domain.Category) (domain.SearchRequest, error) {
	maxResults, err := optionalPositiveInt(context.Query("max_results"))
	if err != nil {
		return domain.SearchRequest{}, err
	}
	page, err := optionalPositiveInt(context.Query("page"))
	if err != nil {
		return domain.SearchRequest{}, err
	}
	query := context.Query("q")
	if query == "" {
		query = context.Query("query")
	}
	return domain.SearchRequest{
		Category:   category,
		Query:      query,
		Region:     context.Query("region"),
		SafeSearch: context.Query("safesearch"),
		TimeLimit:  context.Query("timelimit"),
		MaxResults: maxResults,
		Page:       page,
		Backend:    context.Query("backend"),
		Images: domain.ImageOptions{
			Size: context.Query("size"), Color: context.Query("color"), Type: context.Query("type_image"), Layout: context.Query("layout"), License: context.Query("license_image"),
		},
		Videos: domain.VideoOptions{
			Resolution: context.Query("resolution"), Duration: context.Query("duration"), License: context.Query("license_videos"),
		},
	}, nil
}

func optionalPositiveInt(value string) (*int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return nil, errors.New("value must be a positive integer")
	}
	return &parsed, nil
}

func writeSearchError(ginContext *gin.Context, err error) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		ginContext.JSON(http.StatusGatewayTimeout, gin.H{"error": "request timed out"})
	case errors.Is(err, context.Canceled):
		ginContext.JSON(499, gin.H{"error": "request canceled"})
	case errors.Is(err, domain.ErrSearchTimeout):
		ginContext.JSON(http.StatusGatewayTimeout, gin.H{"error": "search timed out"})
	case errors.Is(err, domain.ErrRateLimited):
		ginContext.JSON(http.StatusTooManyRequests, gin.H{"error": "search rate limited"})
	case errors.Is(err, domain.ErrInvalidSearchRequest), errors.Is(err, domain.ErrInvalidExtractRequest):
		ginContext.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		ginContext.JSON(http.StatusBadGateway, gin.H{"error": "search failed"})
	}
}
