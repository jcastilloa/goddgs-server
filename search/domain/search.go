package domain

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	ErrInvalidSearchRequest  = errors.New("invalid search request")
	ErrInvalidExtractRequest = errors.New("invalid extract request")
	ErrRateLimited           = errors.New("search rate limited")
	ErrSearchTimeout         = errors.New("search timed out")
)

type Category string

const (
	CategoryText   Category = "text"
	CategoryImages Category = "images"
	CategoryNews   Category = "news"
	CategoryVideos Category = "videos"
	CategoryBooks  Category = "books"
)

type RawResult map[string]any

type SearchRequest struct {
	Category    Category
	Query       string
	Region      string
	SafeSearch  string
	TimeLimit   string
	MaxResults  *int
	Page        *int
	Backend     string
	Images      ImageOptions
	Videos      VideoOptions
	Diagnostics *SearchDiagnostics
}

type SearchDiagnostic struct {
	Backend     string
	Provider    string
	ResultCount int
	Err         error
}

type SearchDiagnostics struct {
	OnComplete func(SearchDiagnostic)
}

type ImageOptions struct {
	Size    string
	Color   string
	Type    string
	Layout  string
	License string
}

type VideoOptions struct {
	Resolution string
	Duration   string
	License    string
}

type ExtractRequest struct {
	URL    string
	Format string
	Mode   ExtractMode
}

type ExtractMode string

const (
	ExtractModeHeuristic ExtractMode = "heuristic"
	ExtractModeAI        ExtractMode = "ai"
)

type ExtractResult struct {
	URL     string
	Content any
}

func (r SearchRequest) Validate() error {
	if !r.Category.IsValid() {
		return fmt.Errorf("%w: unsupported category %q", ErrInvalidSearchRequest, r.Category)
	}
	if strings.TrimSpace(r.Query) == "" {
		return fmt.Errorf("%w: query is required", ErrInvalidSearchRequest)
	}
	return nil
}

func (r ExtractRequest) Validate() error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(r.URL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%w: URL is required", ErrInvalidExtractRequest)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: unsupported URL scheme", ErrInvalidExtractRequest)
	}
	if !r.Mode.IsValid() {
		return fmt.Errorf("%w: unsupported extract mode %q", ErrInvalidExtractRequest, r.Mode)
	}
	return nil
}

func (m ExtractMode) IsValid() bool {
	switch ExtractMode(strings.ToLower(strings.TrimSpace(string(m)))) {
	case "", ExtractModeHeuristic, ExtractModeAI:
		return true
	default:
		return false
	}
}

func (m ExtractMode) Normalize() ExtractMode {
	if ExtractMode(strings.ToLower(strings.TrimSpace(string(m)))) == ExtractModeAI {
		return ExtractModeAI
	}
	return ExtractModeHeuristic
}

func (c Category) IsValid() bool {
	switch c {
	case CategoryText, CategoryImages, CategoryNews, CategoryVideos, CategoryBooks:
		return true
	default:
		return false
	}
}
