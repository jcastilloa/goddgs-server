package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) registerDocumentation(apiPrefix string) {
	s.engine.GET("/openapi.json", func(context *gin.Context) {
		context.JSON(http.StatusOK, openAPISpecification(apiPrefix))
	})
	s.engine.GET("/docs/", func(context *gin.Context) {
		context.Header("Content-Type", "text/html; charset=utf-8")
		context.String(http.StatusOK, swaggerHTML)
	})
}

func openAPISpecification(apiPrefix string) gin.H {
	return gin.H{
		"openapi": "3.1.0",
		"info": gin.H{
			"title":   "goddgs-server API",
			"version": "v1",
		},
		"paths": gin.H{
			apiPrefix + "/text":   searchPath("Search text results."),
			apiPrefix + "/images": searchPath("Search image results."),
			apiPrefix + "/news":   searchPath("Search news results."),
			apiPrefix + "/videos": searchPath("Search video results."),
			apiPrefix + "/books":  searchPath("Search book results."),
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
}

func searchPath(summary string) gin.H {
	return gin.H{"get": gin.H{
		"summary": summary,
		"parameters": []gin.H{
			queryParameter("q", true, "string"),
			queryParameter("region", false, "string"),
			queryParameter("safesearch", false, "string"),
			queryParameter("timelimit", false, "string"),
			queryParameter("max_results", false, "integer"),
			queryParameter("page", false, "integer"),
			queryParameter("backend", false, "string"),
		},
		"responses": standardResponses(),
	}}
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
		"502": gin.H{"description": "Search provider failure."},
		"504": gin.H{"description": "Request timed out."},
	}
}

const swaggerHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>goddgs-server API</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"></head>
<body><div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>SwaggerUIBundle({url: '/openapi.json', dom_id: '#swagger-ui', persistAuthorization: true})</script>
</body></html>`
