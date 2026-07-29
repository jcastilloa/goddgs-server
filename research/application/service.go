package application

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	operations "github.com/jcastilloa/goddgs-server/operations/domain"
	"github.com/jcastilloa/goddgs-server/research/domain"
	searchDomain "github.com/jcastilloa/goddgs-server/search/domain"
	extractAIDomain "github.com/jcastilloa/goddgs-server/shared/extractai/domain"
	"golang.org/x/net/html"
)

type Planner interface {
	Plan(context.Context, domain.NormalizedRequest) ([]domain.GeneratedQuery, error)
}

type Searcher interface {
	Search(context.Context, searchDomain.SearchRequest) ([]searchDomain.RawResult, error)
}

type Extractor interface {
	Extract(context.Context, extractAIDomain.Request) (extractAIDomain.Result, error)
}

type Reporter interface {
	Write(context.Context, ReportRequest) (Report, error)
}

type Selector interface {
	Select(context.Context, SelectionRequest) (Selection, error)
}

type SelectionCandidate struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

type SelectionRequest struct {
	Query      string               `json:"query"`
	Candidates []SelectionCandidate `json:"candidates"`
}

type Selection struct {
	CandidateIDs []string `json:"candidate_ids"`
}

type StepRecorder interface {
	StartStep(context.Context, operations.StepStart) (operations.Step, error)
	FinishStep(context.Context, operations.Step, error) error
}

type ReportSource struct {
	ID      string
	URL     string
	Title   string
	Content string
}

type ReportRequest struct {
	Query    string
	Language string
	Sources  []ReportSource
}

type Report struct {
	HTML      string   `json:"html"`
	SourceIDs []string `json:"source_ids"`
}

type Service struct {
	planner                  Planner
	selector                 Selector
	searcher                 Searcher
	extractor                Extractor
	reporter                 Reporter
	recorder                 StepRecorder
	maxSelectionCandidates   int
	maxSelectedSources       int
	maxConcurrentExtractions int
}

type Limits struct {
	MaxSelectionCandidates   int
	MaxSelectedSources       int
	MaxConcurrentExtractions int
}

func NewService(planner Planner, selector Selector, searcher Searcher, extractor Extractor, reporter Reporter, limits Limits, recorders ...StepRecorder) Service {
	var recorder StepRecorder
	if len(recorders) > 0 {
		recorder = recorders[0]
	}
	return Service{
		planner:                  planner,
		selector:                 selector,
		searcher:                 searcher,
		extractor:                extractor,
		reporter:                 reporter,
		recorder:                 recorder,
		maxSelectionCandidates:   limits.MaxSelectionCandidates,
		maxSelectedSources:       limits.MaxSelectedSources,
		maxConcurrentExtractions: limits.MaxConcurrentExtractions,
	}
}

func (s Service) Research(ctx context.Context, request domain.Request) (domain.Result, error) {
	if err := ctx.Err(); err != nil {
		return domain.Result{}, err
	}
	normalized, err := request.Normalize()
	if err != nil {
		return domain.Result{}, err
	}
	if s.planner == nil || s.selector == nil || s.searcher == nil || s.extractor == nil || s.reporter == nil || s.maxSelectionCandidates <= 0 || s.maxSelectedSources <= 0 || s.maxConcurrentExtractions <= 0 {
		return domain.Result{}, domain.ErrUnavailable
	}

	startedAt := time.Now()
	planningStartedAt := time.Now()
	queries, err := recordPhase(ctx, s.recorder, operations.StepResearchPlanning, map[string]string{"query": normalized.Query}, func() ([]domain.GeneratedQuery, error) {
		return s.planner.Plan(ctx, normalized)
	})
	if err != nil {
		return domain.Result{}, fmt.Errorf("generate research queries: %w", err)
	}
	if err := validateQueries(normalized, queries); err != nil {
		return domain.Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.Result{}, err
	}
	planningDuration := time.Since(planningStartedAt)

	diagnostics := newDiagnostics()
	searchStartedAt := time.Now()
	discovered := s.searchURLs(ctx, normalized, queries, diagnostics)
	searchDuration := time.Since(searchStartedAt)
	if err := ctx.Err(); err != nil {
		return domain.Result{}, err
	}
	selectionStartedAt := time.Now()
	selected, err := s.selectCandidates(ctx, normalized, discovered)
	selectionDuration := time.Since(selectionStartedAt)
	if err != nil {
		return domain.Result{}, fmt.Errorf("select research sources: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return domain.Result{}, err
	}
	extractionStartedAt := time.Now()
	sources := s.extractedSources(ctx, selected)
	extractionDuration := time.Since(extractionStartedAt)
	if err := ctx.Err(); err != nil {
		return domain.Result{}, err
	}
	if len(sources) == 0 {
		return domain.Result{}, domain.ErrNoUsableSources
	}
	reportStartedAt := time.Now()
	report, err := s.recordReport(ctx, normalized, sources)
	if err != nil {
		return domain.Result{}, fmt.Errorf("write research report: %w", err)
	}
	result, err := buildResult(report, sources)
	if err != nil {
		return domain.Result{}, err
	}
	result.Diagnostics = diagnostics.result(
		planningDuration,
		searchDuration,
		selectionDuration,
		extractionDuration,
		time.Since(reportStartedAt),
		time.Since(startedAt),
		len(discovered),
		len(selected),
	)
	return result, nil
}

func (s Service) selectCandidates(ctx context.Context, request domain.NormalizedRequest, discovered []candidateSource) ([]candidateSource, error) {
	candidates := selectionCandidates(discovered, s.maxSelectionCandidates)
	if len(candidates) == 0 {
		return nil, domain.ErrNoUsableSources
	}
	return s.recordSelection(ctx, request.Query, candidates, len(discovered))
}

func selectionCandidates(discovered []candidateSource, maximum int) []candidateSource {
	if maximum <= 0 {
		return nil
	}
	limit := min(len(discovered), maximum)
	candidates := make([]candidateSource, limit)
	for index := range candidates {
		candidates[index] = discovered[index]
		candidates[index].ID = fmt.Sprintf("candidate-%d", index+1)
	}
	return candidates
}

func (s Service) recordSelection(ctx context.Context, query string, candidates []candidateSource, candidatesFound int) ([]candidateSource, error) {
	if s.recorder == nil {
		selection, err := s.selector.Select(ctx, selectionRequest(query, candidates))
		if err != nil {
			return nil, err
		}
		return validateSelection(candidates, selection, s.maxSelectedSources)
	}
	request := selectionRequest(query, candidates)
	step, _ := s.recorder.StartStep(ctx, operations.StepStart{Type: operations.StepResearchSelection, Metadata: selectionMetadata(candidatesFound, len(candidates), 0), Details: selectionDetails(request, Selection{}, nil)})
	selection, err := s.selector.Select(ctx, request)
	selected := []candidateSource(nil)
	if err == nil {
		selected, err = validateSelection(candidates, selection, s.maxSelectedSources)
	}
	step.Metadata = selectionMetadata(candidatesFound, len(candidates), len(selected))
	step.Details = selectionDetails(request, selection, selected)
	if finishErr := s.recorder.FinishStep(ctx, step, err); finishErr != nil && err == nil {
		return nil, finishErr
	}
	return selected, err
}

func selectionDetails(request SelectionRequest, selection Selection, selected []candidateSource) json.RawMessage {
	details := struct {
		SelectionRequest   SelectionRequest     `json:"selection_request"`
		SelectionResponse  *Selection           `json:"selection_response,omitempty"`
		SelectedCandidates []SelectionCandidate `json:"selected_candidates,omitempty"`
	}{SelectionRequest: request}
	if selection.CandidateIDs != nil {
		details.SelectionResponse = &selection
	}
	if selected != nil {
		details.SelectedCandidates = selectionCandidatesForDetail(selected)
	}
	encoded, _ := json.Marshal(details)
	return encoded
}

func selectionCandidatesForDetail(candidates []candidateSource) []SelectionCandidate {
	details := make([]SelectionCandidate, len(candidates))
	for index, candidate := range candidates {
		details[index] = SelectionCandidate{ID: candidate.ID, Title: candidate.Title, Description: candidate.Description, URL: candidate.URL}
	}
	return details
}

func selectionMetadata(candidatesFound, candidatesSubmitted, candidatesSelected int) map[string]string {
	return map[string]string{
		"candidates_found":     fmt.Sprintf("%d", candidatesFound),
		"candidates_submitted": fmt.Sprintf("%d", candidatesSubmitted),
		"candidates_selected":  fmt.Sprintf("%d", candidatesSelected),
	}
}

func selectionRequest(query string, candidates []candidateSource) SelectionRequest {
	request := SelectionRequest{Query: query, Candidates: make([]SelectionCandidate, len(candidates))}
	for index, candidate := range candidates {
		request.Candidates[index] = SelectionCandidate{ID: candidate.ID, Title: candidate.Title, Description: candidate.Description, URL: candidate.URL}
	}
	return request
}

func validateSelection(candidates []candidateSource, selection Selection, maximum int) ([]candidateSource, error) {
	if len(selection.CandidateIDs) == 0 {
		return nil, fmt.Errorf("%w: source selection is empty", domain.ErrInvalidResponse)
	}
	if len(selection.CandidateIDs) > maximum {
		return nil, fmt.Errorf("%w: source selection exceeds maximum", domain.ErrInvalidResponse)
	}
	byID := make(map[string]candidateSource, len(candidates))
	for _, candidate := range candidates {
		byID[candidate.ID] = candidate
	}
	selected := make([]candidateSource, 0, len(selection.CandidateIDs))
	seen := make(map[string]struct{}, len(selection.CandidateIDs))
	for _, id := range selection.CandidateIDs {
		candidate, exists := byID[id]
		if !exists {
			return nil, fmt.Errorf("%w: source selection references an unknown candidate", domain.ErrInvalidResponse)
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("%w: source selection references a candidate more than once", domain.ErrInvalidResponse)
		}
		seen[id] = struct{}{}
		selected = append(selected, candidate)
	}
	return selected, nil
}

func validateQueries(request domain.NormalizedRequest, queries []domain.GeneratedQuery) error {
	if len(queries) != request.QueryCount {
		return fmt.Errorf("%w: expected %d generated queries", domain.ErrInvalidResponse, request.QueryCount)
	}

	counts := make(map[string]int, len(request.QueryLanguages))
	allowed := make(map[string]struct{}, len(request.QueryLanguages))
	for _, language := range request.QueryLanguages {
		allowed[language] = struct{}{}
	}
	for _, query := range queries {
		language := strings.ToLower(strings.TrimSpace(query.Language))
		if _, exists := allowed[language]; !exists || strings.TrimSpace(query.Query) == "" {
			return fmt.Errorf("%w: generated query has an unsupported language or is empty", domain.ErrInvalidResponse)
		}
		counts[language]++
	}
	for index, language := range request.QueryLanguages {
		expected := request.QueryCount / len(request.QueryLanguages)
		if index < request.QueryCount%len(request.QueryLanguages) {
			expected++
		}
		if counts[language] != expected {
			return fmt.Errorf("%w: expected %d generated queries in %q", domain.ErrInvalidResponse, expected, language)
		}
	}
	return nil
}

func (s Service) extractedSources(ctx context.Context, urls []candidateSource) []ReportSource {
	extracted, _ := recordPhase(ctx, s.recorder, operations.StepResearchExtract, map[string]string{"candidate_count": fmt.Sprintf("%d", len(urls))}, func() ([]*ReportSource, error) {
		return s.extractAll(ctx, urls), nil
	})
	sources := make([]ReportSource, 0, len(urls))
	seenFinalURLs := make(map[string]struct{}, len(urls))
	for _, source := range extracted {
		if source == nil {
			continue
		}
		if _, exists := seenFinalURLs[source.URL]; exists {
			continue
		}
		seenFinalURLs[source.URL] = struct{}{}
		source.ID = fmt.Sprintf("source-%d", len(sources)+1)
		sources = append(sources, *source)
	}
	return sources
}

func (s Service) extractAll(ctx context.Context, candidates []candidateSource) []*ReportSource {
	results := make([]*ReportSource, len(candidates))
	if len(candidates) == 0 {
		return results
	}

	jobs := make(chan int)
	var waitGroup sync.WaitGroup
	for range min(s.maxConcurrentExtractions, len(candidates)) {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case index, ok := <-jobs:
					if !ok {
						return
					}
					results[index] = s.extractSource(ctx, candidates[index])
				}
			}
		}()
	}

	for index := range candidates {
		select {
		case <-ctx.Done():
			close(jobs)
			waitGroup.Wait()
			return results
		case jobs <- index:
		}
	}
	close(jobs)
	waitGroup.Wait()
	return results
}

func (s Service) extractSource(ctx context.Context, candidate candidateSource) *ReportSource {
	result, err := s.extractor.Extract(ctx, extractAIDomain.Request{URL: candidate.URL})
	if err != nil || strings.TrimSpace(result.Content) == "" {
		return nil
	}
	finalURL := result.URL
	if finalURL == "" {
		finalURL = candidate.URL
	}
	if !isHTTPURL(finalURL) {
		return nil
	}
	content := sourceText(result.Content)
	if content == "" {
		return nil
	}
	return &ReportSource{URL: finalURL, Title: candidate.Title, Content: content}
}

func (s Service) searchURLs(ctx context.Context, request domain.NormalizedRequest, queries []domain.GeneratedQuery, diagnostics *researchDiagnostics) []candidateSource {
	urls, _ := recordPhase(ctx, s.recorder, operations.StepResearchSearch, map[string]string{"query_count": fmt.Sprintf("%d", len(queries))}, func() ([]candidateSource, error) {
		urls := make([]candidateSource, 0, request.QueryCount*request.ResultsPerQuery)
		seenURLs := make(map[string]struct{}, cap(urls))
		for _, query := range queries {
			if ctx.Err() != nil {
				return urls, nil
			}
			region, err := request.RegionFor(query.Language)
			if err != nil {
				return urls, nil
			}
			maxResults := request.ResultsPerQuery
			results, err := s.searcher.Search(ctx, searchDomain.SearchRequest{
				Category:   searchDomain.CategoryText,
				Query:      query.Query,
				Region:     region,
				MaxResults: &maxResults,
				Diagnostics: &searchDomain.SearchDiagnostics{
					OnComplete: diagnostics.record,
				},
			})
			if err != nil {
				continue
			}
			accepted := 0
			for _, result := range results {
				if accepted >= request.ResultsPerQuery {
					break
				}
				candidate, ok := sourceFromResult(result)
				if !ok {
					continue
				}
				if _, exists := seenURLs[candidate.URL]; exists {
					continue
				}
				seenURLs[candidate.URL] = struct{}{}
				urls = append(urls, candidate)
				accepted++
			}
		}
		return urls, nil
	})
	return urls
}

func (s Service) recordReport(ctx context.Context, request domain.NormalizedRequest, sources []ReportSource) (Report, error) {
	return recordPhase(ctx, s.recorder, operations.StepResearchReport, map[string]string{
		"query":        request.Query,
		"source_count": fmt.Sprintf("%d", len(sources)),
	}, func() (Report, error) {
		return s.reporter.Write(ctx, ReportRequest{Query: request.Query, Language: request.ReportLanguage, Sources: sources})
	})
}

func recordPhase[T any](ctx context.Context, recorder StepRecorder, stepType operations.StepType, metadata map[string]string, run func() (T, error)) (T, error) {
	var zero T
	if recorder == nil {
		return run()
	}
	step, _ := recorder.StartStep(ctx, operations.StepStart{Type: stepType, Metadata: metadata})
	result, err := run()
	if finishErr := recorder.FinishStep(ctx, step, err); finishErr != nil && err == nil {
		return zero, finishErr
	}
	return result, err
}

type researchDiagnostics struct {
	mu       sync.Mutex
	backends map[string]domain.BackendDiagnostic
}

func newDiagnostics() *researchDiagnostics {
	return &researchDiagnostics{backends: make(map[string]domain.BackendDiagnostic)}
}

func (d *researchDiagnostics) record(diagnostic searchDomain.SearchDiagnostic) {
	d.mu.Lock()
	defer d.mu.Unlock()
	backend := d.backends[diagnostic.Backend]
	backend.Name = diagnostic.Backend
	backend.Provider = diagnostic.Provider
	backend.Attempts++
	backend.ResultCount += diagnostic.ResultCount
	if diagnostic.Err != nil {
		backend.ErrorCount++
	}
	d.backends[diagnostic.Backend] = backend
}

func (d *researchDiagnostics) result(planning, search, selection, extraction, report, total time.Duration, candidatesFound, candidatesSelected int) domain.Diagnostics {
	d.mu.Lock()
	defer d.mu.Unlock()
	backends := make([]domain.BackendDiagnostic, 0, len(d.backends))
	for _, backend := range d.backends {
		backends = append(backends, backend)
	}
	result := domain.Diagnostics{
		Backends:           backends,
		QueryPlanningMS:    planning.Milliseconds(),
		SearchMS:           search.Milliseconds(),
		SourceSelectionMS:  selection.Milliseconds(),
		SourceExtractionMS: extraction.Milliseconds(),
		ReportGenerationMS: report.Milliseconds(),
		TotalMS:            total.Milliseconds(),
		CandidatesFound:    candidatesFound,
		CandidatesSelected: candidatesSelected,
	}
	result.SortBackends()
	return result
}

type candidateSource struct {
	ID          string
	URL         string
	Title       string
	Description string
}

func sourceFromResult(result searchDomain.RawResult) (candidateSource, bool) {
	urlValue := stringValue(result["href"])
	if urlValue == "" {
		urlValue = stringValue(result["url"])
	}
	if !isHTTPURL(urlValue) {
		return candidateSource{}, false
	}
	title := stringValue(result["title"])
	if title == "" {
		title = urlValue
	}
	description := stringValue(result["body"])
	if description == "" {
		description = stringValue(result["description"])
	}
	return candidateSource{URL: urlValue, Title: title, Description: description}, true
}

func buildResult(report Report, sources []ReportSource) (domain.Result, error) {
	if strings.TrimSpace(report.HTML) == "" || len(report.SourceIDs) == 0 {
		return domain.Result{}, fmt.Errorf("%w: report must contain HTML and source IDs", domain.ErrInvalidResponse)
	}
	cleanHTML, err := sanitizeReportHTML(report.HTML)
	if err != nil {
		return domain.Result{}, fmt.Errorf("%w: sanitize report HTML: %v", domain.ErrInvalidResponse, err)
	}
	if cleanHTML == "" {
		return domain.Result{}, fmt.Errorf("%w: report HTML has no usable content", domain.ErrInvalidResponse)
	}
	byID := make(map[string]ReportSource, len(sources))
	for _, source := range sources {
		byID[source.ID] = source
	}
	result := domain.Result{ReportHTML: cleanHTML, Sources: make([]domain.Source, 0, len(report.SourceIDs))}
	seenIDs := make(map[string]struct{}, len(report.SourceIDs))
	for _, id := range report.SourceIDs {
		source, exists := byID[id]
		if !exists {
			return domain.Result{}, fmt.Errorf("%w: report references an unknown source", domain.ErrInvalidResponse)
		}
		if _, exists := seenIDs[id]; exists {
			return domain.Result{}, fmt.Errorf("%w: report references a source more than once", domain.ErrInvalidResponse)
		}
		seenIDs[id] = struct{}{}
		result.Sources = append(result.Sources, domain.Source{URL: source.URL, Title: source.Title})
	}
	return result, nil
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func isHTTPURL(rawURL string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	return err == nil && parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func sourceText(content string) string {
	document, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return ""
	}
	var text strings.Builder
	writeText(&text, document)
	return strings.Join(strings.Fields(text.String()), " ")
}

func writeText(output *strings.Builder, node *html.Node) {
	if node.Type == html.TextNode {
		output.WriteString(node.Data)
		output.WriteByte(' ')
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		writeText(output, child)
	}
}
