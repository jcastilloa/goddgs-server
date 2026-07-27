package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func (s *Server) registerDocumentation(apiPrefix, version string, requiresAuthentication bool) {
	s.engine.GET("/openapi.json", func(context *gin.Context) {
		context.JSON(http.StatusOK, openAPISpecification(apiPrefix, version, requiresAuthentication))
	})
	swaggerHandler := ginSwagger.WrapHandler(
		swaggerFiles.Handler,
		ginSwagger.URL("/openapi.json"),
		ginSwagger.PersistAuthorization(true),
	)
	s.engine.GET("/docs/*any", func(context *gin.Context) {
		if context.Param("any") == "/" {
			context.Request.URL.Path = "/docs/index.html"
			context.Request.RequestURI = "/docs/index.html"
		}
		swaggerHandler(context)
	})
}

func openAPISpecification(apiPrefix, version string, requiresAuthentication bool) gin.H {
	if version == "" {
		version = "0.1.0"
	}
	specification := gin.H{
		"openapi": "3.0.3",
		"info": gin.H{
			"title":   "goddgs-server API",
			"version": version,
		},
		"paths": gin.H{
			apiPrefix + "/text":   searchPath("Search text results.", nil),
			apiPrefix + "/images": searchPath("Search image results.", imageParameters()),
			apiPrefix + "/news":   searchPath("Search news results.", nil),
			apiPrefix + "/videos": searchPath("Search video results.", videoParameters()),
			apiPrefix + "/books":  searchPath("Search book results.", nil),
			apiPrefix + "/extract": gin.H{"get": gin.H{
				"summary": "Extract content from one URL.",
				"parameters": []gin.H{
					queryParameter("url", true, "string"),
					queryParameter("format", false, "string"),
				},
				"responses": standardResponses(),
			}},
		},
		"components": gin.H{
			"securitySchemes": gin.H{
				"bearerAuth": gin.H{"type": "http", "scheme": "bearer"},
			},
		},
	}
	if requiresAuthentication {
		specification["security"] = []gin.H{{"bearerAuth": []string{}}}
	}
	return specification
}

func searchPath(summary string, specificParameters []gin.H) gin.H {
	parameters := []gin.H{
		queryParameter("q", true, "string"),
		queryParameter("region", false, "string"),
		queryParameter("safesearch", false, "string"),
		queryParameter("timelimit", false, "string"),
		queryParameter("max_results", false, "integer"),
		queryParameter("page", false, "integer"),
		queryParameter("backend", false, "string"),
	}
	parameters = append(parameters, specificParameters...)
	return gin.H{"get": gin.H{
		"summary":    summary,
		"parameters": parameters,
		"responses":  standardResponses(),
	}}
}

func imageParameters() []gin.H {
	return []gin.H{
		queryParameter("size", false, "string"),
		queryParameter("color", false, "string"),
		queryParameter("type_image", false, "string"),
		queryParameter("layout", false, "string"),
		queryParameter("license_image", false, "string"),
	}
}

func videoParameters() []gin.H {
	return []gin.H{
		queryParameter("resolution", false, "string"),
		queryParameter("duration", false, "string"),
		queryParameter("license_videos", false, "string"),
	}
}

func queryParameter(name string, required bool, valueType string) gin.H {
	return gin.H{
		"name": name, "in": "query", "required": required,
		"schema": gin.H{"type": valueType},
	}
}

func standardResponses() gin.H {
	return gin.H{
		"200": gin.H{"description": "Successful response."},
		"400": gin.H{"description": "Invalid request."},
		"401": gin.H{"description": "Authentication required when enabled."},
		"429": gin.H{"description": "Search provider rate limit."},
		"502": gin.H{"description": "Search provider failure."},
		"504": gin.H{"description": "Request timed out."},
	}
}
