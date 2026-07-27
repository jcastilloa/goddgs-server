package domain

import (
	"errors"
	"testing"
)

func TestSearchRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		request SearchRequest
		wantErr error
	}{
		{
			name:    "valid text search",
			request: SearchRequest{Category: CategoryText, Query: "metasearch"},
		},
		{
			name:    "empty query",
			request: SearchRequest{Category: CategoryText, Query: "  "},
			wantErr: ErrInvalidSearchRequest,
		},
		{
			name:    "unknown category",
			request: SearchRequest{Category: "unknown", Query: "metasearch"},
			wantErr: ErrInvalidSearchRequest,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.request.Validate()
			if !errors.Is(err, testCase.wantErr) {
				t.Errorf("Validate() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestExtractRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		request ExtractRequest
		wantErr error
	}{
		{
			name:    "HTTPS URL",
			request: ExtractRequest{URL: "https://example.com/article"},
		},
		{
			name:    "missing URL",
			request: ExtractRequest{},
			wantErr: ErrInvalidExtractRequest,
		},
		{
			name:    "unsupported scheme",
			request: ExtractRequest{URL: "file:///tmp/article.html"},
			wantErr: ErrInvalidExtractRequest,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.request.Validate()
			if !errors.Is(err, testCase.wantErr) {
				t.Errorf("Validate() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}
