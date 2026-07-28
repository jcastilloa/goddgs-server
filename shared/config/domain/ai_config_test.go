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

func TestOperationsConfigDefaultsAndValidatesRetention(t *testing.T) {
	configuration := ServerConfig{
		Operations: OperationsConfig{Retention: 30 * 24 * time.Hour, DashboardAuth: DashboardAuthConfig{SessionTTL: DefaultDashboardAuthSessionTTL}},
	}

	if err := configuration.Operations.Validate(); err != nil {
		t.Fatalf("Operations.Validate() error = %v", err)
	}

	configuration.Operations.Retention = 0
	if err := configuration.Operations.Validate(); err == nil || err.Error() != "invalid configuration: operations retention must be positive" {
		t.Errorf("Operations.Validate() error = %v", err)
	}
}

func TestOperationsProbeConfigRequiresCompleteEnabledConfiguration(t *testing.T) {
	valid := OperationsConfig{
		Retention:     time.Hour,
		DashboardAuth: DashboardAuthConfig{SessionTTL: DefaultDashboardAuthSessionTTL},
		Probe: ProbeConfig{
			Enabled:          true,
			URL:              "https://status.example.com/probe",
			Interval:         time.Minute,
			Timeout:          5 * time.Second,
			SuccessThreshold: 2,
			FailureThreshold: 3,
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ProbeConfig)
	}{
		{name: "missing URL", mutate: func(c *ProbeConfig) { c.URL = "" }},
		{name: "unsupported URL scheme", mutate: func(c *ProbeConfig) { c.URL = "ftp://status.example.com" }},
		{name: "zero interval", mutate: func(c *ProbeConfig) { c.Interval = 0 }},
		{name: "zero timeout", mutate: func(c *ProbeConfig) { c.Timeout = 0 }},
		{name: "zero success threshold", mutate: func(c *ProbeConfig) { c.SuccessThreshold = 0 }},
		{name: "zero failure threshold", mutate: func(c *ProbeConfig) { c.FailureThreshold = 0 }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			configuration := valid
			testCase.mutate(&configuration.Probe)
			if err := configuration.Validate(); err == nil {
				t.Error("Validate() error = nil, want invalid probe configuration")
			}
		})
	}
}

func TestOperationsProbeConfigAllowsDisabledProbeWithoutSettings(t *testing.T) {
	if err := (OperationsConfig{Retention: time.Hour, DashboardAuth: DashboardAuthConfig{SessionTTL: DefaultDashboardAuthSessionTTL}}).Validate(); err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

func TestServerConfigRejectsEnabledInvalidProbeWithoutOperationsRetention(t *testing.T) {
	configuration := ServerConfig{
		Operations: OperationsConfig{Probe: ProbeConfig{Enabled: true}},
		Proxies:    []ProxyConfig{{Name: "direct", Type: "direct"}},
	}
	if err := configuration.Validate(); err == nil {
		t.Error("Validate() error = nil, want invalid probe configuration")
	}
}
