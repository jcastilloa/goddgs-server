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

type ResearchAIConfig struct {
	Model       string
	Timeout     time.Duration
	Temperature float64
	Retries     int
}

type ResearchConfig struct {
	Timeout                  time.Duration
	MaxConcurrentExtractions int
	QueryAI                  ResearchAIConfig
	ReportAI                 ResearchAIConfig
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
	return validateAIConfig("AI extraction", c.Model, c.Timeout, c.Temperature, c.Retries)
}

func (c ResearchAIConfig) Validate() error {
	return validateAIConfig("research AI", c.Model, c.Timeout, c.Temperature, c.Retries)
}

func (c ResearchConfig) Validate() error {
	if c.Timeout <= 0 {
		return fmt.Errorf("%w: research timeout must be positive", ErrInvalidConfiguration)
	}
	if c.MaxConcurrentExtractions <= 0 {
		return fmt.Errorf("%w: research max concurrent extractions must be positive", ErrInvalidConfiguration)
	}
	if err := validateAIConfig("research query AI", c.QueryAI.Model, c.QueryAI.Timeout, c.QueryAI.Temperature, c.QueryAI.Retries); err != nil {
		return err
	}
	return validateAIConfig("research report AI", c.ReportAI.Model, c.ReportAI.Timeout, c.ReportAI.Temperature, c.ReportAI.Retries)
}

func validateAIConfig(name, model string, timeout time.Duration, temperature float64, retries int) error {
	if strings.TrimSpace(model) == "" {
		return fmt.Errorf("%w: %s model is required", ErrInvalidConfiguration, name)
	}
	if timeout <= 0 {
		return fmt.Errorf("%w: %s timeout must be positive", ErrInvalidConfiguration, name)
	}
	if temperature < 0 || temperature > 2 {
		return fmt.Errorf("%w: %s temperature must be between 0 and 2", ErrInvalidConfiguration, name)
	}
	if retries < 0 {
		return fmt.Errorf("%w: %s retries cannot be negative", ErrInvalidConfiguration, name)
	}
	return nil
}
