package domain

import (
	"testing"
	"time"
)

func TestServerConfigAIExtractionConfigurationErrorExplainsMissingSettings(t *testing.T) {
	configuration := ServerConfig{}

	err := configuration.AIExtractionConfigurationError()
	if err == nil || err.Error() != "llm.api_key is required" {
		t.Errorf("AIExtractionConfigurationError() = %v", err)
	}
}

func TestLLMConfigConfigurationErrorExplainsMissingBaseURL(t *testing.T) {
	configuration := LLMConfig{APIKey: "key"}

	err := configuration.ConfigurationError()
	if err == nil || err.Error() != "invalid configuration: LLM base URL is required" {
		t.Errorf("ConfigurationError() = %v", err)
	}
}

func TestServerConfigAIExtractionConfigurationErrorAcceptsCompleteConfiguration(t *testing.T) {
	configuration := ServerConfig{
		LLM: LLMConfig{BaseURL: "https://llm.example.com/v1", APIKey: "key"},
		ExtractAI: ExtractAIConfig{
			Model: "model", Timeout: time.Second,
		},
	}

	if err := configuration.AIExtractionConfigurationError(); err != nil {
		t.Errorf("AIExtractionConfigurationError() error = %v", err)
	}
}
