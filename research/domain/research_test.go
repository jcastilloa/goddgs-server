package domain

import (
	"errors"
	"reflect"
	"testing"
)

func TestRequestNormalizeAppliesDefaultsAndDerivesRegions(t *testing.T) {
	request, err := (Request{Query: " E.T. opening box office "}).Normalize()
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	want := NormalizedRequest{
		Query:           "E.T. opening box office",
		ReportLanguage:  "en",
		QueryLanguages:  []string{"en"},
		QueryCount:      10,
		ResultsPerQuery: 10,
		Region:          "",
	}
	if !reflect.DeepEqual(request, want) {
		t.Errorf("Normalize() = %#v, want %#v", request, want)
	}
	if region, err := request.RegionFor("en"); err != nil || region != "us-en" {
		t.Errorf("RegionFor(en) = (%q, %v), want (us-en, nil)", region, err)
	}
	if region, err := request.RegionFor("es"); err != nil || region != "es-es" {
		t.Errorf("RegionFor(es) = (%q, %v), want (es-es, nil)", region, err)
	}
}

func TestRequestNormalizeValidatesResearchParameters(t *testing.T) {
	tests := []struct {
		name    string
		request Request
	}{
		{name: "empty query", request: Request{}},
		{name: "invalid report language", request: Request{Query: "topic", ReportLanguage: "english"}},
		{name: "empty query languages", request: Request{Query: "topic", QueryLanguages: []string{}}},
		{name: "duplicate query languages", request: Request{Query: "topic", QueryLanguages: []string{"en", "EN"}}},
		{name: "invalid query language", request: Request{Query: "topic", QueryLanguages: []string{"eng"}}},
		{name: "non ASCII query language", request: Request{Query: "topic", QueryLanguages: []string{"ñá"}}},
		{name: "zero query count", request: Request{Query: "topic", QueryCount: intPointer(0), ResultsPerQuery: intPointer(1), QueryLanguages: []string{"en"}}},
		{name: "negative results per query", request: Request{Query: "topic", QueryCount: intPointer(1), ResultsPerQuery: intPointer(-1), QueryLanguages: []string{"en"}}},
		{name: "unsupported implicit region", request: Request{Query: "topic", QueryLanguages: []string{"zz"}, QueryCount: intPointer(1), ResultsPerQuery: intPointer(1)}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := testCase.request.Normalize()
			if !errors.Is(err, ErrInvalidRequest) {
				t.Errorf("Normalize() error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestRequestNormalizeAllowsAnyTwoLetterLanguageWithExplicitRegion(t *testing.T) {
	request, err := (Request{
		Query:           "topic",
		ReportLanguage:  "zz",
		QueryLanguages:  []string{"zz"},
		QueryCount:      intPointer(1),
		ResultsPerQuery: intPointer(1),
		Region:          "us-en",
	}).Normalize()
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if region, err := request.RegionFor("zz"); err != nil || region != "us-en" {
		t.Errorf("RegionFor(zz) = (%q, %v), want (us-en, nil)", region, err)
	}
}

func intPointer(value int) *int {
	return &value
}
