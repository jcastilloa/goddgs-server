## Why

The current HTML extraction reaches pages through an HTTP client. Pages that require browser execution or reject non-browser traffic, including Cloudflare-protected sites, therefore cannot reliably supply their rendered HTML to extraction and research workflows.

The server needs an optional browser-backed source loader that uses the existing proxy pool while avoiding permanently running Chrome processes.

## What Changes

- Add an optional Chrome HTML extraction provider backed by `chromedp`.
- Select the Chrome provider from configuration for all workflows that need source HTML; retain the existing `goddgs` HTTP extraction path when Chrome is disabled.
- Reuse Chrome instances concurrently per selected proxy while they have work, then terminate idle instances after a configured TTL.
- Route Chrome traffic through the same direct and SSH-backed proxy pool that `goddgs` already uses.
- Add bounded browser and page concurrency, request cancellation, cleanup, health-compatible proxy selection, and browser-specific failure reporting.
- Document Chrome installation and configuration requirements, runtime behavior, proxy interaction, and error responses.

## Capabilities

### New Capabilities

- `chrome-html-extraction`: Optional, proxy-aware retrieval of rendered page HTML through ephemeral reusable Chrome instances.

### Modified Capabilities

- `research-source-selection`: Selected research sources use the configured HTML loader, including Chrome when enabled, before AI content extraction.

## Impact

- Affected packages: `search/application`, `platform/extractai`, new `platform/chrome`, `platform/goddgs`, `platform/config`, `shared/config/domain`, and `cmd/api`.
- Affected behavior: `GET /v1/extract?format=html`, `GET /v1/extract?mode=ai`, and research source extraction can load rendered HTML through Chrome when enabled; their public response schema remains unchanged.
- New runtime dependency: a compatible Chrome/Chromium executable and the Go module `github.com/chromedp/chromedp`.
- Existing API documentation and README require updates for mode-specific configuration and meaningful browser failures.
