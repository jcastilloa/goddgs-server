package hello

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const GetHelloHandlerLabel = "handler.hello.get"

type Get struct{}

func NewGet() Get {
	return Get{}
}

func (h Get) Handle(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"message": "hello world",
	})
}
