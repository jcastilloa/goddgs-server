package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	configDomain "github.com/jcastilloa/goddgs-server/shared/config/domain"
	extractAIDomain "github.com/jcastilloa/goddgs-server/shared/extractai/domain"
)

func TestExtractClientSendsConfiguredChatCompletionRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", request.URL.Path)
		}
		if authorization := request.Header.Get("Authorization"); authorization != "Bearer test-key" {
			t.Errorf("Authorization = %q", authorization)
		}
		if value := request.Header.Get("X-Client-Name"); value != "goddgs-server" {
			t.Errorf("X-Client-Name = %q", value)
		}

		var body struct {
			Model       string  `json:"model"`
			Temperature float64 `json:"temperature"`
			Messages    []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Model != "gpt-4.1-mini" || body.Temperature != 0.15 {
			t.Errorf("request body = %#v", body)
		}
		if len(body.Messages) != 2 || body.Messages[0].Role != "system" || body.Messages[1].Content != "source document" {
			t.Errorf("messages = %#v", body.Messages)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"<article><p>Clean</p></article>"}}]}`))
	}))
	defer server.Close()

	client := newExtractClient(t, server.URL, 0, nil)
	got, err := client.Complete(context.Background(), "system instructions", "source document")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if got != "<article><p>Clean</p></article>" {
		t.Errorf("Complete() = %q", got)
	}
}

func TestExtractClientRetriesTransientCompatibleAPIResponses(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			writer.WriteHeader(http.StatusTooManyRequests)
			_, _ = writer.Write([]byte(`{"error":{"message":"slow down"}}`))
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":[{"type":"text","text":"<p>Clean</p>"}]}}]}`))
	}))
	defer server.Close()

	client := newExtractClient(t, server.URL, 1, func(context.Context, time.Duration) error { return nil })
	got, err := client.Complete(context.Background(), "system", "source")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
	if got != "<p>Clean</p>" {
		t.Errorf("Complete() = %q", got)
	}
}

func TestExtractClientClassifiesRateLimitsAndInvalidResponses(t *testing.T) {
	tests := []struct {
		name     string
		response string
		status   int
		wantErr  error
	}{
		{name: "rate limited", status: http.StatusTooManyRequests, response: `{"error":{"message":"slow down"}}`, wantErr: extractAIDomain.ErrRateLimited},
		{name: "missing choice", status: http.StatusOK, response: `{"choices":[]}`, wantErr: extractAIDomain.ErrInvalidResponse},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(testCase.status)
				_, _ = writer.Write([]byte(testCase.response))
			}))
			defer server.Close()

			client := newExtractClient(t, server.URL, 0, nil)
			_, err := client.Complete(context.Background(), "system", "source")
			if !errors.Is(err, testCase.wantErr) {
				t.Errorf("Complete() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestExtractClientAcceptsAnEmptyCompletionAsNoEditorialContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":""}}]}`))
	}))
	defer server.Close()

	client := newExtractClient(t, server.URL, 0, nil)
	got, err := client.Complete(context.Background(), "system", "source")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if got != "" {
		t.Errorf("Complete() = %q, want empty string", got)
	}
}

func TestExtractClientRetriesRetryableServerErrors(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"<p>Clean</p>"}}]}`))
	}))
	defer server.Close()

	client := newExtractClient(t, server.URL, 1, func(context.Context, time.Duration) error { return nil })
	_, err := client.Complete(context.Background(), "system", "source")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
}

func newExtractClient(t *testing.T, baseURL string, retries int, wait retryWait) *ExtractClient {
	t.Helper()
	client, err := newClient(
		configDomain.LLMConfig{BaseURL: baseURL, APIKey: "test-key", Headers: map[string]string{"X-Client-Name": "goddgs-server"}},
		configDomain.ExtractAIConfig{Model: "gpt-4.1-mini", Timeout: time.Second, Temperature: 0.15, Retries: retries},
		&http.Client{},
		wait,
	)
	if err != nil {
		t.Fatalf("newClient() error = %v", err)
	}
	return client
}
