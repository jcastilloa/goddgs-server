package domain

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type LLMConfig struct {
	BaseURL string
	APIKey  string
	Headers map[string]string
}

type ExtractAIConfig struct {
	Model       string
	Timeout     time.Duration
	Temperature float64
	Retries     int
}

func (c LLMConfig) ConfigurationError() error {
	if strings.TrimSpace(c.APIKey) == "" {
		return errors.New("llm.api_key is required")
	}
	if err := c.Validate(); err != nil {
		return err
	}
	return nil
}

func (c LLMConfig) Validate() error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("%w: LLM base URL is required", ErrInvalidConfiguration)
	}
	parsed, err := url.ParseRequestURI(strings.TrimSpace(c.BaseURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%w: LLM base URL must be an HTTP(S) URL", ErrInvalidConfiguration)
	}
	for name, value := range c.Headers {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: LLM headers must have non-empty names and values", ErrInvalidConfiguration)
		}
	}
	return nil
}

func (c ExtractAIConfig) Validate() error {
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("%w: AI extraction model is required", ErrInvalidConfiguration)
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("%w: AI extraction timeout must be positive", ErrInvalidConfiguration)
	}
	if c.Temperature < 0 || c.Temperature > 2 {
		return fmt.Errorf("%w: AI extraction temperature must be between 0 and 2", ErrInvalidConfiguration)
	}
	if c.Retries < 0 {
		return fmt.Errorf("%w: AI extraction retries cannot be negative", ErrInvalidConfiguration)
	}
	return nil
}
