package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func (s *Server) registerDocumentation(group *gin.RouterGroup, apiPrefix, version string, requiresAuthentication bool) {
	group.GET("/openapi.json", func(context *gin.Context) {
		context.JSON(http.StatusOK, openAPISpecification(apiPrefix, version, requiresAuthentication))
	})
	swaggerHandler := ginSwagger.WrapHandler(
		swaggerFiles.Handler,
		ginSwagger.URL("/openapi.json"),
		ginSwagger.PersistAuthorization(true),
	)
	group.GET("/docs/*any", func(context *gin.Context) {
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
			{"name": "Research", "description": "Multi-source evidence-based web research."},
			{"name": "Operations", "description": "Cookie-authenticated operational dashboard, session lifecycle, and telemetry API."},
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
			apiPrefix + "/research":           researchPath(),
			"/operations":                     operationsDashboardPath(),
			"/operations/setup":               operationsSetupPagePath(),
			"/operations/login":               operationsLoginPagePath(),
			"/operations/api/auth/setup":      operationsSetupPath(),
			"/operations/api/auth/login":      operationsLoginPath(),
			"/operations/api/auth/session":    operationsSessionPath(),
			"/operations/api/auth/logout":     operationsLogoutPath(),
			"/operations/api/auth/password":   operationsPasswordPath(),
			"/operations/api/summary":         operationsSummaryPath(),
			"/operations/api/timeseries":      operationsTimeSeriesPath(),
			"/operations/api/operations":      operationsListPath(),
			"/operations/api/operations/{id}": operationsDetailPath(),
			"/operations/api/proxies":         operationsProxiesPath(),
		},
		"components": gin.H{
			"securitySchemes": gin.H{
				"bearerAuth":        gin.H{"type": "http", "scheme": "bearer"},
				"operationsSession": gin.H{"type": "apiKey", "in": "cookie", "name": "operations_session", "description": "Opaque HttpOnly dashboard session cookie. It is issued by setup or login; auth.token bearer credentials never authorize dashboard routes."},
			},
		},
	}
	if requiresAuthentication {
		specification["security"] = []gin.H{{"bearerAuth": []string{}}}
	}
	return specification
}

func researchPath() gin.H {
	return gin.H{"post": gin.H{
		"summary":     "Research a topic from web sources.",
		"description": researchDescription(),
		"tags":        []string{"Research"},
		"requestBody": gin.H{
			"required": true,
			"content":  jsonContent(researchRequestSchema(), gin.H{"default": gin.H{"value": gin.H{"query": "When was E.T. released and what was its opening box office?", "report_language": "en", "query_languages": []string{"en", "es"}, "query_count": 10, "results_per_query": 10}}}),
		},
		"responses": researchResponses(),
	}}
}

func researchDescription() string {
	return `Generates web-search queries with the configured query LLM, searches with goddgs, asks the configured selection LLM to choose result metadata, extracts only approved URLs through ` + "`mode=ai`" + ` behavior, and creates a sanitized HTML report with the configured report LLM.

- ` + "`query`" + ` is required. ` + "`report_language`" + ` is an ISO 639-1 code and defaults to ` + "`en`" + `.
- ` + "`query_languages`" + ` controls query generation and defaults to ` + "`[\"en\"]`" + `. ` + "`query_count`" + ` is the total number of generated queries, distributed across those languages; it defaults to ` + "`10`" + `.
- ` + "`results_per_query`" + ` defaults to ` + "`10`" + `. Discovery produces up to ` + "`query_count × results_per_query`" + ` valid unique URLs. The selector receives only a server-assigned candidate ID, title, description, and URL for up to ` + "`research.max_selection_candidates`" + ` candidates, then may approve up to ` + "`research.max_selected_sources`" + ` URLs by ID.
- ` + "`region`" + ` applies to every search when present. Otherwise it is derived per query language: ` + "`en → us-en`" + ` and ` + "`es → es-es`" + `. An unsupported query language requires an explicit region.
- Candidates not approved by selection, or omitted by its input limit, are not crawled or submitted to AI extraction. Failed, empty, invalid, or duplicate extractions are omitted silently. The report and ` + "`sources`" + ` contain only successfully extracted sources selected by the report model. All cited source IDs are verified by the server, and returned HTML is sanitized.
- ` + "`diagnostics`" + ` reports actual completed goddgs backend attempts (including result and error counts), ` + "`candidates_found`" + `, ` + "`candidates_selected`" + `, and elapsed milliseconds for query planning, searches, ` + "`source_selection_ms`" + `, parallel source extraction, report generation, and the total operation. Backend counts cover generated-query searches only, not source page downloads.

Research requires ` + "`llm.base_url`" + `, ` + "`llm.api_key`" + `, ` + "`extract_ai.*`" + `, ` + "`research.timeout`" + `, ` + "`research.max_selection_candidates`" + `, ` + "`research.max_selected_sources`" + `, ` + "`research.max_concurrent_extractions`" + `, ` + "`research.query_ai.*`" + `, ` + "`research.selection_ai.*`" + `, and ` + "`research.report_ai.*`" + `. Each configured LLM timeout applies to an attempt, and each ` + "`retries`" + ` setting is the number of additional retryable attempts. Its operation timeout is independent from the ordinary server request timeout.`
}

func researchRequestSchema() gin.H {
	return gin.H{
		"type":     "object",
		"required": []string{"query"},
		"properties": gin.H{
			"query":             gin.H{"type": "string", "minLength": 1, "description": "Generic research topic or question."},
			"report_language":   gin.H{"type": "string", "pattern": "^[A-Za-z]{2}$", "default": "en", "description": "ISO 639-1 language of the final HTML report."},
			"query_languages":   gin.H{"type": "array", "minItems": 1, "uniqueItems": true, "default": []string{"en"}, "items": gin.H{"type": "string", "pattern": "^[A-Za-z]{2}$"}, "description": "ISO 639-1 languages used to generate search queries."},
			"query_count":       gin.H{"type": "integer", "minimum": 1, "default": 10, "description": "Total generated queries across query_languages."},
			"results_per_query": gin.H{"type": "integer", "minimum": 1, "default": 10, "description": "Maximum results requested for each generated query."},
			"region":            gin.H{"type": "string", "description": "Optional goddgs region override for all generated queries."},
		},
	}
}

func researchResponses() gin.H {
	return gin.H{
		"200": jsonResponse("Research completed. URLs rejected by selection are not crawled; individual approved but inaccessible sources are omitted without being reported.", researchResultSchema(), "research", gin.H{"report_html": "<article><h1>E.T.</h1><p>E.T. premiered in 1982 and opened with …</p></article>", "sources": []gin.H{{"url": "https://example.com/et", "title": "E.T. release and box office"}}, "diagnostics": gin.H{"backends": []gin.H{{"name": "google", "provider": "google", "attempts": 2, "result_count": 10, "error_count": 0}}, "query_planning_ms": 220, "search_ms": 1400, "source_selection_ms": 340, "source_extraction_ms": 8300, "report_generation_ms": 570, "total_ms": 10830, "candidates_found": 32, "candidates_selected": 12}}),
		"400": errorResponse("The JSON body or research parameters are invalid.", "invalid_request", "invalid research request: query is required"),
		"401": errorResponse("Authentication is required when enabled.", "authentication_required", "unauthorized"),
		"429": errorResponse("A configured LLM rate limited research.", "rate_limited", "research rate limited"),
		"499": errorResponse("The client canceled the research request.", "request_canceled", "request canceled"),
		"502": errorResponse("Query generation, source selection, the report, or all approved source extraction failed. Individual source failures are intentionally not exposed.", "upstream_failure", "research failed"),
		"503": errorResponse("Research AI or AI extraction configuration is unavailable.", "research_not_configured", "research is unavailable: research query AI model is required"),
		"504": errorResponse("The research operation timeout elapsed.", "timeout", "research timed out"),
	}
}

func researchResultSchema() gin.H {
	return gin.H{
		"type":     "object",
		"required": []string{"report_html", "sources", "diagnostics"},
		"properties": gin.H{
			"report_html": gin.H{"type": "string", "description": "Sanitized HTML research report."},
			"sources":     gin.H{"type": "array", "description": "Successfully extracted sources selected for the report.", "items": gin.H{"type": "object", "required": []string{"url", "title"}, "properties": gin.H{"url": gin.H{"type": "string", "format": "uri"}, "title": gin.H{"type": "string"}}}},
			"diagnostics": gin.H{"type": "object", "description": "Observed research workflow timings, candidate counts, and completed search-backend attempts. Durations are elapsed milliseconds; backend data excludes source page downloads.", "required": []string{"backends", "query_planning_ms", "search_ms", "source_selection_ms", "source_extraction_ms", "report_generation_ms", "total_ms", "candidates_found", "candidates_selected"}, "properties": gin.H{
				"backends":             gin.H{"type": "array", "description": "Completed goddgs backend attempts aggregated across generated queries.", "items": gin.H{"type": "object", "required": []string{"name", "provider", "attempts", "result_count", "error_count"}, "properties": gin.H{"name": gin.H{"type": "string", "description": "goddgs backend name."}, "provider": gin.H{"type": "string", "description": "goddgs provider label used for scheduler de-duplication."}, "attempts": gin.H{"type": "integer", "minimum": 0}, "result_count": gin.H{"type": "integer", "minimum": 0}, "error_count": gin.H{"type": "integer", "minimum": 0}}}},
				"query_planning_ms":    gin.H{"type": "integer", "minimum": 0, "description": "Query-planning LLM elapsed time in milliseconds."},
				"search_ms":            gin.H{"type": "integer", "minimum": 0, "description": "All generated-query searches elapsed time in milliseconds."},
				"source_selection_ms":  gin.H{"type": "integer", "minimum": 0, "description": "Search-result metadata selection phase elapsed time in milliseconds."},
				"source_extraction_ms": gin.H{"type": "integer", "minimum": 0, "description": "Parallel source AI extraction elapsed time for selected URLs only."},
				"report_generation_ms": gin.H{"type": "integer", "minimum": 0, "description": "Report-writing LLM elapsed time in milliseconds."},
				"total_ms":             gin.H{"type": "integer", "minimum": 0, "description": "Total successful research operation elapsed time in milliseconds."},
				"candidates_found":     gin.H{"type": "integer", "minimum": 0, "description": "Valid, URL-deduplicated search-result candidates discovered before selector input limiting."},
				"candidates_selected":  gin.H{"type": "integer", "minimum": 0, "description": "Validated candidate URLs approved before source extraction."},
			}},
		},
	}
}

func modeParameter() gin.H {
	return gin.H{
		"name": "mode", "in": "query", "required": false,
		"description": "Extraction strategy. `heuristic` is the default and uses goddgs except that configured Chrome handles `format=html`. `ai` sends sanitized HTML from that same source path to the configured OpenAI-compatible LLM and returns sanitized primary-content HTML.",
		"schema":      gin.H{"type": "string", "default": "heuristic", "enum": []string{"heuristic", "ai"}},
		"example":     "heuristic",
	}
}

func extractResponses() gin.H {
	return gin.H{
		"200": gin.H{
			"description": "Extraction completed. `Content` is determined by the selected mode and format.",
			"content": jsonContent(extractResultSchema(), gin.H{
				"heuristic": gin.H{
					"summary": "Heuristic extraction",
					"value":   gin.H{"URL": "https://example.com/article", "Content": "# Article title\n\nArticle body"},
				},
				"ai": gin.H{
					"summary": "AI primary-content extraction",
					"value":   gin.H{"URL": "https://example.com/article", "Content": "<article><h1>Article title</h1><p>Article body.</p></article>"},
				},
				"html": gin.H{
					"summary": "Sanitized HTML extraction",
					"value":   gin.H{"URL": "https://example.com/article", "Content": "<h1>Article title</h1><p>Article body.</p>"},
				},
			}),
		},
		"400": errorResponse("The URL is invalid, or mode is not `heuristic` or `ai`.", "invalid_mode", "invalid extract request: unsupported extract mode \"other\""),
		"401": errorResponse("Authentication is required when enabled.", "authentication_required", "authentication required"),
		"429": errorResponse("The source or the AI provider rate limited the request.", "rate_limited", "AI extraction rate limited"),
		"502": errorResponse("The source page or the configured AI provider could not be reached or returned an invalid response.", "upstream_failure", "search failed"),
		"503": errorResponse("AI extraction is not configured, or optional Chrome HTML loading cannot start. Chrome failures never change non-HTML extraction.", "extraction_unavailable", "HTML browser extraction is unavailable"),
		"504": errorResponse("The request, configured AI timeout, or configured Chrome page timeout elapsed.", "timeout", "request timed out"),
	}
}

func extractDescription() string {
	return `Choose the extraction strategy with ` + "`mode`" + `.

- ` + "`mode=heuristic`" + ` (default) preserves the existing goddgs extraction behavior. Use ` + "`format`" + ` to choose its output: ` + "`text_markdown`" + ` (default), ` + "`text_plain`" + `, ` + "`text_rich`" + `, ` + "`text`" + `, ` + "`content`" + `, or ` + "`html`" + `. When optional ` + "`chrome.enabled`" + ` is false, ` + "`html`" + ` renders extracted Markdown as sanitized HTML. When it is true, ` + "`html`" + ` loads and sanitizes rendered page DOM through Chrome/Chromium. There is no public browser mode parameter; every non-HTML format keeps the goddgs path. ` + "`content`" + ` returns the unprocessed source document and can be Base64-encoded in JSON.
- ` + "`mode=ai`" + ` asks an OpenAI-compatible ` + "`POST /chat/completions`" + ` provider for the page's primary editorial content. It supplies sanitized ` + "`format=html`" + ` output, so with Chrome enabled AI extraction and selected research sources use the rendered DOM. The response is always sanitized HTML; ` + "`format`" + ` is ignored in this mode. Navigation, sidebars, ads, banners, cookie notices, related content, scripts, forms, embedded content, unsafe URLs, event handlers, and presentation attributes are removed. Links retain only ` + "`href`" + `; images retain only ` + "`src`" + ` and ` + "`alt`" + `. URLs are not rewritten. If no editorial content exists, ` + "`Content`" + ` is an empty string.

Chrome requires a compatible executable and positive ` + "`chrome.timeout`" + `, ` + "`chrome.max_browsers`" + `, ` + "`chrome.max_pages_per_browser`" + `, and ` + "`chrome.idle_timeout`" + ` when enabled. It uses the same health-aware direct and SSH proxy rotation as goddgs. A page receives the earlier of its caller deadline and ` + "`chrome.timeout`" + `; unavailable browser or proxy capacity returns ` + "`503`" + `, page timeout returns ` + "`504`" + `, and a page navigation failure returns ` + "`502`" + `. Proxy credentials, browser paths, and CDP details are never returned.

AI mode requires ` + "`llm.base_url`" + `, ` + "`llm.api_key`" + `, ` + "`extract_ai.model`" + `, ` + "`extract_ai.timeout`" + `, ` + "`extract_ai.temperature`" + `, and ` + "`extract_ai.retries`" + `. It is not constrained by ` + "`service.request_timeout`" + `: source loading is bounded by ` + "`chrome.timeout`" + ` when Chrome is enabled, then each LLM attempt gets ` + "`extract_ai.timeout`" + `. ` + "`extract_ai.retries`" + ` is the number of additional attempts after the first for retryable transport errors, rate limits, retryable HTTP statuses, and attempt timeouts. When it is not usable, this endpoint returns ` + "`503`" + ` with the missing or invalid configuration; ` + "`mode=heuristic`" + ` continues to work.`
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
		"description": "Output format used only by `mode=heuristic`. With `chrome.enabled=false`, `html` renders extracted Markdown as sanitized HTML; when enabled, it returns sanitized rendered DOM from Chrome/Chromium. `content` returns the unprocessed source document, which can be Base64-encoded in JSON. Ignored by `mode=ai`, which always returns sanitized HTML.",
		"schema": gin.H{
			"type": "string", "default": "text_markdown",
			"enum": []string{"text_markdown", "text_plain", "text_rich", "text", "content", "html"},
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
			"Content": gin.H{"description": "Extracted content. `format=html` is sanitized HTML; with Chrome enabled it is the rendered DOM. AI mode always returns sanitized HTML."},
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
