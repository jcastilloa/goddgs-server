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

func TestServerConfigResearchConfigurationRequiresSeparateResearchSettings(t *testing.T) {
	configuration := ServerConfig{
		LLM:       LLMConfig{BaseURL: "https://llm.example.com/v1", APIKey: "key"},
		ExtractAI: ExtractAIConfig{Model: "extract", Timeout: time.Second},
		Research: ResearchConfig{
			Timeout:                  2 * time.Minute,
			MaxConcurrentExtractions: 20,
			QueryAI:                  ResearchAIConfig{Model: "query", Timeout: time.Second},
			ReportAI:                 ResearchAIConfig{Model: "report", Timeout: time.Second},
		},
	}

	if err := configuration.ResearchConfigurationError(); err != nil {
		t.Errorf("ResearchConfigurationError() error = %v", err)
	}
	configuration.Research.MaxConcurrentExtractions = 0
	if err := configuration.ResearchConfigurationError(); err == nil || err.Error() != "invalid configuration: research max concurrent extractions must be positive" {
		t.Errorf("ResearchConfigurationError() = %v", err)
	}
	configuration.Research.MaxConcurrentExtractions = 20
	configuration.Research.ReportAI.Timeout = 0
	if err := configuration.ResearchConfigurationError(); err == nil || err.Error() != "invalid configuration: research report AI timeout must be positive" {
		t.Errorf("ResearchConfigurationError() = %v", err)
	}
}
