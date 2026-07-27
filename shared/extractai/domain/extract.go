package domain

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	ErrInvalidRequest  = errors.New("invalid AI extraction request")
	ErrInvalidSource   = errors.New("invalid AI extraction source")
	ErrInvalidResponse = errors.New("invalid AI extraction response")
	ErrUnavailable     = errors.New("AI extraction is unavailable")
	ErrRateLimited     = errors.New("AI extraction rate limited")
)

type Request struct {
	URL string
}

type Page struct {
	URL  string
	HTML string
}

type Result struct {
	URL     string
	Content string
}

func (r Request) Validate() error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(r.URL))
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%w: URL is required", ErrInvalidRequest)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: unsupported URL scheme", ErrInvalidRequest)
	}
	return nil
}
