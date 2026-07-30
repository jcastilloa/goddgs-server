## 1. Characterize and configure the feature

- [x] 1.1 Add failing configuration tests for disabled defaults, enabled valid Chrome configuration, environment overrides, and every invalid enabled limit.
- [x] 1.2 Add `ChromeConfig` validation in `shared/config/domain`, map it through Viper, and add conservative disabled defaults to `config.sample.yaml` without changing disabled extraction behavior.
- [x] 1.3 Add `chromedp` to the Go module and verify the supported Chrome command-line proxy and executable options against the pinned dependency version.

## 2. Establish the shared extraction and proxy boundaries

- [x] 2.1 Add failing application tests characterizing that only `Format: "html"` uses a configured HTML loader, while every other format continues through the existing `goddgs` gateway.
- [x] 2.2 Introduce the smallest consumer-owned HTML-loader port and compose it into `search/application` without adding `platform` imports to application or domain packages.
- [x] 2.3 Add failing proxy-composition tests proving direct, `tb`, and SSH tunnel entries produce one shared effective-endpoint pool and that Chrome/goddgs selections advance the same health-aware rotation.
- [x] 2.4 Refactor `platform/goddgs.ManagedGateway` to own and expose the shared proxy endpoint selector while retaining one stable `goddgs` client per proxy, existing SSH tunnel ownership, probes, retries, and health transitions.

## 3. Build the Chrome lifecycle manager with TDD

- [x] 3.1 Add failing deterministic manager tests for lazy process creation, reuse of one proxy-scoped browser by concurrent isolated page leases, page-capacity enforcement, and distinct proxy process creation.
- [x] 3.2 Implement the minimal proxy-keyed browser manager with explicit allocator/page-runner seams, lease accounting, and no background resource outside manager ownership.
- [x] 3.3 Add failing deterministic tests for global browser capacity, least-recently-idle eviction of an idle entry only, waiting when every process is busy, and cancellation while waiting.
- [x] 3.4 Implement bounded process capacity and context-aware waiting; ensure active page leases can never be evicted.
- [x] 3.5 Add failing controlled-timer tests for idle expiration, release-after-navigation-error, release-after-cancellation, idempotent close, and shutdown waking all waiters.
- [x] 3.6 Implement idle timer lifecycle and `Close()` so timers, browser processes, and manager-owned cleanup terminate before proxy tunnels are closed.
- [x] 3.7 Run focused manager tests with `-race` after each green/refactor cycle; keep synchronization explicit and avoid sleep-based tests.

## 4. Implement the Chromedp adapter

- [x] 4.1 Add failing adapter tests for executable resolution, Chrome proxy argument construction from the selected shared endpoint, isolated CDP context/target creation, final-URL capture, rendered DOM capture, and cleanup on every return path.
- [x] 4.2 Implement `platform/chrome` with `chromedp`: launch browsers through the lifecycle manager, navigate under the effective request/Chrome deadline, retrieve `document.documentElement.outerHTML`, and return the final URL.
- [x] 4.3 Add failing tests for sanitizing rendered browser HTML before public or AI use, preserving request cancellation, and classifying unavailable browser/proxy, timeout, and navigation failures without exposing sensitive runtime details.
- [x] 4.4 Implement stable error classification and HTTP mapping for Chrome 503/504/502 outcomes while preserving existing 499, HTTP extraction, and non-HTML behavior.
- [x] 4.5 Add focused operation-recording tests and implement sanitized Chrome extraction telemetry with provider `chrome` and proxy key, without recording DOM, browser process output, CDP endpoints, or proxy credentials.

## 5. Compose and preserve extraction workflows

- [x] 5.1 Add failing composition tests proving disabled Chrome launches no browser and enabled Chrome is selected for `format=html` while non-HTML formats keep the `goddgs` client path.
- [x] 5.2 Wire Chrome configuration, shared proxy selector, browser manager, HTML loader, and shutdown ordering in `cmd/api/main.go`; retain an explicit disabled fallback and propagate close errors safely.
- [x] 5.3 Add source/research coverage proving AI extraction and selected research sources enter the configured sanitized HTML path when enabled, share the browser manager's global capacity, honor research cancellation, and retain failed-source omission semantics.
- [x] 5.4 Complete the application/adaptor integration needed for AI extraction and research without bypassing source selection, existing concurrency limits, or final-URL deduplication.
- [x] 5.5 Add a narrow local-browser integration test using only an `httptest` page whose DOM is changed by JavaScript; verify rendered capture and process cleanup, and skip with an explicit reason when no compatible executable is available.

## 6. Document and verify the contract

- [x] 6.1 Update `platform/server/openapi.go` and its contract test to document optional Chrome HTML loading, timeout precedence, no public mode parameter, required configuration, shared-proxy behavior, and browser-specific 502/503/504 responses.
- [x] 6.2 Update `README.md` and `config.sample.yaml` with Chrome/Chromium installation requirements, all `chrome.*` settings and defaults, environment-variable names, resource sizing guidance, proxy limitations, and rollback by disabling Chrome.
- [x] 6.3 Run `gofmt` on changed Go files and execute focused package tests, `go test ./...`, `go test -race ./...`, and `go vet ./...`; investigate and resolve any failures before marking tasks complete.
- [x] 6.4 Add a background startup lookup for Chrome/Chromium that persists an empty `chrome.executable_path` to the active config file without changing `chrome.enabled`; cover configured-path protection, atomic limited persistence, cancellation, and disabled Chrome.
