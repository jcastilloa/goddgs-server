## Context

The current research use case follows query planning, goddgs search, AI extraction of every unique result URL, and report writing. `results_per_query` therefore controls both discovery breadth and the number of pages downloaded. This couples a noisy search-result list to the costly crawling/extraction stage and means weak pages can consume the research timeout without improving the report.

The new stage must work before any source-page extraction. It needs the same OpenAI-compatible infrastructure as query planning and report writing, but its model, timeout, temperature, and retries must be independently configurable. The public route and request shape remain unchanged, while successful diagnostics gain selection measurements.

## Goals / Non-Goals

**Goals:**

- Discover more candidate results without downloading pages that the selector rejects.
- Give `selection_ai` only normalized `id`, `title`, `description`, and `url` metadata, never raw result objects or source HTML.
- Enforce in Go that selected IDs exist, are unique, preserve the selector's order, and do not exceed an operator-configured extraction budget.
- Bound the number of candidates included in the selector prompt independently of caller-supplied search breadth.
- Keep cancellation, timeout, rate-limit, sanitization, final-URL deduplication, report-source validation, and the existing HTTP error mapping intact.
- Make selection independently observable and configurable through Viper, sample configuration, OpenAPI, and README.

**Non-Goals:**

- Ranking or crawling pages outside the current synchronous `POST /v1/research` request.
- Changing research request fields, adding a new endpoint, persisting candidate lists or LLM prompts, or exposing rejected URLs.
- Treating the selector's choice as proof of source quality; an approved page still must yield non-empty AI-extracted content.
- Changing the retry behavior of the existing planner or reporter as part of this change.

## Decisions

### 1. Add a selection phase before extraction

The use case becomes:

```text
plan queries -> search goddgs -> normalize/deduplicate candidates
             -> selection_ai -> validate ordered IDs -> AI extract selected URLs
             -> final-URL deduplication -> report_ai
```

`results_per_query` remains the discovery breadth for each generated query. It no longer implies that every returned URL will be crawled. The selector receives valid, URL-deduplicated discovered candidates in stable order up to `research.max_selection_candidates`, and only its validated output is given to the existing bounded extraction worker pool.

This is preferable to filtering after extraction because it prevents unwanted source-page requests, avoids AI extraction calls for rejected candidates, and keeps the report model focused on actual extracted evidence.

### 2. Define a small application port and typed candidate contract

`research/application` will define a consumer-owned `Selector` port alongside `Planner` and `Reporter`, with typed request/response values for candidate selection. Candidate IDs are assigned by the server in stable discovery order and selection returns only `candidate_ids`.

Each candidate contains only:

- `id`: opaque server-generated candidate ID;
- `title`: result title, falling back to the URL;
- `description`: best-effort result snippet, empty when the backend did not provide one;
- `url`: validated HTTP(S) source URL.

The normalizer will accept only common string description fields (`body` first, then `description`) and will never forward `RawResult` maps, nested provider payloads, or arbitrary fields. Existing URL validation and discovery-order deduplication remain in Go.

The selector is implemented as `LLMSelector` using the existing `CompletionModel`, so the use case remains independent from OpenAI HTTP details and tests can use a small fake. A separate package, queue, or repository is not justified: selection is one synchronous research use-case phase with no persistence requirement.

### 3. Use strict structured output and server-side selection validation

The selector prompt will request exactly this JSON object:

```json
{"candidate_ids":["candidate-3","candidate-8"]}
```

Its system instructions treat the entire research request and candidate list as untrusted data; metadata cannot provide instructions. The user prompt will encode the bounded candidate metadata as JSON inside a data boundary, rather than interpolating provider text into markup attributes.

Go will reject malformed JSON, unknown IDs, duplicate IDs, an empty selection, or a list longer than `research.max_selected_sources` as an invalid upstream response. The selected candidates retain the response order. There is deliberately no fallback to extracting all candidates: that would violate the new cost and crawling boundary. A selector error or invalid selection follows the current research failure path (502 unless cancellation, timeout, or rate limiting already determines the response).

### 4. Make extraction budget explicit and configuration-owned

`ResearchConfig` gains:

```yaml
research:
  # Maximum normalized search results sent as metadata to selection_ai.
  max_selection_candidates: 100
  # Maximum selection_ai-approved URLs that may be downloaded.
  max_selected_sources: 20
  selection_ai:
    model: gpt-4.1-mini
    timeout: 30s
    temperature: 0.1
    retries: 2
```

`max_selection_candidates` is a positive prompt budget: after valid-URL normalization and deduplication, later candidates are omitted from selection and are never downloaded. `max_selected_sources` is a positive extraction budget distinct from `max_concurrent_extractions`: the former limits how many approved pages can be requested; the latter limits how many requests can be in flight. Both are validated at startup and read through Viper. `selection_ai` reuses `ResearchAIConfig` validation and shares only `llm.base_url`, `llm.api_key`, and `llm.headers` with the other research LLM clients.

These explicit caps are preferable to trusting the model to choose a reasonable count or reusing concurrency as a semantic limit. They bound prompt size, downloads, report volume, and worst-case operation latency while allowing operators to increase discovery breadth with `query_count` and `results_per_query`.

### 5. Build and record the selection client independently

The composition root constructs a third OpenAI-compatible research client from `research.selection_ai`, wraps it with the operation completion recorder using provider label `llm_selection`, and injects `LLMSelector` into `research.NewService`. The existing compatible client applies the configured transport/provider retries. Selection parsing and semantic validation occur in the application layer; this change does not add a second retry loop or alter existing query/report retry semantics.

The operation model gains `research_selection`. The research operation records it with discovered and selected candidate counts, without recording candidate URLs, prompts, snippets, or completion responses. This maintains the existing sanitized operational-data boundary.

### 6. Extend successful diagnostics without exposing candidates

`domain.Diagnostics` and the 200 response gain:

- `source_selection_ms`;
- `candidates_found`;
- `candidates_selected`.

Counts describe URL-valid, deduplicated result candidates and the validated pre-extraction shortlist, respectively. They do not disclose candidate metadata or rejected URLs. Existing timing fields keep their meaning; `source_extraction_ms` measures only selected pages after this change and `total_ms` includes selection.

## Risks / Trade-offs

- [A relevant source can be rejected from thin or misleading SERP metadata] → The selector receives title, snippet, and URL, is prompted for coverage/diversity/relevance, and the operator can tune discovery breadth and selection budget. The change does not claim perfect factual quality.
- [Approved pages can fail extraction, leaving few usable sources] → The selector can return an ordered shortlist up to the configured budget; existing empty/failed extraction handling and 502 behavior remain. No rejected page is silently crawled as fallback.
- [Large caller-supplied query/result counts can create an oversized selector prompt] → Candidate metadata is deliberately narrow and `max_selection_candidates` places a validated, configuration-owned bound on selector input; `max_selected_sources` separately bounds downstream requests.
- [Extra LLM call increases request latency and provider failure surface] → Use a separate timeout and retries, record timing/step status, propagate context cancellation, and preserve the global research timeout.
- [SERP metadata contains prompt injection] → Treat it as untrusted data in the system prompt, serialize only the four approved fields, and validate output IDs entirely in Go.
- [New diagnostics fields affect generated clients] → Update OpenAPI examples/schema, its contract test, and README together; successful responses are intentionally extended, not request-breaking.

## Migration Plan

1. Deploy code and sample configuration containing valid `research.selection_ai`, `research.max_selection_candidates`, and `research.max_selected_sources` values.
2. Existing operators add the new required settings before enabling the binary; otherwise research reports its normal 503 configuration error while ordinary search and extraction remain available.
3. Observe `source_selection_ms`, candidate counts, selection failures, and extracted-source counts to tune discovery and selection budgets.
4. Roll back by restoring the preceding binary and configuration. No database migration, durable state, or route migration is required.

## Open Questions

None. Initial production values (`max_selection_candidates: 100` and `max_selected_sources: 20`) are configuration defaults in the sample; operators can tune them based on provider limits and observed selection diagnostics.
