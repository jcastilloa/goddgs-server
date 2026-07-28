package application

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	operations "github.com/jcastilloa/goddgs-server/operations/domain"
	"github.com/jcastilloa/goddgs-server/research/domain"
	searchDomain "github.com/jcastilloa/goddgs-server/search/domain"
	extractAIDomain "github.com/jcastilloa/goddgs-server/shared/extractai/domain"
)

func TestServiceResearchesGeneratedQueriesExtractsUniqueSourcesAndBuildsReport(t *testing.T) {
	maxResults := 2
	searcher := &recordingSearcher{results: map[string][]searchDomain.RawResult{
		"E.T. release date": {
			{"href": "https://example.com/et", "title": "E.T. history"},
			{"url": "https://example.com/box-office", "title": "Box office"},
		},
		"fecha estreno ET": {
			{"href": "https://example.com/et", "title": "Duplicate"},
		},
	}, diagnostics: map[string][]searchDomain.SearchDiagnostic{
		"E.T. release date": {{Backend: "wikipedia", Provider: "wikipedia", ResultCount: 2}, {Backend: "google", Provider: "google", ResultCount: 1}},
		"fecha estreno ET":  {{Backend: "google", Provider: "google", ResultCount: 3}, {Backend: "brave", Provider: "brave", Err: errors.New("blocked")}},
	}}
	extractor := &recordingExtractor{results: map[string]extractAIDomain.Result{
		"https://example.com/et":         {URL: "https://example.com/et", Content: "<article><p>E.T. premiered in 1982.</p></article>"},
		"https://example.com/box-office": {URL: "https://example.com/box-office", Content: "<p>It opened with $11 million.</p>"},
	}}
	reporter := &recordingReporter{result: Report{HTML: "<article><p>Research report.</p></article>", SourceIDs: []string{"source-2"}}}
	service := newResearchService(
		&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "E.T. release date"}, {Language: "es", Query: "fecha estreno ET"}}},
		&recordingSelector{},
		searcher,
		extractor,
		reporter,
	)

	got, err := service.Research(context.Background(), domain.Request{
		Query: "When was E.T. released?", QueryLanguages: []string{"en", "es"}, QueryCount: intPointer(2), ResultsPerQuery: intPointer(maxResults),
	})
	if err != nil {
		t.Fatalf("Research() error = %v", err)
	}
	if got.ReportHTML != "<article><p>Research report.</p></article>" {
		t.Errorf("ReportHTML = %q", got.ReportHTML)
	}
	wantSources := []domain.Source{{URL: "https://example.com/box-office", Title: "Box office"}}
	if !reflect.DeepEqual(got.Sources, wantSources) {
		t.Errorf("Sources = %#v, want %#v", got.Sources, wantSources)
	}
	wantSearches := []searchDomain.SearchRequest{
		{Category: searchDomain.CategoryText, Query: "E.T. release date", Region: "us-en", MaxResults: &maxResults},
		{Category: searchDomain.CategoryText, Query: "fecha estreno ET", Region: "es-es", MaxResults: &maxResults},
	}
	for index := range searcher.requests {
		searcher.requests[index].Diagnostics = nil
	}
	if !reflect.DeepEqual(searcher.requests, wantSearches) {
		t.Errorf("searches = %#v, want %#v", searcher.requests, wantSearches)
	}
	if !sameStrings(extractor.urls, []string{"https://example.com/et", "https://example.com/box-office"}) {
		t.Errorf("extractions = %#v", extractor.urls)
	}
	if len(reporter.requests) != 1 || len(reporter.requests[0].Sources) != 2 || reporter.requests[0].Language != "en" {
		t.Errorf("report requests = %#v", reporter.requests)
	}
	wantBackends := []domain.BackendDiagnostic{
		{Name: "brave", Provider: "brave", Attempts: 1, ErrorCount: 1},
		{Name: "google", Provider: "google", Attempts: 2, ResultCount: 4},
		{Name: "wikipedia", Provider: "wikipedia", Attempts: 1, ResultCount: 2},
	}
	if !reflect.DeepEqual(got.Diagnostics.Backends, wantBackends) {
		t.Errorf("diagnostic backends = %#v, want %#v", got.Diagnostics.Backends, wantBackends)
	}
	if got.Diagnostics.QueryPlanningMS < 0 || got.Diagnostics.SearchMS < 0 || got.Diagnostics.SourceExtractionMS < 0 || got.Diagnostics.ReportGenerationMS < 0 || got.Diagnostics.TotalMS < 0 {
		t.Errorf("diagnostic timings = %#v, want non-negative values", got.Diagnostics)
	}
}

func TestSourceFromResultNormalizesOnlySelectionMetadata(t *testing.T) {
	candidate, ok := sourceFromResult(searchDomain.RawResult{
		"href":          "https://example.com/source",
		"body":          "Preferred description",
		"description":   "Fallback description",
		"provider_data": map[string]any{"secret": "must not reach selection"},
	})
	if !ok {
		t.Fatal("sourceFromResult() ok = false")
	}
	if candidate.URL != "https://example.com/source" || candidate.Title != "https://example.com/source" || candidate.Description != "Preferred description" {
		t.Errorf("candidate = %#v", candidate)
	}

	candidate, ok = sourceFromResult(searchDomain.RawResult{"url": "https://example.com/fallback", "description": "Fallback description"})
	if !ok || candidate.Description != "Fallback description" {
		t.Errorf("fallback candidate = %#v, ok = %v", candidate, ok)
	}
}

func TestServiceExtractsOnlySelectedCandidatesWithoutCrawlingRejectedResults(t *testing.T) {
	searcher := &recordingSearcher{results: map[string][]searchDomain.RawResult{
		"query": {
			{"href": "https://example.com/rejected", "title": "Rejected", "body": "Low relevance"},
			{"href": "https://example.com/selected", "title": "Selected", "description": "Strong evidence"},
			{"href": "https://example.com/excluded", "title": "Excluded", "body": "Past input budget"},
		},
	}}
	selector := &recordingSelector{selection: Selection{CandidateIDs: []string{"candidate-2"}}}
	extractor := &recordingExtractor{results: map[string]extractAIDomain.Result{
		"https://example.com/selected": {URL: "https://example.com/selected", Content: "<p>Selected evidence</p>"},
	}}
	service := newResearchService(
		&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "query"}}},
		selector,
		searcher,
		extractor,
		&recordingReporter{result: Report{HTML: "<p>Report</p>", SourceIDs: []string{"source-1"}}},
		withSelectionLimits(2, 1, 1),
	)

	got, err := service.Research(context.Background(), domain.Request{Query: "topic", QueryCount: intPointer(1), ResultsPerQuery: intPointer(3)})
	if err != nil {
		t.Fatalf("Research() error = %v", err)
	}
	if got.Diagnostics.CandidatesFound != 3 || got.Diagnostics.CandidatesSelected != 1 || got.Diagnostics.SourceSelectionMS < 0 {
		t.Errorf("Diagnostics = %#v", got.Diagnostics)
	}
	if !reflect.DeepEqual(selector.requests, []SelectionRequest{{Query: "topic", Candidates: []SelectionCandidate{
		{ID: "candidate-1", Title: "Rejected", Description: "Low relevance", URL: "https://example.com/rejected"},
		{ID: "candidate-2", Title: "Selected", Description: "Strong evidence", URL: "https://example.com/selected"},
	}}}) {
		t.Errorf("selection requests = %#v", selector.requests)
	}
	if !reflect.DeepEqual(extractor.urls, []string{"https://example.com/selected"}) {
		t.Errorf("extractions = %#v", extractor.urls)
	}
}

func TestServiceExtractsSelectedCandidatesInSelectorOrder(t *testing.T) {
	searcher := &recordingSearcher{results: map[string][]searchDomain.RawResult{
		"query": {
			{"href": "https://example.com/first", "title": "First"},
			{"href": "https://example.com/second", "title": "Second"},
		},
	}}
	selector := &recordingSelector{selection: Selection{CandidateIDs: []string{"candidate-2", "candidate-1"}}}
	extractor := &orderedExtractor{results: map[string]extractAIDomain.Result{
		"https://example.com/first":  {URL: "https://example.com/first", Content: "<p>First evidence</p>"},
		"https://example.com/second": {URL: "https://example.com/second", Content: "<p>Second evidence</p>"},
	}}
	service := newResearchService(
		&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "query"}}},
		selector,
		searcher,
		extractor,
		&recordingReporter{result: Report{HTML: "<p>Report</p>", SourceIDs: []string{"source-1", "source-2"}}},
		withSelectionLimits(2, 2, 1),
	)

	got, err := service.Research(context.Background(), domain.Request{Query: "topic", QueryCount: intPointer(1), ResultsPerQuery: intPointer(2)})
	if err != nil {
		t.Fatalf("Research() error = %v", err)
	}
	if want := []string{"https://example.com/second", "https://example.com/first"}; !reflect.DeepEqual(extractor.urls, want) {
		t.Errorf("extractions = %#v, want %#v", extractor.urls, want)
	}
	if want := []domain.Source{{URL: "https://example.com/second", Title: "Second"}, {URL: "https://example.com/first", Title: "First"}}; !reflect.DeepEqual(got.Sources, want) {
		t.Errorf("Sources = %#v, want %#v", got.Sources, want)
	}
}

func TestServiceRejectsInvalidSelectionsBeforeExtraction(t *testing.T) {
	tests := []struct {
		name      string
		selection Selection
		wantErr   error
	}{
		{name: "empty", selection: Selection{}, wantErr: domain.ErrInvalidResponse},
		{name: "unknown", selection: Selection{CandidateIDs: []string{"unknown"}}, wantErr: domain.ErrInvalidResponse},
		{name: "duplicate", selection: Selection{CandidateIDs: []string{"candidate-1", "candidate-1"}}, wantErr: domain.ErrInvalidResponse},
		{name: "over budget", selection: Selection{CandidateIDs: []string{"candidate-1", "candidate-2"}}, wantErr: domain.ErrInvalidResponse},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			extractor := &recordingExtractor{}
			service := newResearchService(
				&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "query"}}},
				&recordingSelector{selection: testCase.selection},
				&recordingSearcher{results: map[string][]searchDomain.RawResult{"query": {{"href": "https://example.com/one"}, {"href": "https://example.com/two"}}}},
				extractor,
				&recordingReporter{},
				withSelectionLimits(2, 1, 1),
			)
			_, err := service.Research(context.Background(), domain.Request{Query: "topic", QueryCount: intPointer(1), ResultsPerQuery: intPointer(2)})
			if !errors.Is(err, testCase.wantErr) {
				t.Errorf("Research() error = %v, want %v", err, testCase.wantErr)
			}
			if len(extractor.urls) != 0 {
				t.Errorf("extractions = %#v, want none", extractor.urls)
			}
		})
	}
}

func TestServicePropagatesSelectorFailuresBeforeExtraction(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "canceled", err: context.Canceled},
		{name: "rate limited", err: extractAIDomain.ErrRateLimited},
		{name: "failed", err: errors.New("selector failed")},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			extractor := &recordingExtractor{}
			service := newResearchService(
				&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "query"}}},
				&recordingSelector{err: testCase.err},
				&recordingSearcher{results: map[string][]searchDomain.RawResult{"query": {{"href": "https://example.com/one"}}}},
				extractor,
				&recordingReporter{},
			)

			_, err := service.Research(context.Background(), domain.Request{Query: "topic", QueryCount: intPointer(1), ResultsPerQuery: intPointer(1)})
			if !errors.Is(err, testCase.err) {
				t.Errorf("Research() error = %v, want %v", err, testCase.err)
			}
			if len(extractor.urls) != 0 {
				t.Errorf("extractions = %#v, want none", extractor.urls)
			}
		})
	}
}

func TestServiceDoesNotStartSelectionAfterSearchCancelsTheRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	selector := &recordingSelector{}
	extractor := &recordingExtractor{}
	service := newResearchService(
		&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "query"}}},
		selector,
		&recordingSearcher{
			results:  map[string][]searchDomain.RawResult{"query": {{"href": "https://example.com/source"}}},
			onSearch: cancel,
		},
		extractor,
		&recordingReporter{},
	)

	_, err := service.Research(ctx, domain.Request{Query: "topic", QueryCount: intPointer(1), ResultsPerQuery: intPointer(1)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Research() error = %v, want context.Canceled", err)
	}
	if len(selector.requests) != 0 {
		t.Errorf("selection requests = %#v, want none", selector.requests)
	}
	if len(extractor.urls) != 0 {
		t.Errorf("extractions = %#v, want none", extractor.urls)
	}
}

func TestServiceDoesNotStartSearchAfterPlanningCancelsTheRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	searcher := &recordingSearcher{}
	recorder := &recordingStepRecorder{}
	service := NewService(
		&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "query"}}, onPlan: cancel},
		&recordingSelector{},
		searcher,
		&recordingExtractor{},
		&recordingReporter{},
		withSelectionLimits(100, 100, 20),
		recorder,
	)

	_, err := service.Research(ctx, domain.Request{Query: "topic", QueryCount: intPointer(1), ResultsPerQuery: intPointer(1)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Research() error = %v, want context.Canceled", err)
	}
	if len(searcher.requests) != 0 {
		t.Errorf("search requests = %#v, want none", searcher.requests)
	}
	if _, started := recorder.stepStart(operations.StepResearchSearch); started {
		t.Errorf("started steps = %#v, want no research search step", recorder.starts)
	}
}

func TestServiceDoesNotStartExtractionAfterSelectionCancelsTheRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	selector := &recordingSelector{selection: Selection{CandidateIDs: []string{"candidate-1"}}, onSelect: cancel}
	extractor := &recordingExtractor{}
	recorder := &recordingStepRecorder{}
	service := NewService(
		&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "query"}}},
		selector,
		&recordingSearcher{results: map[string][]searchDomain.RawResult{"query": {{"href": "https://example.com/source"}}}},
		extractor,
		&recordingReporter{},
		withSelectionLimits(100, 100, 20),
		recorder,
	)

	_, err := service.Research(ctx, domain.Request{Query: "topic", QueryCount: intPointer(1), ResultsPerQuery: intPointer(1)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Research() error = %v, want context.Canceled", err)
	}
	if len(extractor.urls) != 0 {
		t.Errorf("extractions = %#v, want none", extractor.urls)
	}
	if _, started := recorder.stepStart(operations.StepResearchExtract); started {
		t.Errorf("started steps = %#v, want no research extraction step", recorder.starts)
	}
}

func TestServiceDeduplicatesFinalURLsAfterSelectedExtraction(t *testing.T) {
	service := newResearchService(
		&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "query"}}},
		&recordingSelector{},
		&recordingSearcher{results: map[string][]searchDomain.RawResult{"query": {
			{"href": "https://example.com/first", "title": "First"},
			{"href": "https://example.com/second", "title": "Second"},
		}}},
		&recordingExtractor{results: map[string]extractAIDomain.Result{
			"https://example.com/first":  {URL: "https://example.com/canonical", Content: "<p>Evidence</p>"},
			"https://example.com/second": {URL: "https://example.com/canonical", Content: "<p>Duplicate evidence</p>"},
		}},
		&recordingReporter{result: Report{HTML: "<p>Report</p>", SourceIDs: []string{"source-1"}}},
		withSelectionLimits(2, 2, 1),
	)

	got, err := service.Research(context.Background(), domain.Request{Query: "topic", QueryCount: intPointer(1), ResultsPerQuery: intPointer(2)})
	if err != nil {
		t.Fatalf("Research() error = %v", err)
	}
	if want := []domain.Source{{URL: "https://example.com/canonical", Title: "First"}}; !reflect.DeepEqual(got.Sources, want) {
		t.Errorf("Sources = %#v, want %#v", got.Sources, want)
	}
}

func TestServiceRecordsOnlySelectionCounts(t *testing.T) {
	recorder := &recordingStepRecorder{}
	service := NewService(
		&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "query"}}},
		&recordingSelector{selection: Selection{CandidateIDs: []string{"unknown"}}},
		&recordingSearcher{results: map[string][]searchDomain.RawResult{"query": {
			{"href": "https://example.com/first", "title": "First", "description": "First description"},
			{"href": "https://example.com/second", "title": "Second", "description": "Second description"},
		}}},
		&recordingExtractor{},
		&recordingReporter{},
		withSelectionLimits(2, 1, 1),
		recorder,
	)

	_, err := service.Research(context.Background(), domain.Request{Query: "private topic", QueryCount: intPointer(1), ResultsPerQuery: intPointer(2)})
	if !errors.Is(err, domain.ErrInvalidResponse) {
		t.Fatalf("Research() error = %v, want ErrInvalidResponse", err)
	}
	start, started := recorder.stepStart(operations.StepResearchSelection)
	finish, finished := recorder.stepFinish(operations.StepResearchSelection)
	if !started || !finished {
		t.Fatalf("selection step start = %v, finish = %v", started, finished)
	}
	if want := map[string]string{"candidates_found": "2", "candidates_submitted": "2", "candidates_selected": "0"}; !reflect.DeepEqual(start.Metadata, want) {
		t.Errorf("selection start metadata = %#v, want %#v", start.Metadata, want)
	}
	if want := map[string]string{"candidates_found": "2", "candidates_submitted": "2", "candidates_selected": "0"}; !reflect.DeepEqual(finish.Metadata, want) {
		t.Errorf("selection finish metadata = %#v, want %#v", finish.Metadata, want)
	}
}

func TestServiceRejectsInvalidLimitsWithoutStartingWorkers(t *testing.T) {
	service := NewService(
		&recordingPlanner{},
		&recordingSelector{},
		&recordingSearcher{},
		&recordingExtractor{},
		&recordingReporter{},
		Limits{},
	)
	_, err := service.Research(context.Background(), domain.Request{Query: "topic"})
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("Research() error = %v, want ErrUnavailable", err)
	}
}

func TestServiceSkipsFailedOrUnusableExtractions(t *testing.T) {
	searcher := &recordingSearcher{results: map[string][]searchDomain.RawResult{
		"query": {
			{"href": "javascript:alert(1)"},
			{"href": "https://example.com/failed"},
			{"href": "https://example.com/empty"},
			{"href": "https://example.com/usable", "title": "Usable"},
		},
	}}
	extractor := &recordingExtractor{
		results: map[string]extractAIDomain.Result{
			"https://example.com/empty":  {URL: "https://example.com/empty"},
			"https://example.com/usable": {URL: "https://example.com/usable", Content: "<p>Useful evidence.</p>"},
		},
		errors: map[string]error{"https://example.com/failed": errors.New("blocked")},
	}
	reporter := &recordingReporter{result: Report{HTML: "<p>Result</p>", SourceIDs: []string{"source-1"}}}
	service := newResearchService(&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "query"}}}, &recordingSelector{}, searcher, extractor, reporter)

	got, err := service.Research(context.Background(), domain.Request{Query: "topic", QueryCount: intPointer(1), ResultsPerQuery: intPointer(4)})
	if err != nil {
		t.Fatalf("Research() error = %v", err)
	}
	if !reflect.DeepEqual(got.Sources, []domain.Source{{URL: "https://example.com/usable", Title: "Usable"}}) {
		t.Errorf("Sources = %#v", got.Sources)
	}
	if len(reporter.requests) != 1 || len(reporter.requests[0].Sources) != 1 {
		t.Errorf("report requests = %#v", reporter.requests)
	}
}

func TestServiceExtractsAtMostTheRequestedResultsForEachGeneratedQuery(t *testing.T) {
	searcher := &recordingSearcher{results: map[string][]searchDomain.RawResult{
		"first query": {
			{"href": "https://example.com/one"},
			{"href": "https://example.com/two"},
		},
		"second query": {
			{"href": "https://example.com/three"},
		},
	}}
	extractor := &recordingExtractor{results: map[string]extractAIDomain.Result{
		"https://example.com/one":   {URL: "https://example.com/one", Content: "<p>one</p>"},
		"https://example.com/two":   {URL: "https://example.com/two", Content: "<p>two</p>"},
		"https://example.com/three": {URL: "https://example.com/three", Content: "<p>three</p>"},
	}}
	service := newResearchService(
		&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "first query"}, {Language: "en", Query: "second query"}}},
		&recordingSelector{},
		searcher,
		extractor,
		&recordingReporter{result: Report{HTML: "<p>Report</p>", SourceIDs: []string{"source-1"}}},
	)

	_, err := service.Research(context.Background(), domain.Request{Query: "topic", QueryCount: intPointer(2), ResultsPerQuery: intPointer(1)})
	if err != nil {
		t.Fatalf("Research() error = %v", err)
	}
	if !sameStrings(extractor.urls, []string{"https://example.com/one", "https://example.com/three"}) {
		t.Errorf("extractions = %#v", extractor.urls)
	}
}

func TestServiceLimitsConcurrentExtractionsAndPreservesSourceOrder(t *testing.T) {
	const maxConcurrentExtractions = 20
	const candidates = maxConcurrentExtractions + 2
	results := make([]searchDomain.RawResult, candidates)
	for index := range results {
		results[index] = searchDomain.RawResult{"href": "https://example.com/source-" + string(rune('a'+index))}
	}
	extractor := newBlockingExtractor(candidates)
	reporter := &recordingReporter{result: Report{HTML: "<p>Report</p>", SourceIDs: []string{"source-1"}}}
	service := newResearchService(
		&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "query"}}},
		&recordingSelector{},
		&recordingSearcher{results: map[string][]searchDomain.RawResult{"query": results}},
		extractor,
		reporter,
		withSelectionLimits(candidates, candidates, maxConcurrentExtractions),
	)

	done := make(chan error, 1)
	go func() {
		_, err := service.Research(context.Background(), domain.Request{Query: "topic", QueryCount: intPointer(1), ResultsPerQuery: intPointer(candidates)})
		done <- err
	}()

	for range maxConcurrentExtractions {
		select {
		case <-extractor.started:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for extraction workers")
		}
	}
	if got := extractor.maxActive.Load(); got != maxConcurrentExtractions {
		t.Errorf("maximum concurrent extractions = %d, want %d", got, maxConcurrentExtractions)
	}

	close(extractor.release)
	if err := <-done; err != nil {
		t.Fatalf("Research() error = %v", err)
	}
	if sources := reporter.requests[0].Sources; len(sources) != candidates || sources[0].ID != "source-1" || sources[0].URL != "https://example.com/source-a" || sources[candidates-1].ID != "source-22" {
		t.Errorf("report sources = %#v", sources)
	}
}

func TestServicePropagatesCancellationBeforeWritingTheReport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reporter := &recordingReporter{result: Report{HTML: "<p>Report</p>", SourceIDs: []string{"source-1"}}}
	service := newResearchService(
		&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "query"}}},
		&recordingSelector{},
		&recordingSearcher{results: map[string][]searchDomain.RawResult{"query": {{"href": "https://example.com/source"}}}},
		&recordingExtractor{
			results:   map[string]extractAIDomain.Result{"https://example.com/source": {URL: "https://example.com/source", Content: "<p>Evidence</p>"}},
			onExtract: cancel,
		},
		reporter,
	)

	_, err := service.Research(ctx, domain.Request{Query: "topic", QueryCount: intPointer(1), ResultsPerQuery: intPointer(1)})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Research() error = %v, want context.Canceled", err)
	}
	if len(reporter.requests) != 0 {
		t.Errorf("reporter calls = %d, want 0", len(reporter.requests))
	}
}

func TestServiceRejectsInvalidModelOutputAndFailsWithoutUsableSources(t *testing.T) {
	tests := []struct {
		name    string
		planner *recordingPlanner
		search  *recordingSearcher
		report  *recordingReporter
		wantErr error
	}{
		{
			name:    "planner produces an unrequested language",
			planner: &recordingPlanner{queries: []domain.GeneratedQuery{{Language: "es", Query: "consulta"}}},
			search:  &recordingSearcher{},
			report:  &recordingReporter{},
			wantErr: domain.ErrInvalidResponse,
		},
		{
			name:    "no usable source",
			planner: &recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "query"}}},
			search:  &recordingSearcher{results: map[string][]searchDomain.RawResult{"query": {{"href": "https://failed.example"}}}},
			report:  &recordingReporter{},
			wantErr: domain.ErrNoUsableSources,
		},
		{
			name:    "report selects unknown source",
			planner: &recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "query"}}},
			search:  &recordingSearcher{results: map[string][]searchDomain.RawResult{"query": {{"href": "https://example.com", "title": "Source"}}}},
			report:  &recordingReporter{result: Report{HTML: "<p>Report</p>", SourceIDs: []string{"unknown"}}},
			wantErr: domain.ErrInvalidResponse,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			extractor := &recordingExtractor{results: map[string]extractAIDomain.Result{"https://example.com": {URL: "https://example.com", Content: "<p>Source</p>"}}}
			service := newResearchService(testCase.planner, &recordingSelector{}, testCase.search, extractor, testCase.report)
			_, err := service.Research(context.Background(), domain.Request{Query: "topic", QueryCount: intPointer(1), ResultsPerQuery: intPointer(1)})
			if !errors.Is(err, testCase.wantErr) {
				t.Errorf("Research() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

type recordingPlanner struct {
	queries []domain.GeneratedQuery
	err     error
	onPlan  func()
}

type recordingSelector struct {
	requests  []SelectionRequest
	selection Selection
	err       error
	onSelect  func()
}

func (s *recordingSelector) Select(_ context.Context, request SelectionRequest) (Selection, error) {
	s.requests = append(s.requests, request)
	if s.onSelect != nil {
		s.onSelect()
	}
	if s.selection.CandidateIDs == nil && s.err == nil {
		selection := Selection{CandidateIDs: make([]string, len(request.Candidates))}
		for index, candidate := range request.Candidates {
			selection.CandidateIDs[index] = candidate.ID
		}
		return selection, nil
	}
	return s.selection, s.err
}

func newResearchService(planner Planner, selector Selector, searcher Searcher, extractor Extractor, reporter Reporter, limits ...Limits) Service {
	selectedLimits := withSelectionLimits(100, 100, 20)
	if len(limits) > 0 {
		selectedLimits = limits[0]
	}
	return NewService(planner, selector, searcher, extractor, reporter, selectedLimits)
}

func withSelectionLimits(candidates, sources, extractions int) Limits {
	return Limits{MaxSelectionCandidates: candidates, MaxSelectedSources: sources, MaxConcurrentExtractions: extractions}
}

func (p *recordingPlanner) Plan(context.Context, domain.NormalizedRequest) ([]domain.GeneratedQuery, error) {
	if p.onPlan != nil {
		p.onPlan()
	}
	return p.queries, p.err
}

func intPointer(value int) *int {
	return &value
}

type recordingSearcher struct {
	requests    []searchDomain.SearchRequest
	results     map[string][]searchDomain.RawResult
	diagnostics map[string][]searchDomain.SearchDiagnostic
	err         error
	onSearch    func()
}

func (s *recordingSearcher) Search(_ context.Context, request searchDomain.SearchRequest) ([]searchDomain.RawResult, error) {
	s.requests = append(s.requests, request)
	if request.Diagnostics != nil && request.Diagnostics.OnComplete != nil {
		for _, diagnostic := range s.diagnostics[request.Query] {
			request.Diagnostics.OnComplete(diagnostic)
		}
	}
	if s.onSearch != nil {
		s.onSearch()
	}
	return s.results[request.Query], s.err
}

type recordingExtractor struct {
	mu        sync.Mutex
	urls      []string
	results   map[string]extractAIDomain.Result
	errors    map[string]error
	onExtract func()
}

type orderedExtractor struct {
	urls    []string
	results map[string]extractAIDomain.Result
}

func (e *orderedExtractor) Extract(_ context.Context, request extractAIDomain.Request) (extractAIDomain.Result, error) {
	e.urls = append(e.urls, request.URL)
	return e.results[request.URL], nil
}

func (e *recordingExtractor) Extract(_ context.Context, request extractAIDomain.Request) (extractAIDomain.Result, error) {
	e.mu.Lock()
	e.urls = append(e.urls, request.URL)
	e.mu.Unlock()
	if e.onExtract != nil {
		e.onExtract()
	}
	return e.results[request.URL], e.errors[request.URL]
}

type recordingReporter struct {
	requests []ReportRequest
	result   Report
	err      error
}

type recordingStepRecorder struct {
	starts   []operations.StepStart
	finishes []operations.Step
}

func (r *recordingStepRecorder) StartStep(_ context.Context, start operations.StepStart) (operations.Step, error) {
	r.starts = append(r.starts, start)
	return operations.Step{ID: "step", Type: start.Type, Metadata: start.Metadata}, nil
}

func (r *recordingStepRecorder) FinishStep(_ context.Context, step operations.Step, _ error) error {
	r.finishes = append(r.finishes, step)
	return nil
}

func (r *recordingStepRecorder) stepStart(stepType operations.StepType) (operations.StepStart, bool) {
	for _, start := range r.starts {
		if start.Type == stepType {
			return start, true
		}
	}
	return operations.StepStart{}, false
}

func (r *recordingStepRecorder) stepFinish(stepType operations.StepType) (operations.Step, bool) {
	for _, step := range r.finishes {
		if step.Type == stepType {
			return step, true
		}
	}
	return operations.Step{}, false
}

func (r *recordingReporter) Write(_ context.Context, request ReportRequest) (Report, error) {
	r.requests = append(r.requests, request)
	return r.result, r.err
}

type blockingExtractor struct {
	started   chan struct{}
	release   chan struct{}
	active    atomic.Int32
	maxActive atomic.Int32
}

func newBlockingExtractor(candidateCount int) *blockingExtractor {
	return &blockingExtractor{started: make(chan struct{}, candidateCount), release: make(chan struct{})}
}

func (e *blockingExtractor) Extract(_ context.Context, request extractAIDomain.Request) (extractAIDomain.Result, error) {
	active := e.active.Add(1)
	for {
		maximum := e.maxActive.Load()
		if active <= maximum || e.maxActive.CompareAndSwap(maximum, active) {
			break
		}
	}
	e.started <- struct{}{}
	<-e.release
	e.active.Add(-1)
	return extractAIDomain.Result{URL: request.URL, Content: "<p>Evidence</p>"}, nil
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	got = append([]string(nil), got...)
	want = append([]string(nil), want...)
	sort.Strings(got)
	sort.Strings(want)
	return reflect.DeepEqual(got, want)
}
