## 1. Test specification and configuration

- [x] 1.1 Add failing domain/configuration tests for positive `research.max_selection_candidates`, positive `research.max_selected_sources`, and complete independent `research.selection_ai` settings.
- [x] 1.2 Add failing Viper repository tests proving it reads both selection limits and every `research.selection_ai.*` value without changing query or report configuration.
- [x] 1.3 Add `research.selection_ai`, `research.max_selection_candidates`, and `research.max_selected_sources` to `config.sample.yaml`; implement the minimum configuration loading and validation needed to make the new tests pass.

## 2. Selection contract and LLM adapter

- [x] 2.1 Add failing unit tests for normalized selection candidates: stable IDs, title fallback, `body` then `description` snippet selection, URL validation/deduplication, stable candidate-input truncation, and exclusion of raw-provider fields.
- [x] 2.2 Add failing `LLMSelector` tests for its untrusted-data prompt boundary, strict `{"candidate_ids":[...]}` decoding, configured completion behavior, cancellation/error propagation, and invalid structured responses.
- [x] 2.3 Implement the smallest typed candidate/selection request-response contract, candidate normalization helpers, selector port, and `LLMSelector` required by the tests; preserve the existing planner/reporter behavior.

## 3. Research workflow and observability

- [x] 3.1 Add failing service tests proving that only valid, uniquely selected candidate IDs are extracted in selector order and that rejected or input-budget-excluded URLs are never sent to the extractor.
- [x] 3.2 Add failing service tests for unknown, duplicate, empty, over-budget, malformed, canceled, rate-limited, and failed selector responses; verify no extraction begins on invalid selection and no fallback crawls rejected URLs.
- [x] 3.3 Add failing service tests for selection diagnostics (`source_selection_ms`, discovered/selected counts), final-URL deduplication after selected extraction, and preservation of existing report-source validation/error behavior.
- [x] 3.4 Extend the research service to invoke selection after discovery and before extraction, validate the returned IDs in Go, preserve context cancellation and extraction concurrency semantics, and populate the new diagnostics.
- [x] 3.5 Add `research_selection` to the operations step model and tests; record only selection counts and sanitized failure state, never candidate metadata, prompts, or completion responses.

## 4. Composition and API contract

- [x] 4.1 Add failing composition-root tests for a separately configured selection client and its `llm_selection` operation recorder label.
- [x] 4.2 Construct and inject the `selection_ai` client and `LLMSelector` from `cmd/api/main.go`; retain the existing unavailable-service behavior when any required research setting is invalid.
- [x] 4.3 Update the runtime OpenAPI research description, success schema/examples, configuration prerequisites, and contract test for selection behavior, limits, and diagnostics without changing the request schema or error-status mapping.
- [x] 4.4 Update README research workflow and configuration documentation to state that selection receives only title/description/URL metadata, rejected URLs are never crawled, and extraction/concurrency budgets differ.

## 5. Verification and refactor

- [x] 5.1 Run focused RED/GREEN tests incrementally for domain config, Viper configuration, research application, operation recording, composition, and OpenAPI contract.
- [x] 5.2 Refactor only newly touched research code into small cohesive functions; preserve exported APIs, error identities, slice/order semantics, cancellation, and goroutine lifecycle.
- [x] 5.3 Run `gofmt` on changed Go files, `go test ./...`, `go test -race ./...`, and `go vet ./...`; resolve regressions before marking the change complete.
