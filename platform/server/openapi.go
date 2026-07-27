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
			"title":       "goddgs-server API",
			"version":     version,
			"description": "HTTP API for goddgs metasearch and content extraction. Search results preserve the source fields and value types returned by the selected backends.",
		},
		"tags": []gin.H{
			{"name": "System", "description": "Service version endpoint."},
			{"name": "Search", "description": "goddgs metasearch endpoints."},
			{"name": "Extraction", "description": "Page-content extraction endpoints."},
		},
		"paths": gin.H{
			apiPrefix + "/version": versionPath(),
			apiPrefix + "/text":    searchPath("Search text results.", "Search web pages and return raw source-shaped result objects.", commonSearchParameters()),
			apiPrefix + "/images":  searchPath("Search image results.", "Search images. The common search parameters and image filters are passed to the selected goddgs backends.", append(commonSearchParameters(), imageParameters()...)),
			apiPrefix + "/news":    searchPath("Search news results.", "Search news result metadata. This endpoint does not download article bodies; pass a result URL to `/extract` for that.", commonSearchParameters()),
			apiPrefix + "/videos":  searchPath("Search video results.", "Search video result metadata with optional video filters.", append(commonSearchParameters(), videoParameters()...)),
			apiPrefix + "/books":   searchPath("Search book results.", "Search books. Books support the query, result-count, page, and backend parameters only.", bookParameters()),
			apiPrefix + "/extract": gin.H{"get": gin.H{
				"summary":     "Extract the content of one URL.",
				"description": extractDescription(),
				"tags":        []string{"Extraction"},
				"parameters": []gin.H{
					extractURLParameter(),
					extractFormatParameter(),
					modeParameter(),
				},
				"responses": extractResponses(),
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

func modeParameter() gin.H {
	return gin.H{
		"name": "mode", "in": "query", "required": false,
		"description": "Extraction strategy. `heuristic` is the default and delegates to goddgs. `ai` downloads the source HTML through goddgs, sends it to the configured OpenAI-compatible LLM, and returns sanitized primary-content HTML.",
		"schema":      gin.H{"type": "string", "default": "heuristic", "enum": []string{"heuristic", "ai"}},
		"example":     "heuristic",
	}
}

func extractResponses() gin.H {
	return gin.H{
		"200": gin.H{
			"description": "Extraction completed. `Content` is determined by the selected mode.",
			"content": jsonContent(extractResultSchema(), gin.H{
				"heuristic": gin.H{
					"summary": "Heuristic extraction",
					"value":   gin.H{"URL": "https://example.com/article", "Content": "# Article title\n\nArticle body"},
				},
				"ai": gin.H{
					"summary": "AI primary-content extraction",
					"value":   gin.H{"URL": "https://example.com/article", "Content": "<article><h1>Article title</h1><p>Article body.</p></article>"},
				},
			}),
		},
		"400": errorResponse("The URL is invalid, or mode is not `heuristic` or `ai`.", "invalid_mode", "invalid extract request: unsupported extract mode \"other\""),
		"401": errorResponse("Authentication is required when enabled.", "authentication_required", "authentication required"),
		"429": errorResponse("The source or the AI provider rate limited the request.", "rate_limited", "AI extraction rate limited"),
		"502": errorResponse("The source page or the configured AI provider could not be reached or returned an invalid response.", "upstream_failure", "search failed"),
		"503": errorResponse("AI extraction is not configured or unavailable. Heuristic extraction remains available.", "ai_not_configured", "AI extraction is unavailable: llm.api_key is required"),
		"504": errorResponse("The request or configured AI timeout elapsed.", "timeout", "request timed out"),
	}
}

func extractDescription() string {
	return `Choose the extraction strategy with ` + "`mode`" + `.

- ` + "`mode=heuristic`" + ` (default) preserves the existing goddgs extraction behavior. Use ` + "`format`" + ` to choose its output: ` + "`text_markdown`" + ` (default), ` + "`text_plain`" + `, ` + "`text_rich`" + `, ` + "`text`" + `, or ` + "`content`" + `.
- ` + "`mode=ai`" + ` fetches the original HTML through goddgs and asks an OpenAI-compatible ` + "`POST /chat/completions`" + ` provider for the page's primary editorial content. The response is always sanitized HTML; ` + "`format`" + ` is ignored in this mode. Navigation, sidebars, ads, banners, cookie notices, related content, scripts, forms, embedded content, unsafe URLs, event handlers, and presentation attributes are removed. Links retain only ` + "`href`" + `; images retain only ` + "`src`" + ` and ` + "`alt`" + `. URLs are not rewritten. If no editorial content exists, ` + "`Content`" + ` is an empty string.

AI mode requires ` + "`llm.base_url`" + `, ` + "`llm.api_key`" + `, ` + "`extract_ai.model`" + `, ` + "`extract_ai.timeout`" + `, ` + "`extract_ai.temperature`" + `, and ` + "`extract_ai.retries`" + `. When it is not usable, this endpoint returns ` + "`503`" + ` with the missing or invalid configuration; ` + "`mode=heuristic`" + ` continues to work.`
}

func extractURLParameter() gin.H {
	return gin.H{
		"name": "url", "in": "query", "required": true,
		"description": "Absolute HTTP or HTTPS URL of the page to extract.",
		"schema":      gin.H{"type": "string", "format": "uri"},
		"example":     "https://example.com/article",
	}
}

func extractFormatParameter() gin.H {
	return gin.H{
		"name": "format", "in": "query", "required": false,
		"description": "Output format used only by `mode=heuristic`. Ignored by `mode=ai`, which always returns sanitized HTML.",
		"schema": gin.H{
			"type": "string", "default": "text_markdown",
			"enum": []string{"text_markdown", "text_plain", "text_rich", "text", "content"},
		},
		"example": "text_plain",
	}
}

func extractResultSchema() gin.H {
	return gin.H{
		"type":     "object",
		"required": []string{"URL", "Content"},
		"properties": gin.H{
			"URL":     gin.H{"type": "string", "format": "uri", "description": "Final source URL after redirects, when available."},
			"Content": gin.H{"description": "Extracted content. Dynamic source content in heuristic mode; sanitized HTML string in AI mode."},
		},
	}
}

func errorResponse(description, name, message string) gin.H {
	return gin.H{
		"description": description,
		"content":     jsonContent(gin.H{"type": "object", "required": []string{"error"}, "properties": gin.H{"error": gin.H{"type": "string"}}}, gin.H{name: gin.H{"value": gin.H{"error": message}}}),
	}
}

func jsonContent(schema gin.H, examples gin.H) gin.H {
	return gin.H{"application/json": gin.H{"schema": schema, "examples": examples}}
}

func versionPath() gin.H {
	return gin.H{"get": gin.H{
		"summary":     "Get the API version.",
		"description": "Returns the version configured for this goddgs-server instance.",
		"tags":        []string{"System"},
		"responses": gin.H{
			"200": jsonResponse("Configured API version.", versionSchema(), "version", gin.H{"version": "0.1.0"}),
			"401": errorResponse("Authentication is required when enabled.", "authentication_required", "authentication required"),
		},
	}}
}

func searchPath(summary, description string, parameters []gin.H) gin.H {
	return gin.H{"get": gin.H{
		"summary":     summary,
		"description": description,
		"parameters":  parameters,
		"responses":   searchResponses(),
		"tags":        []string{"Search"},
	}}
}

func commonSearchParameters() []gin.H {
	return []gin.H{
		queryParameter("q", false, "Search query. Either `q` or `query` is required; `q` takes precedence when both are supplied.", gin.H{"type": "string", "minLength": 1}, "artificial intelligence"),
		queryParameter("query", false, "Alias for `q`. It is ignored when `q` is provided.", gin.H{"type": "string", "minLength": 1}, "artificial intelligence"),
		queryParameter("region", false, "Source locale or region. Defaults to `us-en`; accepted values depend on the selected backend.", defaultSchema("string", "us-en"), "es-es"),
		queryParameter("safesearch", false, "Safe-search mode. Defaults to `moderate`; common values are `on`, `moderate`, and `off`, subject to backend support.", defaultSchema("string", "moderate"), "moderate"),
		queryParameter("timelimit", false, "Optional freshness filter. Common values are `d` (day), `w` (week), `m` (month), and `y` (year), subject to backend support.", gin.H{"type": "string"}, "w"),
		queryParameter("max_results", false, "Maximum number of results to return. Must be a positive integer; defaults to `10`.", gin.H{"type": "integer", "minimum": 1, "default": 10}, 10),
		queryParameter("page", false, "Result page. Must be a positive integer; defaults to `1`.", gin.H{"type": "integer", "minimum": 1, "default": 1}, 1),
		queryParameter("backend", false, "Backend selector. Defaults to `auto`. It may be a backend name or a comma-separated list; support varies by endpoint.", defaultSchema("string", "auto"), "auto"),
	}
}

func bookParameters() []gin.H {
	return []gin.H{
		queryParameter("q", false, "Book search query. Either `q` or `query` is required; `q` takes precedence when both are supplied.", gin.H{"type": "string", "minLength": 1}, "The Go Programming Language"),
		queryParameter("query", false, "Alias for `q`. It is ignored when `q` is provided.", gin.H{"type": "string", "minLength": 1}, "The Go Programming Language"),
		queryParameter("max_results", false, "Maximum number of books to return. Must be a positive integer; defaults to `10`.", gin.H{"type": "integer", "minimum": 1, "default": 10}, 10),
		queryParameter("page", false, "Result page. Must be a positive integer; defaults to `1`.", gin.H{"type": "integer", "minimum": 1, "default": 1}, 1),
		queryParameter("backend", false, "Book backend selector. Defaults to `auto`; the bundled book backend is `annasarchive`.", defaultSchema("string", "auto"), "annasarchive"),
	}
}

func imageParameters() []gin.H {
	return []gin.H{
		queryParameter("size", false, "Image size filter accepted by the selected backend, for example `Small`, `Medium`, `Large`, or `Wallpaper`.", gin.H{"type": "string"}, "Large"),
		queryParameter("color", false, "Image colour filter accepted by the selected backend, for example `Color`, `Monochrome`, or a colour name.", gin.H{"type": "string"}, "Blue"),
		queryParameter("type_image", false, "Image type filter accepted by the selected backend, for example `photo`, `clipart`, `gif`, or `transparent`.", gin.H{"type": "string"}, "photo"),
		queryParameter("layout", false, "Image layout filter accepted by the selected backend, for example `Square`, `Tall`, or `Wide`.", gin.H{"type": "string"}, "Wide"),
		queryParameter("license_image", false, "Image licence filter accepted by the selected backend, for example `Any`, `Public`, `Share`, or `ShareCommercially`.", gin.H{"type": "string"}, "Share"),
	}
}

func videoParameters() []gin.H {
	return []gin.H{
		queryParameter("resolution", false, "Video resolution filter accepted by the selected backend, for example `high` or `standard`.", gin.H{"type": "string"}, "high"),
		queryParameter("duration", false, "Video duration filter accepted by the selected backend, for example `short`, `medium`, or `long`.", gin.H{"type": "string"}, "short"),
		queryParameter("license_videos", false, "Video licence or host filter accepted by the selected backend, for example `youtube` or `creativeCommon`.", gin.H{"type": "string"}, "youtube"),
	}
}

func queryParameter(name string, required bool, description string, schema gin.H, example any) gin.H {
	return gin.H{
		"name": name, "in": "query", "required": required,
		"description": description,
		"schema":      schema,
		"example":     example,
	}
}

func searchResponses() gin.H {
	return gin.H{
		"200": jsonResponse("Search completed. Result field names and value types are preserved from the selected goddgs backends and therefore vary by category and provider.", rawResultsSchema(), "results", []gin.H{{"title": "Example result", "href": "https://example.com/result", "body": "Result excerpt."}}),
		"400": errorResponse("The query is missing, or `max_results` or `page` is not a positive integer.", "invalid_request", "invalid search request: query is required"),
		"401": errorResponse("Authentication is required when enabled.", "authentication_required", "authentication required"),
		"429": errorResponse("A selected search provider rate limited the request.", "rate_limited", "search rate limited"),
		"502": errorResponse("A selected search provider failed or could not be reached.", "upstream_failure", "search failed"),
		"503": errorResponse("No configured upstream proxy connection is healthy.", "no_healthy_proxy", "no healthy upstream connection available"),
		"504": errorResponse("The server request timeout or a source search timeout elapsed.", "timeout", "search timed out"),
	}
}

func jsonResponse(description string, schema gin.H, name string, value any) gin.H {
	return gin.H{"description": description, "content": jsonContent(schema, gin.H{name: gin.H{"value": value}})}
}

func rawResultsSchema() gin.H {
	return gin.H{"type": "array", "items": gin.H{"type": "object", "additionalProperties": true}}
}

func versionSchema() gin.H {
	return gin.H{"type": "object", "required": []string{"version"}, "properties": gin.H{"version": gin.H{"type": "string", "description": "Configured API version."}}}
}

func defaultSchema(valueType string, value any) gin.H {
	return gin.H{"type": valueType, "default": value}
}

func enumSchema(defaultValue string, values ...string) gin.H {
	return gin.H{"type": "string", "default": defaultValue, "enum": values}
}
