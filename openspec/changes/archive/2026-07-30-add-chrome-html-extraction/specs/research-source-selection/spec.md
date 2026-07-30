## ADDED Requirements

### Requirement: Configured HTML loader for selected research sources
The system SHALL use the configured HTML extraction loader for each source URL validated by source selection before AI extraction. When Chrome HTML extraction is enabled, selected research sources SHALL use the same Chrome configuration, shared proxy pool, browser concurrency limits, page timeout, sanitization, and error behavior as `GET /v1/extract?format=html`.

The source-extraction stage SHALL retain its existing selected-source concurrency bound and research timeout. Browser-capacity waits and page loads SHALL honor the research operation context; a selected source that cannot load through Chrome SHALL retain the existing behavior of being omitted from the report rather than causing rejected candidates to be crawled as fallback.

#### Scenario: Selected sources share browser capacity
- **WHEN** a research request extracts multiple selection-approved sources while Chrome is enabled
- **THEN** their browser page leases are bounded by Chrome configuration in addition to `research.max_concurrent_extractions`

#### Scenario: Browser loading fails for one selected source
- **WHEN** Chrome cannot load one selection-approved source during research
- **THEN** the research workflow omits that source according to existing extraction failure behavior and does not crawl a selector-rejected source as replacement
