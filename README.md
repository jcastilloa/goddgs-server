![goddgs-server header](assets/header.png)

# goddgs-server

HTTP REST server for [goddgs](https://github.com/jcastilloa/goddgs). It keeps one stable `goddgs` client per proxy and applies per-request rotation.

## Table of contents

- [Run](#run)
  - [Requirements](#requirements)
  - [Installation](#installation)
- [API](#api)
  - [Search parameters](#search-parameters)
  - [Interactive API documentation](#interactive-api-documentation)
  - [Find a news article and extract its body](#find-a-news-article-and-extract-its-body)
  - [AI extraction mode](#ai-extraction-mode)
  - [Research](#research)
  - [Authentication](#authentication)
- [Proxies](#proxies)
  - [Choose a configuration](#choose-a-configuration)
  - [No proxy: direct host connection](#no-proxy-direct-host-connection)
  - [Existing direct proxy](#existing-direct-proxy)
  - [SSH tunnel](#ssh-tunnel)
  - [Mixed rotating pool](#mixed-rotating-pool)
- [Operational storage](#operational-storage)
- [Operations dashboard](#operations-dashboard)
- [Verification](#verification)

## Run

```sh
cp config.sample.yaml config.yaml
go run ./cmd/api
```

Configuration is loaded from `./config.yaml` or `~/.config/goddgs-server/config.yaml`. Environment variables follow Viper's naming convention; for example, `SERVICE_PORT=8081`.

### Requirements

- Go **1.26.1+** to build from source.
- A `config.yaml` with at least one proxy entry to run the server.

### Installation

#### Build from source

```sh
make build
./bin/goddgs-server
```

To install the binary in `~/.local/bin`:

```sh
make install
```

`make release VERSION=vX.Y.Z` creates compressed artifacts and SHA-256 checksums in `./dist` for Linux (`amd64`, `arm64`), macOS (`amd64`, `arm64`), and Windows (`amd64`). Each archive includes the binary, this README, and `config.sample.yaml`.

#### Prebuilt binary

Linux and macOS:

```sh
# Latest release
curl -fsSL https://raw.githubusercontent.com/jcastilloa/goddgs-server/master/scripts/install.sh | sh

# Specific version
curl -fsSL https://raw.githubusercontent.com/jcastilloa/goddgs-server/master/scripts/install.sh | VERSION=vX.Y.Z sh
```

Windows PowerShell:

```powershell
# Latest release
iwr https://raw.githubusercontent.com/jcastilloa/goddgs-server/master/scripts/install.ps1 -UseBasicParsing | iex

# Specific version
$env:VERSION='vX.Y.Z'; iwr https://raw.githubusercontent.com/jcastilloa/goddgs-server/master/scripts/install.ps1 -UseBasicParsing | iex
```

The installers place the binary and `config.sample.yaml` in `~/.local/bin` (Linux/macOS) or `%LOCALAPPDATA%\goddgs-server\bin` (Windows). Copy that sample to `config.yaml` and configure its proxy before starting the server.

Installer environment variables:

| Variable | Default |
| --- | --- |
| `REPO` | `jcastilloa/goddgs-server` |
| `SERVICE_NAME` | `goddgs-server` |
| `INSTALL_DIR` | `~/.local/bin` (Linux/macOS) · `%LOCALAPPDATA%\goddgs-server\bin` (Windows) |
| `VERSION` | Latest GitHub Release tag |

GitHub publishes these artifacts automatically whenever a `v*` tag is pushed. The tag is embedded in the binary and used by `GET /v1/version` unless `service.version` overrides it in configuration.

## Operational storage

The server requires a local SQLite database for operational records. Unless overridden, it creates `operations.sqlite` beside the executable before opening the HTTP listener. SQLite may also create adjacent `operations.sqlite-wal` and `operations.sqlite-shm` files while the server runs; keep all three files together.

The executable directory must be writable. If it is read-only (for example, a system install directory), configure an explicit writable database path with `operations.database_path` or `OPERATIONS_DATABASE_PATH`. Relative paths are resolved from the process working directory; use an absolute path for deployments.

```yaml
operations:
  # Empty uses operations.sqlite beside the resolved executable.
  database_path: /var/lib/goddgs-server/operations.sqlite
  # Completed records and proxy health data are kept for 30 days by default.
  retention: 720h
  dashboard_auth:
    # Dashboard sessions last 12 hours unless configured otherwise.
    session_ttl: 12h
    # Enable when the browser reaches the service over HTTPS.
    cookie_secure: true
```

The store uses SQLite WAL mode, foreign keys, bounded lock waits, transactional migrations, and hourly retention cleanup. Initialization or cleanup failures stop startup rather than running without persistent operational data.

### Proxy health probes

Active proxy probes are disabled by default so deployments never send unexpected outbound traffic. To enable them, configure an explicit, stable HTTP(S) URL owned or selected by the operator. Each configured proxy, including an SSH-backed proxy through its local SOCKS tunnel, sends a minimal `GET` request to this URL at the configured interval. Redirects (2xx/3xx) are successful; 4xx, 5xx, transport errors, and timeouts are failures. Redirects are not followed.

```yaml
operations:
  probe:
    enabled: true
    url: https://status.example.net/proxy-probe
    interval: 1m
    timeout: 10s
    success_threshold: 2
    failure_threshold: 3
```

Every round records timestamp, latency, HTTP status when available, outcome, and error category in SQLite. Proxies begin in `unknown`; consecutive successes promote them to `healthy`, a failure below the configured failure threshold makes them `degraded`, and the threshold makes them `unhealthy`. `healthy` and `degraded` proxies remain eligible for the pool. Each status change is persisted once. An SSH tunnel disconnection immediately makes its proxy `unhealthy`; after reconnect it returns to `unknown` and remains unavailable until the configured number of successful probes confirms recovery.

## Operations dashboard

`GET /operations` serves an embedded dashboard from the same process and HTTP address as the API. It reads the SQLite operational store and refreshes every five seconds without reloading the page. The dashboard uses Tailwind CSS and Chart.js directly from their CDNs: no Node.js, npm, bundled assets, or frontend build step is required.

![Operations dashboard](assets/operations-dashboard.png)

It displays active, successful, and failed operations; p50/p95 latency; volume and latency charts; recent operations with expandable sanitized detail; and proxy health, availability history, and latency when probe records exist. Use the 24-hour, 7-day, or 30-day selector. Empty operation data produces empty states; the proxy section is hidden when no proxy probe results are available.

### Local dashboard access

The dashboard has one local administrator account stored in the operational SQLite database. On the first visit, `GET /operations` redirects to `/operations/setup`; the first operator creates the username and password. Later anonymous visits redirect to `/operations/login`. Usernames are limited to 3–64 ASCII letters, digits, `.`, `_`, or `-`; passwords must be 12–128 characters. Passwords are stored only as Argon2id hashes.

Successful setup or login creates an opaque, server-side session in an `operations_session` cookie. It is `HttpOnly`, `SameSite=Strict`, scoped to `/operations`, and lasts `operations.dashboard_auth.session_ttl` (12 hours by default). The dashboard also receives an `operations_csrf` cookie and sends it in `X-Operations-CSRF` for logout and password changes. Use the account badge in the top-right corner to change the password or sign out. Changing the password revokes all existing dashboard sessions and immediately creates a replacement session for the current browser.

Set `operations.dashboard_auth.cookie_secure: true` (or `OPERATIONS_DASHBOARD_AUTH_COOKIE_SECURE=true`) whenever users access the dashboard through HTTPS. It is `false` by default only to support local HTTP development; a `Secure` cookie cannot be set over plain HTTP. Treat the first setup as a deployment-sensitive step: start the service on a trusted network until the initial account is created.

The dashboard session is independent of `auth.token`: a bearer token never grants access to `/operations` or `/operations/api/*`. Conversely, a dashboard session does not authorize `/v1`, `/openapi.json`, or `/docs/` when `auth.token` is enabled.

### Dashboard JSON API

All dashboard JSON endpoints require the `operations_session` cookie; unauthenticated requests return `401 {"error":"dashboard authentication required"}`. The full cookie, CSRF, validation, and error contracts are documented in `/openapi.json`:

| Endpoint | Method | Purpose |
| --- | --- | --- |
| `/operations/api/auth/setup` | `POST` | Create the initial account and session; returns `409 setup_completed` once configured. |
| `/operations/api/auth/login` | `POST` | Create a session with username/password. |
| `/operations/api/auth/session` | `GET` | Return the current username. |
| `/operations/api/auth/logout` | `POST` | Revoke the current session; requires `X-Operations-CSRF`. |
| `/operations/api/auth/password` | `POST` | Change the password and revoke prior sessions; requires `X-Operations-CSRF`. |

| Endpoint | Method | Purpose |
| --- | --- |
| `/operations/api/summary` | `GET` | Counts and p50/p95 latency for a range. |
| `/operations/api/timeseries` | `GET` | Success/error volume and p50/p95 buckets. |
| `/operations/api/operations` | `GET` | Sanitized, paginated operation list. |
| `/operations/api/operations/{id}` | `GET` | Sanitized operation, steps, and errors. |
| `/operations/api/proxies` | `GET` | Probe-based proxy state, latency, and history. |

The summary, series, list, and proxy endpoints accept `range=24h`, `range=7d`, or `range=30d` (default `24h`), or an explicit `from` and `to` RFC3339 pair no wider than 30 days. The operation-detail endpoint accepts only its path `id`. The list accepts `status`, `type`, `limit` (1–100, default 50), and `offset` (0–10000). The series accepts `interval=1h`, `6h`, or `24h`, with a safe default selected from the range. Invalid dates, ranges, filters, intervals, limits, or IDs return `400`; a missing operation returns `404`; storage failures return `500`.

`auth.token` continues to protect the versioned API, `/openapi.json`, and `/docs/`. It intentionally does **not** substitute for the separate dashboard session. The dashboard only exposes the already-sanitized operational data, never request bodies, provider responses, prompts, credentials, or unredacted URLs.

## API

The metasearch routes use `GET` and the `service.api_prefix` prefix (default: `/v1`). Research uses `POST`.

| Endpoint | Method |
| --- | --- |
| `/v1/text` | `GET` |
| `/v1/images` | `GET` |
| `/v1/news` | `GET` |
| `/v1/videos` | `GET` |
| `/v1/books` | `GET` |
| `/v1/extract` | `GET` |
| `/v1/research` | `POST` |

Search results are returned without narrowing `goddgs` types: numbers, nested maps, and null values are preserved. Documentation is served at `/docs/`, and the OpenAPI specification at `/openapi.json`.

### Search parameters

The text, image, news, and video endpoints require either `q` or `query` (`q` wins when both are sent). Their defaults are:

- `region=us-en`
- `safesearch=moderate`
- `max_results=10`
- `page=1`
- `backend=auto`
- `timelimit` is optional.

Additional parameters per endpoint:

- **Images**: `size`, `color`, `type_image`, `layout`, and `license_image`.
- **Videos**: `resolution`, `duration`, and `license_videos`.
- **Books**: only `query`, `max_results`, `page`, and `backend`.
- **`extract`**: `url`, `format`, and `mode`.

### Interactive API documentation

With the default configuration, open Swagger UI at:

```text
http://localhost:8080/docs/
```

For a server listening on `10.9.0.1:8097`, the URL is:

```text
http://10.9.0.1:8097/docs/
```

Swagger UI loads the dynamically generated OpenAPI document from `/openapi.json`. You can retrieve that document directly, for code generation or another OpenAPI client:

```text
http://10.9.0.1:8097/openapi.json
```

When `auth.token` is configured, the documentation and specification also require authentication. In Swagger UI, click **Authorize**, enter only the token value (without `Bearer `), then execute requests. From the command line:

```sh
curl -H 'Authorization: Bearer <token>' http://10.9.0.1:8097/openapi.json
```

### Find a news article and extract its body

`/v1/news` finds articles; it does not download their complete bodies. Use the `url` returned by a news result with `/v1/extract`. This keeps each API route mapped to one `goddgs` operation.

```sh
ARTICLE_URL="$(
  curl -sG 'http://10.9.0.1:8097/v1/news' \
    --data-urlencode 'q=Go programming' \
    --data-urlencode 'max_results=5' |
  jq -r '.[0].url'
)"

curl -sG 'http://10.9.0.1:8097/v1/extract' \
  --data-urlencode "url=$ARTICLE_URL" \
  --data-urlencode 'format=text_plain' |
jq -r '.Content'
```

Use `text_plain` for clean, readable text. `text_markdown` is the default, `text_rich` retains richer structure, and `text` returns the source response text. Paywalls, JavaScript-only pages, or publisher blocking can prevent a complete extraction.

### AI extraction mode

`/v1/extract` defaults to `mode=heuristic`, which preserves the existing `goddgs` extraction behavior. Use `format=html` to render its extracted Markdown as a sanitized HTML fragment; it is the convenient HTML response for clients. `format=content` instead returns the unprocessed source document and appears Base64-encoded in JSON because it is binary content.

Set `mode=ai` to send the same sanitized HTML produced by heuristic `format=html` to a configured OpenAI-compatible LLM for primary-content selection. The model never receives the full source document, including scripts and page chrome. In AI mode, `format` is ignored because the response is always clean HTML. The server removes scripts, event handlers, embedded content, forms, presentation attributes, and unsafe URLs from the model output. It retains only `href` on links and `src`/`alt` on images, without resolving or rewriting URLs. If no editorial content is found, `Content` is an empty string.

```sh
curl -sG 'http://localhost:8080/v1/extract' \
  --data-urlencode 'url=https://example.com/article' \
  --data-urlencode 'format=html' |
jq -r '.Content'
```

AI mode is enabled only when the complete `llm` and `extract_ai` configuration is usable. Otherwise heuristic mode remains fully available and AI requests return `503` with the exact settings required. It is not constrained by `service.request_timeout`: extracting Markdown through goddgs uses that timeout, and then the LLM call uses `extract_ai.timeout`.

```yaml
llm:
  # Any OpenAI-compatible API base URL. It must expose POST /chat/completions.
  base_url: https://your-llm-provider.example.com/v1
  api_key: ""
  # Optional provider-specific headers, for example HTTP-Referer or X-Title.
  headers: {}

extract_ai:
  model: gpt-4.1-mini
  timeout: 45s
  temperature: 0.1
  retries: 2
```

Set `llm.api_key` or the equivalent `LLM_API_KEY` environment variable to enable the mode. `llm.base_url`, `extract_ai.model`, `extract_ai.timeout`, `extract_ai.temperature`, and `extract_ai.retries` are also required when AI mode is enabled.

The AI instructions are deliberately narrow: source HTML is treated as untrusted data; the model must preserve the original language, return only the main editorial content in HTML, and exclude navigation, sidebars, ads, cookie notices, related links, subscriptions, and other chrome. The final sanitization policy is enforced by Go rather than delegated to the model.

### Research

`POST /v1/research` turns a generic topic into search queries, searches through goddgs, sends only each result's server-assigned ID, title, description, and URL to a selection LLM, extracts only the approved URLs with AI extraction, and produces a sanitized HTML report with the sources it actually used. The selector never receives page HTML, and results it rejects are never crawled or sent to AI extraction. The source list has the shape `[{"url":"…","title":"…"}]`; both LLM stages select only IDs supplied by the server, so they cannot add arbitrary URLs.

```sh
curl -sS -X POST 'http://localhost:8080/v1/research' \
  -H 'Content-Type: application/json' \
  -d '{
    "query": "When was E.T. released and what was its opening box office?",
    "report_language": "en",
    "query_languages": ["en", "es"],
    "query_count": 10,
    "results_per_query": 10
  }' | jq .
```

#### Parameters

- `query` — **required**.
- `report_language` — an ISO 639-1 code; defaults to `en`.
- `query_languages` — controls the languages in which the query LLM creates search terms; defaults to `["en"]`.
- `query_count` — the total number of generated queries, divided across those languages; defaults to `10`.
- `results_per_query` — defaults to `10`.

Set `region` to use one goddgs region for every query. If omitted, it is derived separately from the language of each generated query: `en` uses `us-en` and `es` uses `es-es`. Other query languages require an explicit `region`.

#### Behavior

Research discovers up to `query_count × results_per_query` valid unique URLs. It assigns stable candidate IDs and sends each ID, title, description when available, and URL for the first `research.max_selection_candidates` discovered candidates to `selection_ai`; this model returns an ordered shortlist of at most `research.max_selected_sources` candidate IDs. Candidates outside either selection boundary are not crawled or submitted to AI extraction. Any approved page that fails extraction, returns empty content, or duplicates another final URL is silently ignored — it is neither included in the report nor listed as a source. If selection fails or returns an invalid shortlist, or if no usable source remains, the endpoint returns `502`.

The selector is instructed to prefer relevance to the research topic, useful coverage across facets, diverse sources, and authoritative or primary sources when the result metadata supports those judgments. This is a prefilter, not proof that an approved page is factual or extractable.

#### Diagnostics

Every successful response includes `diagnostics`. `diagnostics.backends` aggregates the actual completed goddgs backend attempts across generated-query searches: `name`, scheduler `provider`, `attempts`, `result_count`, and `error_count`. The duration fields are elapsed milliseconds:

- `query_planning_ms`
- `search_ms`
- `source_selection_ms`
- `source_extraction_ms` (the parallel AI source-extraction stage for selection-approved URLs only)
- `report_generation_ms`
- `total_ms`
- `candidates_found` (valid, URL-deduplicated discovered results)
- `candidates_selected` (validated URLs approved before extraction)

Backend diagnostics do not cover source-page downloads.

#### Configuration

Research needs the existing `llm` and `extract_ai` settings plus a global research timeout, independent source-selection and extraction budgets, and separate LLM settings for query planning, source selection, and report writing:

```yaml
research:
  # Independent from service.request_timeout; must cover the entire workflow.
  timeout: 10m
  # Maximum normalized search-result metadata candidates sent to selection_ai.
  max_selection_candidates: 100
  # Maximum selection_ai-approved URLs that can be downloaded and extracted.
  max_selected_sources: 20
  # Maximum source pages extracted through AI at the same time.
  max_concurrent_extractions: 20
  query_ai:
    model: gpt-4.1-mini
    timeout: 30s
    temperature: 0.1
    retries: 2
  selection_ai:
    model: gpt-4.1-mini
    timeout: 30s
    temperature: 0.1
    retries: 2
  report_ai:
    model: gpt-4.1-mini
    timeout: 60s
    temperature: 0.1
    retries: 2
```

The query, selection, and report models each use their own model, temperature, timeout, and retry policy while sharing `llm.base_url`, `llm.api_key`, and `llm.headers`. `selection_ai` receives only the server-assigned candidate ID, title, description, and URL metadata. `research.max_selection_candidates` limits selector input, `research.max_selected_sources` limits source-page downloads, and `research.max_concurrent_extractions` limits concurrent extraction requests; all must be positive. `extract_ai` controls the AI extraction call for each selection-approved source. If any required setting is missing, research returns `503`; ordinary search and heuristic extraction continue to work.

### Authentication

If `auth.token` is not empty, every versioned API route plus `/openapi.json` and `/docs/` requires:

```text
Authorization: Bearer <token>
```

`/operations` and `/operations/api/*` use their own local cookie session; see [Operations dashboard](#operations-dashboard).

## Proxies

`proxies` is required and must contain at least one uniquely named entry. Each entry creates one persistent `goddgs` client, so its outbound route and browser identity remain consistent for the lifetime of the process. The server selects healthy entries round-robin. A transport failure can retry another entry for that request, but does not permanently disable a direct proxy; rate limits do not force rotation. SSH tunnel health is managed by its reconnect supervisor.

### Choose a configuration

| Situation | Entry type | What to configure |
| --- | --- | --- |
| No proxy | `direct` | Omit `url`. This is the normal local-development configuration. |
| One existing HTTP(S) or SOCKS proxy | `direct` | Set `url` to that proxy URL. |
| Tor Browser | `direct` | Set `url: tb`; it resolves to `socks5h://127.0.0.1:9150`. |
| One SSH egress host | `ssh` | Set host, user, private-key path, and verified host key. |
| Rotation | mixed | Configure two or more `direct` and/or `ssh` entries. |

### No proxy: direct host connection

Use this when the server can access the Internet itself. `url` must be omitted; do not set it to a placeholder such as `127.0.0.1:9050` unless a SOCKS server actually listens there.

```yaml
service:
  host: 0.0.0.0
  port: 8080
  api_prefix: /v1
  request_timeout: 30s
  max_proxy_retries: 1

auth:
  token: ""

proxies:
  - name: direct
    type: direct
```

The underlying library honors `DDGS_PROXY` when `url` is omitted. Leave `DDGS_PROXY` unset for a true direct connection:

```sh
unset DDGS_PROXY
```

### Existing direct proxy

`url` accepts `http://`, `https://`, `socks5://`, `socks5h://`, and `tb`. Use `socks5h` when DNS resolution must happen at the proxy rather than on this host.

```yaml
proxies:
  - name: company-connect
    type: direct
    url: http://proxy.example.net:3128

  - name: remote-dns-socks
    type: direct
    url: socks5h://proxy.example.net:1080
```

`tb` is a shortcut for the Tor Browser SOCKS listener at `127.0.0.1:9150`; it is not the usual Tor daemon port `9050`.

```yaml
proxies:
  - name: tor-browser
    type: direct
    url: tb
```

### SSH tunnel

For an `ssh` entry, the server creates and supervises its own loopback SOCKS5H listener. Do not configure the local SOCKS port: the operating system assigns it and the server keeps it stable while the SSH connection reconnects. The destination hostname is resolved on the SSH host.

```yaml
proxies:
  - name: madrid-egress
    type: ssh
    host: proxy.example.net
    port: 22 # Optional; defaults to 22.
    user: deploy
    private_key_path: /run/secrets/proxy_ed25519
    host_key: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... proxy.example.net"
```

`host_key` is mandatory and is verified with `ssh.FixedHostKey`; `InsecureIgnoreHostKey` is never used. The private key path is read by the server process, so it must be readable by that process and should not be committed.

### Mixed rotating pool

The following configuration rotates requests between a direct connection, an existing SOCKS proxy, and an SSH egress host. Give each entry a unique `name`.

```yaml
proxies:
  - name: direct
    type: direct

  - name: socks-eu
    type: direct
    url: socks5h://socks.example.net:1080

  - name: madrid-egress
    type: ssh
    host: proxy.example.net
    port: 22
    user: deploy
    private_key_path: /run/secrets/proxy_ed25519
    host_key: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... proxy.example.net"
```

The server tries the next healthy entry after a transport failure, up to `service.max_proxy_retries`. A direct entry remains eligible for future requests after a transient transport failure. SSH entries become ineligible only while their reconnect supervisor reports them unavailable. When no healthy entry is available for a request, the API returns `503` with `no healthy upstream connection available`. Failed requests are logged with the HTTP method, path, status, and underlying cause.

## Verification

```sh
go test ./...
go test -race ./...
go vet ./...
```
