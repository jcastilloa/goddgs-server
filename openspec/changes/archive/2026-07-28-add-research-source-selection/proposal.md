## Why

Research currently sends every unique URL returned by goddgs to AI extraction. Search-result quality is uneven, so irrelevant or weak pages consume crawling, extraction time, and LLM capacity while reports may still receive too few useful sources.

## What Changes

- Add an LLM selection stage between goddgs discovery and source extraction.
- Send the selector only normalized search-result metadata: stable candidate ID, title, description when available, and URL. It must never receive page HTML or trigger page downloads.
- Extract and report only the server-validated candidate IDs selected by that stage; discarded candidates must not be crawled or passed to AI extraction.
- Add independent `research.selection_ai` configuration for model, timeout, temperature, and retries, parallel to `query_ai` and `report_ai`.
- Add configuration-owned limits for the candidate list submitted to selection and the approved URLs permitted to reach extraction.
- Record and return source-selection timing and candidate counts so operators can distinguish discovery, selection, extraction, and report generation.
- Update research API documentation, its OpenAPI contract test, sample configuration, and README to describe the new workflow and configuration.

## Capabilities

### New Capabilities

- `research-source-selection`: Select a bounded, relevant set of discovered search results before any source page is downloaded or extracted.

### Modified Capabilities

- `operation-event-recording`: Record source-selection as a correlated research step.

## Impact

- Affects `research/domain` and `research/application` workflow, prompts, LLM construction in `cmd/api/main.go`, and research configuration loading and validation.
- Affects the successful `POST /v1/research` diagnostics payload and its OpenAPI/README contract, but not its request schema, route, authentication, or error-status mapping.
- Adds one configured OpenAI-compatible completion call per research operation before source crawling; it reuses the existing shared LLM endpoint, credentials, and headers.
