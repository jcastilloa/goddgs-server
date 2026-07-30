package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jcastilloa/goddgs-server/shared/buildinfo"
	configDomain "github.com/jcastilloa/goddgs-server/shared/config/domain"

	"github.com/spf13/viper"
)

type ViperRepository struct {
	v       *viper.Viper
	writeMu sync.Mutex
}

func New(serviceName string) (*ViperRepository, error) {
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
	chromeTimeout := r.v.GetDuration("chrome.timeout")
	if !r.v.IsSet("chrome.timeout") {
		chromeTimeout = configDomain.DefaultChromeTimeout
	}
	chromeMaxBrowsers := r.v.GetInt("chrome.max_browsers")
	if !r.v.IsSet("chrome.max_browsers") {
		chromeMaxBrowsers = configDomain.DefaultChromeMaxBrowsers
	}
	chromeMaxPages := r.v.GetInt("chrome.max_pages_per_browser")
	if !r.v.IsSet("chrome.max_pages_per_browser") {
		chromeMaxPages = configDomain.DefaultChromeMaxPagesPerBrowser
	}
	chromeIdleTimeout := r.v.GetDuration("chrome.idle_timeout")
	if !r.v.IsSet("chrome.idle_timeout") {
		chromeIdleTimeout = configDomain.DefaultChromeIdleTimeout
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
			MaxSelectionCandidates:   r.v.GetInt("research.max_selection_candidates"),
			MaxSelectedSources:       r.v.GetInt("research.max_selected_sources"),
			QueryAI: configDomain.ResearchAIConfig{
				Model:       r.v.GetString("research.query_ai.model"),
				Timeout:     r.v.GetDuration("research.query_ai.timeout"),
				Temperature: r.v.GetFloat64("research.query_ai.temperature"),
				Retries:     r.v.GetInt("research.query_ai.retries"),
			},
			SelectionAI: configDomain.ResearchAIConfig{
				Model:       r.v.GetString("research.selection_ai.model"),
				Timeout:     r.v.GetDuration("research.selection_ai.timeout"),
				Temperature: r.v.GetFloat64("research.selection_ai.temperature"),
				Retries:     r.v.GetInt("research.selection_ai.retries"),
			},
			ReportAI: configDomain.ResearchAIConfig{
				Model:       r.v.GetString("research.report_ai.model"),
				Timeout:     r.v.GetDuration("research.report_ai.timeout"),
				Temperature: r.v.GetFloat64("research.report_ai.temperature"),
				Retries:     r.v.GetInt("research.report_ai.retries"),
			},
		},
		Chrome: configDomain.ChromeConfig{
			Enabled:            r.v.GetBool("chrome.enabled"),
			ExecutablePath:     r.v.GetString("chrome.executable_path"),
			Timeout:            chromeTimeout,
			MaxBrowsers:        chromeMaxBrowsers,
			MaxPagesPerBrowser: chromeMaxPages,
			IdleTimeout:        chromeIdleTimeout,
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

func (r *ViperRepository) PersistChromeExecutablePath(ctx context.Context, executablePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	executablePath = strings.TrimSpace(executablePath)
	if executablePath == "" || strings.TrimSpace(r.v.GetString("chrome.executable_path")) != "" {
		return nil
	}

	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	return persistChromeExecutablePath(ctx, r.v.ConfigFileUsed(), executablePath)
}

func persistChromeExecutablePath(ctx context.Context, configPath, executablePath string) error {
	contents, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read configuration: %w", err)
	}
	updated, changed := setChromeExecutablePath(string(contents), executablePath)
	if !changed {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return writeConfigurationAtomically(configPath, []byte(updated))
}

func setChromeExecutablePath(contents, executablePath string) (string, bool) {
	lines := strings.SplitAfter(contents, "\n")
	chromeStart, chromeEnd := chromeSection(lines)
	if chromeStart == -1 {
		return appendChromeSection(contents, executablePath), true
	}

	value := strconv.Quote(executablePath)
	for index := chromeStart + 1; index < chromeEnd; index++ {
		if indentation(lines[index]) == "" || !strings.HasPrefix(strings.TrimSpace(lines[index]), "executable_path:") {
			continue
		}
		updated, changed := setChromeExecutablePathLine(lines[index], executablePath)
		if !changed {
			return contents, false
		}
		lines[index] = updated
		return strings.Join(lines, ""), true
	}

	indent := chromeIndent(lines, chromeStart, chromeEnd)
	lines = append(lines[:chromeStart+1], append([]string{indent + "executable_path: " + value + "\n"}, lines[chromeStart+1:]...)...)
	return strings.Join(lines, ""), true
}

func setChromeExecutablePathLine(line, executablePath string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	value := strings.TrimPrefix(trimmed, "executable_path:")
	value, comment := splitYAMLComment(value)
	if !isEmptyYAMLValue(strings.TrimSpace(value)) {
		return line, false
	}
	return indentation(line) + "executable_path: " + strconv.Quote(executablePath) + comment + lineEnding(line), true
}

func splitYAMLComment(value string) (string, string) {
	commentIndex := strings.Index(value, "#")
	if commentIndex == -1 {
		return value, ""
	}
	beforeComment := value[:commentIndex]
	return beforeComment, value[len(strings.TrimRight(beforeComment, " \t")):]
}

func isEmptyYAMLValue(value string) bool {
	return value == "" || value == `""` || value == `''` || value == "~" || strings.EqualFold(value, "null")
}

func chromeSection(lines []string) (int, int) {
	for index, line := range lines {
		if indentation(line) == "" && strings.TrimSpace(line) == "chrome:" {
			end := index + 1
			for end < len(lines) && !isTopLevelSetting(lines[end]) {
				end++
			}
			return index, end
		}
	}
	return -1, -1
}

func isTopLevelSetting(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed != "" && !strings.HasPrefix(trimmed, "#") && indentation(line) == ""
}

func chromeIndent(lines []string, start, end int) string {
	for index := start + 1; index < end; index++ {
		if indent := indentation(lines[index]); indent != "" {
			return indent
		}
	}
	return "  "
}

func indentation(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

func lineEnding(line string) string {
	if strings.HasSuffix(line, "\r\n") {
		return "\r\n"
	}
	if strings.HasSuffix(line, "\n") {
		return "\n"
	}
	return ""
}

func appendChromeSection(contents, executablePath string) string {
	if contents != "" && !strings.HasSuffix(contents, "\n") {
		contents += "\n"
	}
	if contents != "" {
		contents += "\n"
	}
	return contents + "chrome:\n  executable_path: " + strconv.Quote(executablePath) + "\n"
}

func writeConfigurationAtomically(path string, contents []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat configuration: %w", err)
	}
	temporaryFile, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return fmt.Errorf("create temporary configuration: %w", err)
	}
	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)
	if err := temporaryFile.Chmod(info.Mode()); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("set temporary configuration permissions: %w", err)
	}
	if _, err := temporaryFile.Write(contents); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("write temporary configuration: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("close temporary configuration: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace configuration: %w", err)
	}
	return nil
}
