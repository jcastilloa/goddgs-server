## Context

`goddgs-server` currently builds one stable `goddgs` client for every configured proxy and rotates requests through a health-aware pool. `goddgs.Extract` is HTTP-based: it cannot execute JavaScript and some sites reject its transport even when its headers resemble a browser. The server uses `format=html` both as the public HTML extraction result and as the source HTML passed to AI extraction and research.

The new loader must reach the same direct proxies and SSH tunnel endpoints as `goddgs`, honor the existing health state and rotation, work during the existing concurrent research extraction stage, and not leave Chrome processes resident indefinitely. The public extraction request and response schema remain unchanged.

## Goals / Non-Goals

**Goals:**

- Make rendered DOM HTML available through `chromedp` when `chrome.enabled` is true.
- Share one proxy-selection and health pool between `goddgs` and Chrome, including effective local SOCKS URLs for SSH tunnels and the `tb` alias.
- Lazily create a Chrome process for a proxy, allow bounded concurrent isolated browser contexts in it, and terminate it after a bounded idle interval.
- Preserve the existing `goddgs` path exactly when Chrome is disabled and preserve the existing public response shape in either mode.
- Make cancellation, timeouts, process cleanup, configuration failures, and browser navigation failures observable and testable.

**Non-Goals:**

- Guaranteeing access through Cloudflare, CAPTCHAs, paywalls, IP-reputation controls, or other anti-bot systems.
- Browser automation beyond retrieving the rendered HTML: no login flows, CAPTCHA solving, click scripting, profile persistence, arbitrary waits, screenshots, or user-provided browser flags.
- Adding a public `mode=chrome` parameter, changing non-HTML extraction formats, or adding a persistence/SQL schema.
- Altering the endpoint's existing URL-validation policy as part of this change.

## Decisions

### 1. Chrome is a server adapter selected for HTML, not a `goddgs` feature

The implementation adds `github.com/chromedp/chromedp` only to `goddgs-server`. It does not modify or fork `goddgs`: that library deliberately owns a source-compatible HTTP extraction transport and does not own Chrome executable discovery, process lifetime, deployment configuration, proxy-pool policy, or server telemetry.

`search/application` gains the smallest consumer-owned HTML-loader port. Its extraction service routes only `ExtractRequest{Format: "html"}` to that port when configured; all other formats retain the current gateway route. The `platform/chrome` adapter implements the port, navigates to the URL, reads the final URL and rendered document HTML, and applies the existing HTML sanitization before returning it. `platform/extractai.Source` already requests `Format: "html"`, so AI extraction and research automatically share the selected loader without duplicating logic.

This keeps dependencies inward: the application sees a narrow port and result value, while the Chrome package owns CDP and process I/O. Adding a Chrome option directly to `goddgs.Extract` would make a reusable HTTP library depend on deployment-specific browser behavior and would not solve server-side proxy lifecycle ownership.

### 2. One shared proxy pool is the authority for `goddgs` and Chrome

The proxy composition currently produces one stable client per configured proxy inside `platform/goddgs.ManagedGateway`. It will be refactored minimally so that it first creates one shared health-aware pool of proxy transport endpoints:

```text
configured direct / SSH proxy
        -> effective transport URL
        -> shared pool entry { key, transport URL }
              |                    |
              |                    +-> Chrome browser manager
              +-> stable goddgs client selected by the same lease key
```

`ManagedGateway` remains the owner of SSH tunnels, probes, and proxy health transitions. Both consumers select a lease from the same round-robin pool; an unhealthy tunnel or probe result therefore removes the same entry from both paths. The stable `goddgs` clients remain mapped by lease key, so search and non-Chrome formats do not gain per-request client construction.

The Chrome adapter receives the shared selector as an explicit constructor dependency. It never parses proxy configuration itself and never starts a second SSH tunnel. A direct connection is represented by an empty proxy URL; proxied Chrome processes receive the selected effective URL through `chromedp.ProxyServer` / Chrome's `--proxy-server` switch.

Separate pools were rejected because they would silently diverge in round-robin position and risk health state. A Chrome-specific tunnel manager was rejected because it duplicates connection ownership and lifecycle.

### 3. Lazily pooled browsers are scoped by proxy and have bounded page leases

A `platform/chrome` manager owns a map keyed by proxy lease key. A browser entry contains its allocator/process cancellation function, its effective proxy URL, active page count, last-idle time, and idle-close timer.

On an HTML load, the adapter selects a healthy proxy lease, then acquires a page lease from the manager for that key:

- If a live browser for that proxy has fewer than `max_pages_per_browser` active leases, reuse it.
- Otherwise create one lazily if the global `max_browsers` limit permits it. At most one browser entry exists per proxy key.
- If the selected proxy has no capacity and no browser can be created, wait for capacity or cancellation; when the limit is full, evict the least-recently-idle entry with no active pages before creating a browser for a new proxy. It MUST NOT terminate a browser with active leases.
- Each lease creates a new CDP browser context and target, performs navigation and DOM capture with the caller context, then disposes the target/context and releases its manager capacity in `defer` paths.
- When an entry's last page lease is released, start/restart its idle timer. The timer removes and cancels that entry only if it remains idle for `idle_timeout`.

This gives concurrent requests a process and startup-cost reuse boundary without persistent browser daemons. Isolated browser contexts prevent cookies, local storage, and page state from leaking between requests. Recreating a process on every request was rejected because research extraction can issue concurrent page loads and would impose repeated startup cost. A permanent allocator per proxy was rejected because idle proxies would retain Chrome processes and make failure recovery and resource use harder to control.

The manager exposes `Close()`. Application shutdown marks it closed, stops timers, cancels all allocators, wakes capacity waiters, and waits for manager-owned cleanup to finish before SSH tunnels are closed. It accepts injected clock/timer and allocator seams only where needed to test lease accounting and cleanup without launching real Chrome.

### 4. Configuration is opt-in and has explicit resource bounds

`ServerConfig` gains:

```yaml
chrome:
  enabled: false
  executable_path: ""       # Chrome/Chromium discovered from PATH at startup when empty
  timeout: 45s              # full navigation and DOM-capture budget per page
  max_browsers: 2           # processes across all proxy keys
  max_pages_per_browser: 3  # simultaneous isolated page contexts per process
  idle_timeout: 1m          # close an unused process after this interval
```

Viper also supports the corresponding `CHROME_*` environment variables. At startup, when the active configuration file has an empty `chrome.executable_path`, a background task searches the conventional Chrome/Chromium executable names on `PATH`. A successful lookup atomically writes that one resolved path back to the active `config.yaml`, without changing `chrome.enabled` or starting a browser; an explicit configured path is never replaced. When disabled, omitted Chrome settings have no behavioral effect and invalid optional values do not prevent the existing HTTP extractor from running. When enabled, timeout and all limits must be positive, `idle_timeout` must be positive, and the configured executable path must be non-empty only if the operator intentionally chooses to override PATH discovery. A missing/unlaunchable discovered executable remains a runtime browser-unavailable error rather than a startup failure, so an operator can start the server before an executable becomes available but receives a meaningful request response.

`timeout` is applied as a child deadline to the request context, never as a background browser lifetime. The existing `service.request_timeout` continues to bound heuristic `GET /extract`; Chrome therefore uses the shorter of that HTTP deadline and `chrome.timeout`. AI extraction remains exempt from `service.request_timeout` as today, but each Chrome source load is bounded by `chrome.timeout` and research remains bounded by `research.timeout`.

### 5. Browser failures receive stable error mapping and telemetry

The Chrome adapter introduces classified errors that preserve their wrapped causes:

| Condition | HTTP result | Meaning |
| --- | --- | --- |
| Chrome disabled/unavailable, manager closed, or no healthy shared proxy | 503 | HTML browser extraction cannot start now. |
| Caller or Chrome page deadline expires | 504 | Page navigation/rendering did not complete in time. |
| Navigation, CDP, or page-load failure | 502 | The source page could not be rendered through the selected browser/proxy. |
| Caller cancellation | 499 | Existing cancellation behavior. |

The existing `writeSearchError` mapping recognizes the new unavailable classification without exposing executable paths, proxy credentials, CDP addresses, or raw browser stderr. The source recorder's existing extraction step records provider `chrome` and selected proxy key for browser loads; it retains the existing sanitized URL metadata. The regular `goddgs` extraction path remains identified as before. No raw DOM, browser profile data, proxy URL, or process output is persisted.

### 6. Test the manager as a deterministic state machine, then test the adapter boundary

The TDD sequence starts with failing unit tests around configuration validation, shared-pool selection, browser lease capacity, same-proxy concurrent reuse, cross-proxy eviction, idle close, cancellation while waiting, and idempotent shutdown. These tests use a fake allocator/page runner, controlled clock/timer, and synchronization channels; they do not sleep or launch Chrome.

After the manager is green, adapter tests verify proxy argument construction, final URL/DOM forwarding, sanitization, classified errors, and page/context cleanup with a fake CDP runner. Composition and handler tests then characterize disabled fallback and enabled HTML routing. A narrow integration test is conditional on a local executable: it uses a local `httptest` page only, confirms JavaScript-mutated DOM capture and process cleanup, and is skipped with an explicit reason when Chrome is unavailable. Concurrent code is verified with `go test -race ./...`.

## Risks / Trade-offs

- [Cloudflare or another anti-bot service still rejects headless Chrome] → Document Chrome as an optional rendering transport, not a guaranteed bypass; return a sanitized upstream failure and retain the HTTP fallback when operators disable it.
- [Chrome costs memory/CPU and research creates many extractions] → Require global process and per-browser page caps; make capacity waits context-aware; expose idle termination and recommend tuning below host capacity.
- [A stale browser or timer leaks after cancellation] → Centralize ownership in the manager, release every page through `defer`, test shutdown/idle/cancellation paths, and run race tests.
- [Proxy health or rotation diverges from `goddgs`] → Share the exact pool object and effective transport endpoint rather than copying configuration into Chrome.
- [HTTP proxy authentication is not universally handled by the command-line proxy switch] → Support the existing unauthenticated direct, SOCKS, Tor, and SSH tunnel endpoint forms initially; document authenticated proxy challenge handling as an explicit future extension rather than silently logging credentials or claiming support.
- [Rendered DOM contains scripts or unsafe markup] → Continue using the existing sanitizer before public or AI use; browser execution does not bypass output sanitization.
- [A browser process cannot start on a deployment target] → Keep the feature opt-in; return the classified 503 error, document Chrome/Chromium installation, and avoid failing the whole server solely for a missing executable.

## Migration Plan

1. Deploy the code with `chrome.enabled: false`; this preserves existing extraction behavior and creates no Chrome process.
2. Install a compatible Chrome/Chromium executable on the hosts selected for browser extraction. Set `chrome.executable_path` only when PATH discovery is insufficient.
3. Enable Chrome with conservative bounds (`max_browsers: 2`, `max_pages_per_browser: 3`, `idle_timeout: 1m`) and observe browser extraction outcomes and resource use, particularly during research.
4. Tune bounds to host capacity and proxy availability. Disable `chrome.enabled` to return immediately to the current `goddgs` HTML path.
5. Roll back by restoring the preceding binary/configuration. No endpoint migration or database migration is required.

## Open Questions

- No decision is required to implement the initial feature. Authenticated HTTP proxy challenge support is deliberately outside its scope and can be proposed separately if the configured proxy fleet requires it.
