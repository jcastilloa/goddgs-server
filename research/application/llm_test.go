package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jcastilloa/goddgs-server/research/domain"
)

func TestLLMPlannerDecodesStrictStructuredQueries(t *testing.T) {
	model := &recordingModel{response: "```json\n{\"queries\":[{\"language\":\"en\",\"query\":\"E.T. release date\"}]}\n```"}
	planner := NewLLMPlanner(model)

	queries, err := planner.Plan(context.Background(), domain.NormalizedRequest{Query: "E.T.", QueryCount: 1, QueryLanguages: []string{"en"}})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(queries) != 1 || queries[0].Query != "E.T. release date" {
		t.Errorf("queries = %#v", queries)
	}
	if !strings.Contains(model.systemPrompt, "Return only a JSON object") || !strings.Contains(model.userPrompt, "query_count: 1") {
		t.Errorf("prompts = (%q, %q)", model.systemPrompt, model.userPrompt)
	}
}

func TestLLMReporterRejectsInvalidJSONAndBuildResultSanitizesHTML(t *testing.T) {
	reporter := NewLLMReporter(&recordingModel{response: `{"html":"<article onclick=\"bad()\"><script>bad()</script><p>Evidence</p></article>","source_ids":["source-1"]}`})
	report, err := reporter.Write(context.Background(), ReportRequest{Query: "topic", Language: "en", Sources: []ReportSource{{ID: "source-1", URL: "https://example.com", Title: "Source", Content: "Evidence"}}})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	result, err := buildResult(report, []ReportSource{{ID: "source-1", URL: "https://example.com", Title: "Source"}})
	if err != nil {
		t.Fatalf("buildResult() error = %v", err)
	}
	if result.ReportHTML != "<article><p>Evidence</p></article>" {
		t.Errorf("ReportHTML = %q", result.ReportHTML)
	}

	_, err = NewLLMReporter(&recordingModel{response: `{"html":"<p>report</p>","source_ids":["source-1"],"extra":true}`}).Write(context.Background(), ReportRequest{})
	if !errors.Is(err, domain.ErrInvalidResponse) {
		t.Errorf("Write() error = %v, want ErrInvalidResponse", err)
	}
}

type recordingModel struct {
	response     string
	err          error
	systemPrompt string
	userPrompt   string
}

func (m *recordingModel) Complete(_ context.Context, systemPrompt, userPrompt string) (string, error) {
	m.systemPrompt = systemPrompt
	m.userPrompt = userPrompt
	return m.response, m.err
}
