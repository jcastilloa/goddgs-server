package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	defaultReportLanguage  = "en"
	defaultQueryCount      = 10
	defaultResultsPerQuery = 10
)

var (
	ErrInvalidRequest  = errors.New("invalid research request")
	ErrInvalidResponse = errors.New("invalid research response")
	ErrNoUsableSources = errors.New("no usable research sources")
	ErrUnavailable     = errors.New("research is unavailable")
)

type Request struct {
	Query           string   `json:"query"`
	ReportLanguage  string   `json:"report_language"`
	QueryLanguages  []string `json:"query_languages"`
	QueryCount      *int     `json:"query_count"`
	ResultsPerQuery *int     `json:"results_per_query"`
	Region          string   `json:"region"`
}

type NormalizedRequest struct {
	Query           string
	ReportLanguage  string
	QueryLanguages  []string
	QueryCount      int
	ResultsPerQuery int
	Region          string
}

type GeneratedQuery struct {
	Language string `json:"language"`
	Query    string `json:"query"`
}

type Source struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

type Result struct {
	ReportHTML  string      `json:"report_html"`
	Sources     []Source    `json:"sources"`
	Diagnostics Diagnostics `json:"diagnostics"`
}

type Diagnostics struct {
	Backends           []BackendDiagnostic `json:"backends"`
	QueryPlanningMS    int64               `json:"query_planning_ms"`
	SearchMS           int64               `json:"search_ms"`
	SourceSelectionMS  int64               `json:"source_selection_ms"`
	SourceExtractionMS int64               `json:"source_extraction_ms"`
	ReportGenerationMS int64               `json:"report_generation_ms"`
	TotalMS            int64               `json:"total_ms"`
	CandidatesFound    int                 `json:"candidates_found"`
	CandidatesSelected int                 `json:"candidates_selected"`
}

type BackendDiagnostic struct {
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	Attempts    int    `json:"attempts"`
	ResultCount int    `json:"result_count"`
	ErrorCount  int    `json:"error_count"`
}

func (d *Diagnostics) SortBackends() {
	sort.Slice(d.Backends, func(left, right int) bool {
		return d.Backends[left].Name < d.Backends[right].Name
	})
}

func (r Request) Normalize() (NormalizedRequest, error) {
	request := NormalizedRequest{
		Query:           strings.TrimSpace(r.Query),
		ReportLanguage:  normalizeLanguage(r.ReportLanguage),
		QueryLanguages:  normalizeLanguages(r.QueryLanguages),
		QueryCount:      defaultInt(r.QueryCount, defaultQueryCount),
		ResultsPerQuery: defaultInt(r.ResultsPerQuery, defaultResultsPerQuery),
		Region:          strings.TrimSpace(r.Region),
	}
	if request.ReportLanguage == "" {
		request.ReportLanguage = defaultReportLanguage
	}
	if len(request.QueryLanguages) == 0 && r.QueryLanguages == nil {
		request.QueryLanguages = []string{defaultReportLanguage}
	}
	if err := request.Validate(); err != nil {
		return NormalizedRequest{}, err
	}
	return request, nil
}

func (r NormalizedRequest) Validate() error {
	if r.Query == "" {
		return fmt.Errorf("%w: query is required", ErrInvalidRequest)
	}
	if !isLanguage(r.ReportLanguage) {
		return fmt.Errorf("%w: report_language must be an ISO 639-1 code", ErrInvalidRequest)
	}
	if len(r.QueryLanguages) == 0 {
		return fmt.Errorf("%w: query_languages must contain at least one ISO 639-1 code", ErrInvalidRequest)
	}
	seenLanguages := make(map[string]struct{}, len(r.QueryLanguages))
	for _, language := range r.QueryLanguages {
		if !isLanguage(language) {
			return fmt.Errorf("%w: query_languages must contain ISO 639-1 codes", ErrInvalidRequest)
		}
		if _, exists := seenLanguages[language]; exists {
			return fmt.Errorf("%w: query_languages must not contain duplicates", ErrInvalidRequest)
		}
		seenLanguages[language] = struct{}{}
		if r.Region == "" && defaultRegion(language) == "" {
			return fmt.Errorf("%w: region is required for query language %q", ErrInvalidRequest, language)
		}
	}
	if r.QueryCount <= 0 {
		return fmt.Errorf("%w: query_count must be a positive integer", ErrInvalidRequest)
	}
	if r.QueryCount < len(r.QueryLanguages) {
		return fmt.Errorf("%w: query_count must be at least the number of query_languages", ErrInvalidRequest)
	}
	if r.ResultsPerQuery <= 0 {
		return fmt.Errorf("%w: results_per_query must be a positive integer", ErrInvalidRequest)
	}
	return nil
}

func (r NormalizedRequest) RegionFor(language string) (string, error) {
	if r.Region != "" {
		return r.Region, nil
	}
	region := defaultRegion(normalizeLanguage(language))
	if region == "" {
		return "", fmt.Errorf("%w: region is required for query language %q", ErrInvalidRequest, language)
	}
	return region, nil
}

func defaultInt(value *int, defaultValue int) int {
	if value == nil {
		return defaultValue
	}
	return *value
}

func normalizeLanguages(languages []string) []string {
	if languages == nil {
		return nil
	}
	normalized := make([]string, len(languages))
	for index, language := range languages {
		normalized[index] = normalizeLanguage(language)
	}
	return normalized
}

func normalizeLanguage(language string) string {
	return strings.ToLower(strings.TrimSpace(language))
}

func isLanguage(language string) bool {
	return len(language) == 2 && language[0] >= 'a' && language[0] <= 'z' && language[1] >= 'a' && language[1] <= 'z'
}

func defaultRegion(language string) string {
	switch language {
	case "en":
		return "us-en"
	case "es":
		return "es-es"
	default:
		return ""
	}
}
