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
	service := NewService(
		&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "E.T. release date"}, {Language: "es", Query: "fecha estreno ET"}}},
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
	service := NewService(&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "query"}}}, searcher, extractor, reporter)

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
	service := NewService(
		&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "first query"}, {Language: "en", Query: "second query"}}},
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
	const candidates = maxConcurrentExtractions + 2
	results := make([]searchDomain.RawResult, candidates)
	for index := range results {
		results[index] = searchDomain.RawResult{"href": "https://example.com/source-" + string(rune('a'+index))}
	}
	extractor := newBlockingExtractor(candidates)
	reporter := &recordingReporter{result: Report{HTML: "<p>Report</p>", SourceIDs: []string{"source-1"}}}
	service := NewService(
		&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "query"}}},
		&recordingSearcher{results: map[string][]searchDomain.RawResult{"query": results}},
		extractor,
		reporter,
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
	if sources := reporter.requests[0].Sources; len(sources) != candidates || sources[0].ID != "source-1" || sources[0].URL != "https://example.com/source-a" || sources[candidates-1].ID != "source-12" {
		t.Errorf("report sources = %#v", sources)
	}
}

func TestServicePropagatesCancellationBeforeWritingTheReport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reporter := &recordingReporter{result: Report{HTML: "<p>Report</p>", SourceIDs: []string{"source-1"}}}
	service := NewService(
		&recordingPlanner{queries: []domain.GeneratedQuery{{Language: "en", Query: "query"}}},
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
			service := NewService(testCase.planner, testCase.search, extractor, testCase.report)
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
}

func (p *recordingPlanner) Plan(context.Context, domain.NormalizedRequest) ([]domain.GeneratedQuery, error) {
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
}

func (s *recordingSearcher) Search(_ context.Context, request searchDomain.SearchRequest) ([]searchDomain.RawResult, error) {
	s.requests = append(s.requests, request)
	if request.Diagnostics != nil && request.Diagnostics.OnComplete != nil {
		for _, diagnostic := range s.diagnostics[request.Query] {
			request.Diagnostics.OnComplete(diagnostic)
		}
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
