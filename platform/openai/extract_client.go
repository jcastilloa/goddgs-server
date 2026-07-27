package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	configDomain "github.com/jcastilloa/goddgs-server/shared/config/domain"
	extractAIDomain "github.com/jcastilloa/goddgs-server/shared/extractai/domain"
)

const (
	chatCompletionsPath = "/chat/completions"
	maxResponseBytes    = 4 << 20
	retryDelay          = 250 * time.Millisecond
)

type retryWait func(context.Context, time.Duration) error

type ExtractClient struct {
	baseURL     string
	apiKey      string
	headers     map[string]string
	model       string
	temperature float64
	timeout     time.Duration
	retries     int
	httpClient  *http.Client
	wait        retryWait
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(headers))
	for name, value := range headers {
		cloned[name] = value
	}
	return cloned
}

func NewCompatibleExtractClient(llm configDomain.LLMConfig, extractAI configDomain.ExtractAIConfig) (*ExtractClient, error) {
	return newClient(llm, extractAI, &http.Client{Timeout: extractAI.Timeout}, waitForRetry)
}

func newClient(llm configDomain.LLMConfig, extractAI configDomain.ExtractAIConfig, httpClient *http.Client, wait retryWait) (*ExtractClient, error) {
	if err := llm.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(llm.APIKey) == "" {
		return nil, fmt.Errorf("%w: LLM API key is required", extractAIDomain.ErrUnavailable)
	}
	if err := extractAI.Validate(); err != nil {
		return nil, err
	}
	if httpClient == nil {
		return nil, errors.New("OpenAI-compatible LLM HTTP client is required")
	}
	if wait == nil {
		wait = waitForRetry
	}

	return &ExtractClient{
		baseURL:     strings.TrimRight(strings.TrimSpace(llm.BaseURL), "/"),
		apiKey:      strings.TrimSpace(llm.APIKey),
		headers:     cloneHeaders(llm.Headers),
		model:       strings.TrimSpace(extractAI.Model),
		temperature: extractAI.Temperature,
		timeout:     extractAI.Timeout,
		retries:     extractAI.Retries,
		httpClient:  httpClient,
		wait:        wait,
	}, nil
}

func (c *ExtractClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	for attempt := 0; ; attempt++ {
		content, retryable, err := c.complete(ctx, systemPrompt, userPrompt)
		if err == nil {
			return content, nil
		}
		if !retryable || attempt >= c.retries {
			return "", err
		}
		if err := c.wait(ctx, retryDelay*time.Duration(1<<attempt)); err != nil {
			return "", err
		}
	}
}

func (c *ExtractClient) complete(ctx context.Context, systemPrompt, userPrompt string) (string, bool, error) {
	body, err := json.Marshal(chatCompletionRequest{
		Model:       c.model,
		Temperature: c.temperature,
		Messages: []chatCompletionMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	})
	if err != nil {
		return "", false, fmt.Errorf("marshal OpenAI-compatible LLM request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+chatCompletionsPath, bytes.NewReader(body))
	if err != nil {
		return "", false, fmt.Errorf("create OpenAI-compatible LLM request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")
	for name, value := range c.headers {
		request.Header.Set(name, value)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return "", false, ctx.Err()
		}
		return "", isTemporary(err), fmt.Errorf("call OpenAI-compatible LLM: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return "", false, fmt.Errorf("read OpenAI-compatible LLM response: %w", err)
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return "", true, fmt.Errorf("%w: %s", extractAIDomain.ErrRateLimited, responseError(responseBody))
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", retryableStatus(response.StatusCode), fmt.Errorf("OpenAI-compatible LLM returned status %d: %s", response.StatusCode, responseError(responseBody))
	}

	content, err := parseCompletion(responseBody)
	if err != nil {
		return "", false, err
	}
	return content, false, nil
}

func parseCompletion(responseBody []byte) (string, error) {
	var response chatCompletionResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return "", fmt.Errorf("%w: decode OpenAI-compatible LLM response: %v", extractAIDomain.ErrInvalidResponse, err)
	}
	if len(response.Choices) == 0 {
		return "", extractAIDomain.ErrInvalidResponse
	}
	content, err := response.Choices[0].Message.content()
	if err != nil {
		return "", extractAIDomain.ErrInvalidResponse
	}
	return content, nil
}

func responseError(responseBody []byte) string {
	var response struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(responseBody, &response) == nil && response.Error.Message != "" {
		return response.Error.Message
	}
	return strings.TrimSpace(string(responseBody))
}

func isTemporary(err error) bool {
	var urlError *url.Error
	if errors.As(err, &urlError) {
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError) && (networkError.Timeout() || networkError.Temporary())
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooEarly || status >= http.StatusInternalServerError
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type chatCompletionRequest struct {
	Model       string                  `json:"model"`
	Temperature float64                 `json:"temperature"`
	Messages    []chatCompletionMessage `json:"messages"`
}

type chatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []chatCompletionChoice `json:"choices"`
}

type chatCompletionChoice struct {
	Message chatCompletionResponseMessage `json:"message"`
}

type chatCompletionResponseMessage struct {
	Content json.RawMessage `json:"content"`
}

func (m chatCompletionResponseMessage) content() (string, error) {
	var content string
	if err := json.Unmarshal(m.Content, &content); err == nil {
		return content, nil
	}

	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(m.Content, &parts); err != nil {
		return "", err
	}
	var output strings.Builder
	for _, part := range parts {
		if part.Type != "text" || strings.TrimSpace(part.Text) == "" {
			continue
		}
		output.WriteString(part.Text)
	}
	return output.String(), nil
}
