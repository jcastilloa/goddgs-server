package search

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jcastilloa/goddgs-server/search/domain"
)

type ExtractUseCase interface {
	Extract(context.Context, domain.ExtractRequest) (domain.ExtractResult, error)
}

const GetExtractHandlerLabel = "handler.search.extract.get"

type Extract struct {
	useCase ExtractUseCase
}

func NewExtract(useCase ExtractUseCase) Extract {
	return Extract{useCase: useCase}
}

func (h Extract) Handle(context *gin.Context) {
	request := domain.ExtractRequest{URL: context.Query("url"), Format: context.Query("format")}
	if err := request.Validate(); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.useCase.Extract(context.Request.Context(), request)
	if err != nil {
		writeSearchError(context, err)
		return
	}
	context.JSON(http.StatusOK, result)
}
