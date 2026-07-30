# chrome-html-extraction Specification

## Purpose
TBD - created by archiving change add-chrome-html-extraction. Update Purpose after archive.
## Requirements
### Requirement: Opt-in browser-backed HTML loading
The system SHALL obtain `chrome.enabled`, `chrome.executable_path`, `chrome.timeout`, `chrome.max_browsers`, `chrome.max_pages_per_browser`, and `chrome.idle_timeout` through Viper. `chrome.enabled` SHALL default to `false`.

When Chrome is disabled, the system SHALL preserve the existing `goddgs` extraction behavior for every format. When Chrome is enabled, the system SHALL use Chrome to load the rendered source HTML for an extraction whose format is `html`, including the HTML requested by AI extraction and research. It SHALL preserve the existing extraction response schema and SHALL sanitize browser-captured HTML before returning it or supplying it to AI extraction.

When the active configuration file has an empty `chrome.executable_path`, the system SHALL start a background PATH lookup during process startup, regardless of `chrome.enabled`. When it finds a supported Chrome or Chromium executable, it SHALL atomically persist the resolved path in that same active configuration file, without changing `chrome.enabled` or launching a browser. It SHALL NOT replace an explicit executable path.

#### Scenario: Chrome is disabled
- **WHEN** an operator omits `chrome` configuration or sets `chrome.enabled` to `false`
- **THEN** no Chrome process is launched and extraction uses the existing `goddgs` path

#### Scenario: Disabled Chrome discovers an executable
- **WHEN** `chrome.enabled` is `false`, `chrome.executable_path` is empty, and Chrome or Chromium is on PATH
- **THEN** startup writes the resolved executable path to the active `config.yaml` without changing `chrome.enabled` or launching a Chrome process

#### Scenario: Chrome loads rendered public HTML
- **WHEN** `chrome.enabled` is true and a client requests `GET /v1/extract` with a valid URL and `format=html`
- **THEN** the response contains sanitized rendered HTML and the final URL without changing the response shape

#### Scenario: AI extraction uses Chrome source HTML
- **WHEN** Chrome is enabled and a client requests AI extraction or research selects a source URL
- **THEN** the AI extraction source loader receives sanitized rendered HTML from Chrome rather than HTTP-extracted Markdown rendered as HTML

### Requirement: Valid browser configuration
When `chrome.enabled` is true, `chrome.timeout`, `chrome.max_browsers`, `chrome.max_pages_per_browser`, and `chrome.idle_timeout` SHALL each be positive. An invalid enabled configuration SHALL be rejected as invalid server configuration. An empty `chrome.executable_path` SHALL instruct the adapter to discover Chrome or Chromium through PATH.

When Chrome is disabled, omitted Chrome values SHALL retain their defaults and SHALL NOT prevent normal server startup or the existing HTTP extraction path.

#### Scenario: Enabled Chrome has valid limits
- **WHEN** Chrome is enabled with positive timeout, browser/page limits, and idle timeout
- **THEN** the configuration repository exposes those values to the composition root

#### Scenario: Enabled Chrome has an invalid limit
- **WHEN** Chrome is enabled with a zero or negative timeout, browser limit, page limit, or idle timeout
- **THEN** server configuration is rejected before the HTTP listener starts

#### Scenario: Executable is discovered at startup
- **WHEN** the active configuration file has an empty executable path and a compatible browser is on PATH
- **THEN** the background startup lookup persists that path before a later restart, and the adapter can use it immediately if a browser load arrives after discovery

### Requirement: Shared proxy routing and health
Chrome HTML loading SHALL select proxy transport endpoints from the same health-aware, round-robin proxy pool as `goddgs` search and extraction. It SHALL use the selected effective proxy endpoint when starting Chrome, including direct proxy URLs, `tb`'s effective Tor SOCKS URL, and the local SOCKS URL of an existing SSH tunnel.

An endpoint marked unhealthy by the existing pool or SSH tunnel health flow SHALL be unavailable to both `goddgs` and Chrome. The Chrome loader SHALL NOT create a second SSH tunnel or maintain an independent health state.

#### Scenario: Chrome uses an SSH-backed proxy
- **WHEN** the shared pool selects a healthy proxy backed by an SSH tunnel
- **THEN** Chrome starts with that tunnel's existing local SOCKS URL and no additional tunnel is created

#### Scenario: A proxy becomes unhealthy
- **WHEN** the shared proxy pool marks an endpoint unhealthy
- **THEN** neither Chrome nor `goddgs` selects that endpoint for subsequent extraction work

#### Scenario: Proxy selection is rotated across consumers
- **WHEN** `goddgs` and Chrome extraction requests are interleaved with multiple healthy proxies
- **THEN** every selection advances the same round-robin pool rather than maintaining separate rotation state

### Requirement: Bounded ephemeral browser reuse
The system SHALL create Chrome processes lazily, keyed by selected proxy, and SHALL reuse a live process for concurrent extraction requests through the same proxy up to `chrome.max_pages_per_browser`. Each request SHALL run in a distinct browser context and target and SHALL dispose them after its HTML load completes, fails, or is canceled.

The system SHALL retain no more than `chrome.max_browsers` live Chrome processes. When the limit is reached, it SHALL evict only a least-recently-idle process with no active page lease before starting a process for another proxy; otherwise it SHALL wait for capacity while honoring the caller context. It SHALL close an idle process after `chrome.idle_timeout` and SHALL NOT close a process with active page leases.

#### Scenario: Concurrent pages reuse a browser
- **WHEN** two HTML extractions select the same proxy while its browser has available page capacity
- **THEN** they use one Chrome process with separate browser contexts and targets

#### Scenario: Capacity waits respect cancellation
- **WHEN** every eligible browser/page slot is busy and an extraction context is canceled while waiting
- **THEN** the extraction returns cancellation without starting a browser or leaking a capacity lease

#### Scenario: An idle browser is expired
- **WHEN** a Chrome process has no active page lease for at least `chrome.idle_timeout`
- **THEN** the manager terminates the process and removes it from reusable capacity

#### Scenario: Shutdown closes browser resources
- **WHEN** the server begins shutdown
- **THEN** the browser manager stops idle timers, terminates its Chrome processes, and completes before the SSH tunnels it uses are closed

### Requirement: Browser timeouts and failures are meaningful
The Chrome adapter SHALL apply `chrome.timeout` as the per-page navigation and DOM-capture budget while preserving any earlier caller deadline. It SHALL preserve context cancellation. It SHALL map an unavailable browser process, closed manager, or no healthy shared proxy to the documented 503 extraction response; page deadline expiration to 504; and navigation or CDP failures to 502. Error responses and operational telemetry SHALL NOT disclose executable paths, proxy credentials or URLs, CDP addresses, profile data, raw DOM, or browser process output.

The runtime OpenAPI document, its extraction contract test, sample configuration, and README SHALL document Chrome's opt-in behavior, required executable, each setting and default, shared-proxy behavior, resource limits, timeout behavior, and meaningful 502/503/504 outcomes.

#### Scenario: Chrome executable is unavailable
- **WHEN** Chrome is enabled but the configured or discovered executable cannot be launched
- **THEN** an HTML extraction returns the documented 503 response and non-HTML `goddgs` extraction remains available

#### Scenario: Browser navigation times out
- **WHEN** navigation or DOM capture does not finish before the effective Chrome page deadline
- **THEN** the endpoint returns the documented 504 response and releases browser/page resources

#### Scenario: Browser navigation fails
- **WHEN** Chrome starts but the selected source page cannot be rendered
- **THEN** the endpoint returns the documented 502 response with a sanitized error and releases browser/page resources
