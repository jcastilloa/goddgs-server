package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jcastilloa/goddgs-server/shared/buildinfo"
	configDomain "github.com/jcastilloa/goddgs-server/shared/config/domain"

	"github.com/spf13/viper"
)

type ViperRepository struct {
	v *viper.Viper
}

func New(serviceName string) (configDomain.Repository, error) {
	v := viper.New()
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(filepath.Join(home, ".config", serviceName))
	v.AddConfigPath(".")
	configureEnvironment(v)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("%w: read config: %w", configDomain.ErrInvalidConfiguration, err)
	}

	repository := &ViperRepository{v: v}
	if err := repository.ServerConfig().Validate(); err != nil {
		return nil, err
	}
	return repository, nil
}

func NewFromFile(path string) (*ViperRepository, error) {
	v := viper.New()
	v.SetConfigFile(path)
	configureEnvironment(v)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("%w: read config: %w", configDomain.ErrInvalidConfiguration, err)
	}
	repository := &ViperRepository{v: v}
	if err := repository.ServerConfig().Validate(); err != nil {
		return nil, err
	}
	return repository, nil
}

func configureEnvironment(v *viper.Viper) {
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
}

func (r *ViperRepository) ServiceConfig() configDomain.ServiceConfig {
	version := r.v.GetString("service.version")
	if version == "" {
		version = buildinfo.CurrentVersion()
	}

	return configDomain.ServiceConfig{
		Host:      r.v.GetString("service.host"),
		Port:      r.v.GetInt("service.port"),
		APIPrefix: r.v.GetString("service.api_prefix"),
		Version:   version,
	}
}

func (r *ViperRepository) ServerConfig() configDomain.ServerConfig {
	requestTimeout := r.v.GetDuration("service.request_timeout")
	if requestTimeout <= 0 {
		requestTimeout = 30 * time.Second
	}
	researchTimeout := r.v.GetDuration("research.timeout")
	if researchTimeout <= 0 {
		researchTimeout = 10 * time.Minute
	}
	operationsRetention := r.v.GetDuration("operations.retention")
	if operationsRetention == 0 {
		operationsRetention = configDomain.DefaultOperationsRetention
	}
	dashboardSessionTTL := r.v.GetDuration("operations.dashboard_auth.session_ttl")
	if !r.v.IsSet("operations.dashboard_auth.session_ttl") {
		dashboardSessionTTL = configDomain.DefaultDashboardAuthSessionTTL
	}

	return configDomain.ServerConfig{
		Service:         r.ServiceConfig(),
		AuthToken:       r.v.GetString("auth.token"),
		RequestTimeout:  requestTimeout,
		MaxProxyRetries: r.v.GetInt("service.max_proxy_retries"),
		Proxies:         r.proxyConfigs(),
		LLM: configDomain.LLMConfig{
			BaseURL: r.v.GetString("llm.base_url"),
			APIKey:  r.v.GetString("llm.api_key"),
			Headers: r.v.GetStringMapString("llm.headers"),
		},
		ExtractAI: configDomain.ExtractAIConfig{
			Model:       r.v.GetString("extract_ai.model"),
			Timeout:     r.v.GetDuration("extract_ai.timeout"),
			Temperature: r.v.GetFloat64("extract_ai.temperature"),
			Retries:     r.v.GetInt("extract_ai.retries"),
		},
		Research: configDomain.ResearchConfig{
			Timeout:                  researchTimeout,
			MaxConcurrentExtractions: r.v.GetInt("research.max_concurrent_extractions"),
			QueryAI: configDomain.ResearchAIConfig{
				Model:       r.v.GetString("research.query_ai.model"),
				Timeout:     r.v.GetDuration("research.query_ai.timeout"),
				Temperature: r.v.GetFloat64("research.query_ai.temperature"),
				Retries:     r.v.GetInt("research.query_ai.retries"),
			},
			ReportAI: configDomain.ResearchAIConfig{
				Model:       r.v.GetString("research.report_ai.model"),
				Timeout:     r.v.GetDuration("research.report_ai.timeout"),
				Temperature: r.v.GetFloat64("research.report_ai.temperature"),
				Retries:     r.v.GetInt("research.report_ai.retries"),
			},
		},
		Operations: configDomain.OperationsConfig{
			DatabasePath: r.v.GetString("operations.database_path"),
			Retention:    operationsRetention,
			Probe: configDomain.ProbeConfig{
				Enabled:          r.v.GetBool("operations.probe.enabled"),
				URL:              r.v.GetString("operations.probe.url"),
				Interval:         r.v.GetDuration("operations.probe.interval"),
				Timeout:          r.v.GetDuration("operations.probe.timeout"),
				SuccessThreshold: r.v.GetInt("operations.probe.success_threshold"),
				FailureThreshold: r.v.GetInt("operations.probe.failure_threshold"),
			},
			DashboardAuth: configDomain.DashboardAuthConfig{
				SessionTTL:   dashboardSessionTTL,
				CookieSecure: r.v.GetBool("operations.dashboard_auth.cookie_secure"),
			},
		},
	}
}

func (r *ViperRepository) proxyConfigs() []configDomain.ProxyConfig {
	var proxies []configDomain.ProxyConfig
	if err := r.v.UnmarshalKey("proxies", &proxies); err != nil {
		return nil
	}
	return proxies
}
