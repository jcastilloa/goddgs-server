package search

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jcastilloa/goddgs-server/search/domain"
	extractAIDomain "github.com/jcastilloa/goddgs-server/shared/extractai/domain"
)

type ExtractUseCase interface {
	Extract(context.Context, domain.ExtractRequest) (domain.ExtractResult, error)
}

type ExtractAIUseCase interface {
	Extract(context.Context, extractAIDomain.Request) (extractAIDomain.Result, error)
}

const GetExtractHandlerLabel = "handler.search.extract.get"

type Extract struct {
	heuristicUseCase ExtractUseCase
	aiUseCase        ExtractAIUseCase
}

func NewExtract(heuristicUseCase ExtractUseCase, aiUseCase ExtractAIUseCase) Extract {
	return Extract{heuristicUseCase: heuristicUseCase, aiUseCase: aiUseCase}
}

func (h Extract) Handle(context *gin.Context) {
	request := domain.ExtractRequest{URL: context.Query("url"), Format: context.Query("format"), Mode: domain.ExtractMode(context.Query("mode"))}
	if err := request.Validate(); err != nil {
		context.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if request.Mode.Normalize() == domain.ExtractModeAI {
		h.extractAI(context, request)
		return
	}
	h.extractHeuristic(context, request)
}

func (h Extract) extractHeuristic(context *gin.Context, request domain.ExtractRequest) {
	result, err := h.heuristicUseCase.Extract(context.Request.Context(), request)
	if err != nil {
		writeSearchError(context, err)
		return
	}
	context.JSON(http.StatusOK, result)
}

func (h Extract) extractAI(context *gin.Context, request domain.ExtractRequest) {
	if h.aiUseCase == nil {
		writeSearchError(context, fmt.Errorf("%w: configure llm.base_url, llm.api_key, extract_ai.model, extract_ai.timeout, extract_ai.temperature, and extract_ai.retries", extractAIDomain.ErrUnavailable))
		return
	}
	result, err := h.aiUseCase.Extract(context.Request.Context(), extractAIDomain.Request{URL: request.URL})
	if err != nil {
		writeSearchError(context, err)
		return
	}
	context.JSON(http.StatusOK, result)
}
