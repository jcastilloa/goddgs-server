package system

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const GetVersionHandlerLabel = "handler.system.version.get"

type GetVersion struct {
	version string
}

func NewGetVersion(version string) GetVersion {
	if version == "" {
		version = "0.1.0"
	}
	return GetVersion{version: version}
}

func (h GetVersion) Handle(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"version": h.version,
	})
}
