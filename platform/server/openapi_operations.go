package server

import "github.com/gin-gonic/gin"

func operationsDashboardPath() gin.H {
	return gin.H{"get": gin.H{
		"summary":     "Open the operations dashboard.",
		"description": "Serves the embedded HTML dashboard after a valid `operations_session` cookie is supplied. Without that session the server redirects with 303 to `/operations/setup` until the first local account exists, then to `/operations/login`. It loads Tailwind CSS and Chart.js from CDNs and refreshes the operations JSON API every five seconds. `auth.token` bearer authentication is not accepted for this route.",
		"tags":        []string{"Operations"},
		"security":    operationsSecurity(),
		"responses": gin.H{
			"200": gin.H{"description": "Operations dashboard HTML.", "content": gin.H{"text/html": gin.H{"schema": gin.H{"type": "string"}}}},
			"303": gin.H{"description": "Redirect to first-time setup or login when no valid dashboard session is present."},
		},
	}}
}

func operationsSetupPagePath() gin.H {
	return operationsPublicPagePath("Open first-time dashboard setup.", "Displays the first-account form only while no dashboard user exists. A valid session redirects to `/operations`; after setup completes it redirects to `/operations/login`.")
}

func operationsLoginPagePath() gin.H {
	return operationsPublicPagePath("Open dashboard login.", "Displays the dashboard login form after first-time setup. A valid session redirects to `/operations`; if no account exists it redirects to `/operations/setup`.")
}

func operationsPublicPagePath(summary, description string) gin.H {
	return gin.H{"get": gin.H{"summary": summary, "description": description, "tags": []string{"Operations"}, "security": []gin.H{}, "responses": gin.H{
		"200": gin.H{"description": "Embedded HTML access page.", "content": gin.H{"text/html": gin.H{"schema": gin.H{"type": "string"}}}},
		"303": gin.H{"description": "Redirect according to the setup and session state."},
	}}}
}

func operationsSetupPath() gin.H {
	return operationsCredentialsPath("Create the initial dashboard account.", "Creates the one local dashboard account only when setup has not been completed. On success it sets the HttpOnly `operations_session` cookie and the readable `operations_csrf` cookie. A simultaneous or later setup returns `setup_completed`.", "201", "Dashboard account and session created.", gin.H{
		"400": errorResponse("Username or password violates the documented policy.", "invalid_input", "username must be 3-64 ASCII letters, digits, dots, underscores, or hyphens; password must be 12-128 characters"),
		"409": errorResponse("The local dashboard account already exists.", "setup_completed", "setup_completed"),
		"500": errorResponse("The dashboard authentication store is unavailable.", "operations_auth_unavailable", "operations dashboard authentication is unavailable"),
	})
}

func operationsLoginPath() gin.H {
	return operationsCredentialsPath("Start a dashboard session.", "Verifies the one local dashboard account and sets the HttpOnly `operations_session` cookie plus the readable `operations_csrf` cookie. Invalid usernames and passwords intentionally share one error.", "200", "Dashboard session created.", gin.H{
		"400": errorResponse("The request body is not valid JSON.", "invalid_request", "invalid dashboard authentication request"),
		"401": errorResponse("Credentials are invalid.", "invalid_credentials", "invalid dashboard credentials"),
		"500": errorResponse("The dashboard authentication store is unavailable.", "operations_auth_unavailable", "operations dashboard authentication is unavailable"),
	})
}

func operationsCredentialsPath(summary, description, successStatus, successDescription string, failures gin.H) gin.H {
	responses := gin.H{successStatus: sessionJSONResponse(successDescription, httpCookieHeader())}
	for status, response := range failures {
		responses[status] = response
	}
	return gin.H{"post": gin.H{"summary": summary, "description": description, "tags": []string{"Operations"}, "security": []gin.H{}, "requestBody": gin.H{"required": true, "content": jsonContent(dashboardCredentialsSchema(), gin.H{"default": gin.H{"value": gin.H{"username": "operator", "password": "a secure password"}}})}, "responses": responses}}
}

func operationsSessionPath() gin.H {
	return gin.H{"get": gin.H{"summary": "Get the dashboard session identity.", "description": "Returns the local dashboard username for a valid cookie session. It never returns credentials, password hashes, tokens, or CSRF values.", "tags": []string{"Operations"}, "security": operationsSecurity(), "responses": gin.H{
		"200": jsonResponse("Current dashboard session.", dashboardIdentitySchema(), "session", gin.H{"username": "operator"}),
		"401": dashboardAuthenticationResponse(),
		"500": errorResponse("The dashboard authentication store is unavailable.", "operations_auth_unavailable", "operations dashboard authentication is unavailable"),
	}}}
}

func operationsLogoutPath() gin.H {
	return gin.H{"post": gin.H{"summary": "Close the dashboard session.", "description": "Revokes the current opaque session and clears both dashboard cookies. The `X-Operations-CSRF` header must exactly match the readable `operations_csrf` cookie.", "tags": []string{"Operations"}, "security": operationsSecurity(), "parameters": []gin.H{csrfParameter()}, "responses": gin.H{
		"204": gin.H{"description": "Session revoked and cookies cleared.", "headers": clearedCookieHeader()},
		"401": dashboardAuthenticationResponse(),
		"403": errorResponse("The CSRF cookie/header pair is missing or invalid.", "invalid_csrf", "invalid dashboard CSRF token"),
		"500": errorResponse("The dashboard authentication store is unavailable.", "operations_auth_unavailable", "operations dashboard authentication is unavailable"),
	}}}
}

func operationsPasswordPath() gin.H {
	return gin.H{"post": gin.H{"summary": "Change the dashboard password.", "description": "Requires the current password, a different 12–128 character new password, a valid dashboard session, and the matching `X-Operations-CSRF` header. On success all prior sessions are revoked and replacement session/CSRF cookies are set for this browser.", "tags": []string{"Operations"}, "security": operationsSecurity(), "parameters": []gin.H{csrfParameter()}, "requestBody": gin.H{"required": true, "content": jsonContent(dashboardPasswordSchema(), gin.H{"default": gin.H{"value": gin.H{"current_password": "a secure password", "new_password": "a different secure password"}}})}, "responses": gin.H{
		"204": gin.H{"description": "Password changed; previous sessions revoked and replacement cookies set.", "headers": httpCookieHeader()},
		"400": errorResponse("The JSON body or new password is invalid, including when it matches the current password.", "password_unchanged", "new dashboard password must be different from the current password"),
		"401": errorResponse("The session is missing or the current password is incorrect.", "invalid_credentials", "invalid dashboard credentials"),
		"403": errorResponse("The CSRF cookie/header pair is missing or invalid.", "invalid_csrf", "invalid dashboard CSRF token"),
		"500": errorResponse("The dashboard authentication store is unavailable.", "operations_auth_unavailable", "operations dashboard authentication is unavailable"),
	}}}
}

func operationsSecurity() []gin.H { return []gin.H{{"operationsSession": []string{}}} }

func dashboardAuthenticationResponse() gin.H {
	return errorResponse("A valid dashboard cookie session is required.", "authentication_required", "dashboard authentication required")
}

func csrfParameter() gin.H {
	return gin.H{"name": "X-Operations-CSRF", "in": "header", "required": true, "description": "Value from the readable operations_csrf cookie. Required for authenticated state-changing requests.", "schema": gin.H{"type": "string"}, "example": "csrf-token"}
}

func dashboardCredentialsSchema() gin.H {
	return gin.H{"type": "object", "required": []string{"username", "password"}, "properties": gin.H{"username": gin.H{"type": "string", "minLength": 3, "maxLength": 64, "pattern": "^[A-Za-z0-9._-]+$", "description": "Local dashboard username; surrounding whitespace is removed."}, "password": gin.H{"type": "string", "format": "password", "minLength": 12, "maxLength": 128, "description": "Password; surrounding whitespace is removed and it is never returned."}}}
}

func dashboardPasswordSchema() gin.H {
	return gin.H{"type": "object", "required": []string{"current_password", "new_password"}, "properties": gin.H{"current_password": gin.H{"type": "string", "format": "password"}, "new_password": gin.H{"type": "string", "format": "password", "minLength": 12, "maxLength": 128}}}
}

func dashboardIdentitySchema() gin.H {
	return gin.H{"type": "object", "required": []string{"username"}, "properties": gin.H{"username": gin.H{"type": "string"}}}
}

func sessionJSONResponse(description string, headers gin.H) gin.H {
	response := jsonResponse(description+" Response headers set `operations_session` and `operations_csrf` cookies; `operations_session` is HttpOnly and both cookies use SameSite=Strict and Path=/operations.", dashboardIdentitySchema(), "session", gin.H{"username": "operator"})
	response["headers"] = headers
	return response
}

func httpCookieHeader() gin.H {
	return gin.H{"Set-Cookie": gin.H{"description": "Issues both operations_session and operations_csrf cookies. The former is HttpOnly; both use SameSite=Strict and Path=/operations.", "schema": gin.H{"type": "string"}}}
}

func clearedCookieHeader() gin.H {
	return gin.H{"Set-Cookie": gin.H{"description": "Clears both operations_session and operations_csrf cookies.", "schema": gin.H{"type": "string"}}}
}

func operationsSummaryPath() gin.H {
	return operationsJSONPath("Get the operations summary.", "Counts active, successful, and failed operations plus p50/p95 durations for the selected time range.", operationsRangeParameters(), operationsSummarySchema(), gin.H{"active": 2, "succeeded": 48, "failed": 3, "p50_ms": 120, "p95_ms": 900})
}

func operationsTimeSeriesPath() gin.H {
	parameters := append(operationsRangeParameters(), queryParameter("interval", false, "Aggregation interval. Defaults to `1h` for 24h, `6h` for 7d, and `24h` for 30d. Allowed values are `1h`, `6h`, and `24h`; it cannot exceed the selected range.", enumSchema("1h", "1h", "6h", "24h"), "1h"))
	return operationsJSONPath("Get operations time series.", "Returns buckets with succeeded/failed volume and p50/p95 duration for the selected range.", parameters, gin.H{"type": "array", "items": operationsBucketSchema()}, []gin.H{{"started_at": "2026-07-28T10:00:00Z", "succeeded": 8, "failed": 1, "p50_ms": 110, "p95_ms": 840}})
}

func operationsListPath() gin.H {
	parameters := append(operationsRangeParameters(),
		queryParameter("status", false, "Optional operation status filter.", enumSchema("running", "running", "succeeded", "failed"), "succeeded"),
		queryParameter("type", false, "Optional operation type filter.", enumSchema("search", "search", "extract", "research"), "search"),
		queryParameter("limit", false, "Page size; defaults to 50 and must be between 1 and 100.", gin.H{"type": "integer", "minimum": 1, "maximum": 100, "default": 50}, 50),
		queryParameter("offset", false, "Zero-based page offset; defaults to 0 and must be between 0 and 10000.", gin.H{"type": "integer", "minimum": 0, "maximum": 10000, "default": 0}, 0),
	)
	return operationsJSONPath("List recent operations.", "Returns a sanitized, paginated list of operations. The selected range defaults to 24 hours.", parameters, gin.H{"type": "object", "required": []string{"operations", "total"}, "properties": gin.H{"operations": gin.H{"type": "array", "items": operationSchema()}, "total": gin.H{"type": "integer", "minimum": 0}}}, gin.H{"operations": []gin.H{{"id": "operation-123", "type": "search", "status": "succeeded", "started_at": "2026-07-28T10:00:00Z", "duration_ms": 120, "result": "succeeded"}}, "total": 1})
}

func operationsDetailPath() gin.H {
	return gin.H{"get": gin.H{
		"summary":     "Get one operation.",
		"description": "Returns the sanitized operation, its recorded steps, and categorized errors. It never exposes credentials, request bodies, prompts, or provider responses.",
		"tags":        []string{"Operations"},
		"security":    operationsSecurity(),
		"parameters":  []gin.H{{"name": "id", "in": "path", "required": true, "description": "Opaque operation identifier.", "schema": gin.H{"type": "string"}, "example": "operation-123"}},
		"responses": gin.H{
			"200": jsonResponse("Sanitized operation detail.", gin.H{"type": "object", "required": []string{"operation", "steps", "errors"}, "properties": gin.H{"operation": operationSchema(), "steps": gin.H{"type": "array", "items": stepSchema()}, "errors": gin.H{"type": "array", "items": operationErrorSchema()}}}, "operation", gin.H{"operation": gin.H{"id": "operation-123", "type": "search", "status": "failed"}, "steps": []gin.H{}, "errors": []gin.H{{"category": "timeout", "message": "request timed out"}}}),
			"400": errorResponse("The operation identifier is blank.", "invalid_operation_id", "operation id is required"),
			"404": errorResponse("No operation matches the identifier.", "operation_not_found", "operation not found"),
			"401": dashboardAuthenticationResponse(),
			"500": errorResponse("The operation store could not be queried.", "operations_unavailable", "operations dashboard is unavailable"),
		},
	}}
}

func operationsProxiesPath() gin.H {
	return operationsJSONPath("List proxy health.", "Returns only proxies with persisted probe results for the selected range. An empty array means there are no configured proxies to show or no probe results; the dashboard hides the proxy section in that case.", operationsRangeParameters(), gin.H{"type": "array", "items": proxySchema()}, []gin.H{{"name": "direct", "healthy": true, "status": "healthy", "observed_at": "2026-07-28T10:00:00Z", "duration_ms": 80, "points": []gin.H{{"observed_at": "2026-07-28T10:00:00Z", "healthy": true, "status": "healthy", "duration_ms": 80}}}})
}

func operationsJSONPath(summary, description string, parameters []gin.H, schema gin.H, example any) gin.H {
	return gin.H{"get": gin.H{
		"summary": summary, "description": description, "tags": []string{"Operations"}, "security": operationsSecurity(), "parameters": parameters,
		"responses": gin.H{
			"200": jsonResponse("Operations data for the selected range.", schema, "operations", example),
			"400": errorResponse("Range, ISO-8601 timestamps, interval, filters, limit, or offset are invalid.", "invalid_operations_query", "range must be one of 24h, 7d, or 30d"),
			"401": dashboardAuthenticationResponse(),
			"500": errorResponse("The operation store could not be queried.", "operations_unavailable", "operations dashboard is unavailable"),
		},
	}}
}

func operationsRangeParameters() []gin.H {
	return []gin.H{
		queryParameter("range", false, "Convenience time range. Defaults to `24h`; allowed values are `24h`, `7d`, and `30d`. Omit it when using `from` and `to`.", enumSchema("24h", "24h", "7d", "30d"), "24h"),
		queryParameter("from", false, "Inclusive ISO-8601/RFC3339 start timestamp. Must be supplied together with `to`; the span cannot exceed 30 days.", gin.H{"type": "string", "format": "date-time"}, "2026-07-27T10:00:00Z"),
		queryParameter("to", false, "Inclusive ISO-8601/RFC3339 end timestamp. Must be supplied together with `from` and be later than it.", gin.H{"type": "string", "format": "date-time"}, "2026-07-28T10:00:00Z"),
	}
}

func operationsSummarySchema() gin.H {
	return gin.H{"type": "object", "required": []string{"active", "succeeded", "failed", "p50_ms", "p95_ms"}, "properties": gin.H{"active": gin.H{"type": "integer", "minimum": 0}, "succeeded": gin.H{"type": "integer", "minimum": 0}, "failed": gin.H{"type": "integer", "minimum": 0}, "p50_ms": gin.H{"type": "integer", "minimum": 0}, "p95_ms": gin.H{"type": "integer", "minimum": 0}}}
}

func operationsBucketSchema() gin.H {
	return gin.H{"type": "object", "required": []string{"started_at", "succeeded", "failed", "p50_ms", "p95_ms"}, "properties": gin.H{"started_at": gin.H{"type": "string", "format": "date-time"}, "succeeded": gin.H{"type": "integer", "minimum": 0}, "failed": gin.H{"type": "integer", "minimum": 0}, "p50_ms": gin.H{"type": "integer", "minimum": 0}, "p95_ms": gin.H{"type": "integer", "minimum": 0}}}
}

func operationSchema() gin.H {
	return gin.H{"type": "object", "required": []string{"id", "type", "status", "started_at", "duration_ms"}, "properties": gin.H{"id": gin.H{"type": "string"}, "type": gin.H{"type": "string", "enum": []string{"search", "extract", "research"}}, "status": gin.H{"type": "string", "enum": []string{"running", "succeeded", "failed"}}, "started_at": gin.H{"type": "string", "format": "date-time"}, "finished_at": gin.H{"type": "string", "format": "date-time"}, "duration_ms": gin.H{"type": "integer", "minimum": 0}, "result": gin.H{"type": "string"}, "http_method": gin.H{"type": "string"}, "http_path": gin.H{"type": "string"}, "http_status": gin.H{"type": "integer"}, "metadata": gin.H{"type": "object", "additionalProperties": gin.H{"type": "string"}}}}
}

func stepSchema() gin.H {
	return gin.H{"type": "object", "properties": gin.H{"id": gin.H{"type": "string"}, "type": gin.H{"type": "string"}, "status": gin.H{"type": "string"}, "duration_ms": gin.H{"type": "integer", "minimum": 0}, "provider": gin.H{"type": "string"}, "backend": gin.H{"type": "string"}, "proxy": gin.H{"type": "string"}}}
}

func operationErrorSchema() gin.H {
	return gin.H{"type": "object", "properties": gin.H{"category": gin.H{"type": "string"}, "message": gin.H{"type": "string"}, "occurred_at": gin.H{"type": "string", "format": "date-time"}}}
}

func proxySchema() gin.H {
	return gin.H{"type": "object", "required": []string{"name", "healthy", "points"}, "properties": gin.H{"name": gin.H{"type": "string"}, "healthy": gin.H{"type": "boolean"}, "status": gin.H{"type": "string"}, "observed_at": gin.H{"type": "string", "format": "date-time"}, "duration_ms": gin.H{"type": "integer", "minimum": 0}, "points": gin.H{"type": "array", "items": gin.H{"type": "object", "properties": gin.H{"observed_at": gin.H{"type": "string", "format": "date-time"}, "healthy": gin.H{"type": "boolean"}, "status": gin.H{"type": "string"}, "duration_ms": gin.H{"type": "integer", "minimum": 0}}}}}}
}
