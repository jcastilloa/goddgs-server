## ADDED Requirements

### Requirement: Pre-extraction selection of search results
The system SHALL normalize and URL-deduplicate the valid HTTP(S) results discovered from every generated research query before any source page is requested. It SHALL submit only each candidate's server-assigned ID, title, description when available, and URL to the configured selection LLM. It MUST NOT submit page HTML, raw provider result maps, nested provider data, or arbitrary result fields to that LLM.

The system SHALL submit no more than `research.max_selection_candidates` normalized candidates to the selection LLM. The selection LLM SHALL return an ordered JSON list of candidate IDs. The system MUST validate that the list is non-empty, contains no duplicate or unknown IDs, and contains no more than `research.max_selected_sources` IDs. It SHALL pass only the validated candidates, in returned order, to source extraction. Candidates not selected by the LLM, including candidates beyond the selection-input limit, MUST NOT be downloaded, crawled, or submitted to AI extraction.

#### Scenario: Irrelevant result is rejected before crawling
- **WHEN** goddgs discovers valid candidate results and the selection LLM returns only a subset of their IDs
- **THEN** the system requests source extraction only for that subset and makes no source-page request for every rejected candidate

#### Scenario: Candidate metadata is incomplete
- **WHEN** a valid search result has a title and URL but no provider description
- **THEN** the selector receives the candidate with an empty description and the result remains eligible for selection

#### Scenario: Discovery exceeds the selector input budget
- **WHEN** normalized and deduplicated discovery yields more candidates than `research.max_selection_candidates`
- **THEN** the selector receives only the first candidates in stable discovery order up to that limit and the omitted candidates are not downloaded or extracted

#### Scenario: Selector returns an invalid shortlist
- **WHEN** the selection LLM returns malformed JSON, no IDs, an unknown ID, a duplicate ID, or more IDs than the configured maximum
- **THEN** the research request fails as an upstream research failure and the system does not start source extraction

#### Scenario: Selected page cannot be extracted
- **WHEN** a selected candidate's source page fails extraction or has no usable content
- **THEN** the system omits that page using the existing extraction behavior and MUST NOT crawl a candidate rejected by the selector as fallback

### Requirement: Independently configured source selection LLM
The system SHALL obtain `research.selection_ai.model`, `research.selection_ai.timeout`, `research.selection_ai.temperature`, `research.selection_ai.retries`, `research.max_selection_candidates`, and `research.max_selected_sources` through Viper. It SHALL require the same validity constraints as the query and report research AI configurations and SHALL require both limits to be positive before research is available.

The selection client SHALL use the shared `llm.base_url`, `llm.api_key`, and `llm.headers`, while its model, timeout, temperature, and retries remain independent from `research.query_ai` and `research.report_ai`.

#### Scenario: Valid independent selection configuration
- **WHEN** an operator configures valid query, selection, and report AI settings with different model or timeout values
- **THEN** the service constructs the selection completion client with the selection settings and the other research clients retain their own settings

#### Scenario: Missing selection configuration
- **WHEN** any required `research.selection_ai` field, `research.max_selection_candidates`, or `research.max_selected_sources` is missing or invalid
- **THEN** research returns its documented configuration-unavailable response while ordinary search and extraction remain available

### Requirement: Observable source selection
The system SHALL record source selection as a `research_selection` step correlated with the enclosing research operation. The recorded step MUST include discovered and selected candidate counts and MUST NOT contain candidate URLs, titles, descriptions, prompts, or LLM responses.

Every successful research response SHALL include `diagnostics.source_selection_ms`, `diagnostics.candidates_found`, and `diagnostics.candidates_selected`. `source_extraction_ms` SHALL measure only the pages selected before extraction, and `total_ms` SHALL include source selection.

#### Scenario: Successful selected research
- **WHEN** a research operation discovers candidates, selects a shortlist, extracts selected pages, and writes a report
- **THEN** its operation record contains a completed `research_selection` step and its response reports the selection duration and both candidate counts without exposing rejected candidates

#### Scenario: Selection provider failure
- **WHEN** the selection LLM call fails
- **THEN** the correlated `research_selection` step is failed with sanitized error data and the request follows the existing research error mapping

### Requirement: Source-selection contract documentation
The runtime OpenAPI document, its research contract test, sample configuration, and README SHALL document that research first evaluates search-result metadata and only downloads URLs selected by `selection_ai`. They SHALL document the `research.selection_ai.*`, `research.max_selection_candidates`, `research.max_selected_sources`, selection diagnostics, selection criteria, metadata fields, and meaningful configuration and upstream-failure responses.

#### Scenario: Client reads research documentation
- **WHEN** a client obtains `/openapi.json` or an operator reads the README or sample configuration
- **THEN** it can determine that rejected results are not crawled, which metadata reaches the selector, which selection settings are required, and how selection metrics appear in a successful response
