package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jcastilloa/goddgs-server/shared/buildinfo"
	configDomain "github.com/jcastilloa/goddgs-server/shared/config/domain"
)

func TestNewFromFileLoadsServerConfiguration(t *testing.T) {
	path := writeConfig(t, `
service:
  host: 127.0.0.1
  port: 9090
  api_prefix: /api/v1
  version: 1.2.3
  request_timeout: 17s
  max_proxy_retries: 2

auth:
  token: secret-token

llm:
  base_url: https://llm.example.com/v1
  api_key: secret-llm-key
  headers:
    X-Client-Name: goddgs-server-test

extract_ai:
  model: gpt-4.1-mini
  timeout: 45s
  temperature: 0.15
  retries: 2

chrome:
  enabled: true
  executable_path: /usr/bin/chromium
  timeout: 40s
  max_browsers: 4
  max_pages_per_browser: 5
  idle_timeout: 2m

research:
  timeout: 8m
  max_concurrent_extractions: 20
  max_selection_candidates: 75
  max_selected_sources: 15
  query_ai:
    model: gpt-4.1-mini
    timeout: 25s
    temperature: 0.2
    retries: 1
  selection_ai:
    model: gpt-4.1-nano
    timeout: 35s
    temperature: 0.4
    retries: 2
  report_ai:
    model: gpt-4.1
    timeout: 55s
    temperature: 0.3
    retries: 3

operations:
  database_path: /var/lib/goddgs/operations.sqlite
  retention: 48h
  dashboard_auth:
    session_ttl: 6h
    cookie_secure: true
  probe:
    enabled: true
    url: https://status.example.com/probe
    interval: 2m
    timeout: 7s
    success_threshold: 2
    failure_threshold: 3

proxies:
  - name: direct-eu
    type: direct
    url: socks5h://127.0.0.1:9050
  - name: tunnel-us
    type: ssh
    host: proxy.example.com
    port: 2222
    user: tunnel-user
    private_key_path: /keys/tunnel
    host_key: ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA proxy.example.com
`)

	repository, err := NewFromFile(path)
	if err != nil {
		t.Fatalf("NewFromFile() error = %v", err)
	}

	got := repository.ServerConfig()
	if got.Service.HTTPAddress() != "127.0.0.1:9090" {
		t.Errorf("HTTPAddress() = %q, want 127.0.0.1:9090", got.Service.HTTPAddress())
	}
	if got.Service.NormalizedAPIPrefix() != "/api/v1" {
		t.Errorf("NormalizedAPIPrefix() = %q, want /api/v1", got.Service.NormalizedAPIPrefix())
	}
	if got.Service.Version != "1.2.3" {
		t.Errorf("Version = %q, want 1.2.3", got.Service.Version)
	}
	if got.RequestTimeout.String() != "17s" {
		t.Errorf("RequestTimeout = %v, want 17s", got.RequestTimeout)
	}
	if got.MaxProxyRetries != 2 {
		t.Errorf("MaxProxyRetries = %d, want 2", got.MaxProxyRetries)
	}
	if got.AuthToken != "secret-token" {
		t.Errorf("AuthToken = %q, want secret-token", got.AuthToken)
	}
	if got.LLM.BaseURL != "https://llm.example.com/v1" || got.LLM.APIKey != "secret-llm-key" || got.LLM.Headers["x-client-name"] != "goddgs-server-test" {
		t.Errorf("LLM = %#v", got.LLM)
	}
	if got.ExtractAI.Model != "gpt-4.1-mini" || got.ExtractAI.Timeout.String() != "45s" || got.ExtractAI.Temperature != 0.15 || got.ExtractAI.Retries != 2 {
		t.Errorf("ExtractAI = %#v", got.ExtractAI)
	}
	if chrome := got.Chrome; !chrome.Enabled || chrome.ExecutablePath != "/usr/bin/chromium" || chrome.Timeout != 40*time.Second || chrome.MaxBrowsers != 4 || chrome.MaxPagesPerBrowser != 5 || chrome.IdleTimeout != 2*time.Minute {
		t.Errorf("Chrome = %#v", chrome)
	}
	if got.Research.Timeout.String() != "8m0s" || got.Research.MaxConcurrentExtractions != 20 || got.Research.MaxSelectionCandidates != 75 || got.Research.MaxSelectedSources != 15 || got.Research.QueryAI.Model != "gpt-4.1-mini" || got.Research.QueryAI.Timeout.String() != "25s" || got.Research.QueryAI.Temperature != 0.2 || got.Research.QueryAI.Retries != 1 || got.Research.SelectionAI.Model != "gpt-4.1-nano" || got.Research.SelectionAI.Timeout.String() != "35s" || got.Research.SelectionAI.Temperature != 0.4 || got.Research.SelectionAI.Retries != 2 || got.Research.ReportAI.Model != "gpt-4.1" || got.Research.ReportAI.Timeout.String() != "55s" || got.Research.ReportAI.Temperature != 0.3 || got.Research.ReportAI.Retries != 3 {
		t.Errorf("Research = %#v", got.Research)
	}
	if got.Operations.DatabasePath != "/var/lib/goddgs/operations.sqlite" || got.Operations.Retention != 48*time.Hour {
		t.Errorf("Operations = %#v", got.Operations)
	}
	if probe := got.Operations.Probe; !probe.Enabled || probe.URL != "https://status.example.com/probe" || probe.Interval != 2*time.Minute || probe.Timeout != 7*time.Second || probe.SuccessThreshold != 2 || probe.FailureThreshold != 3 {
		t.Errorf("Operations.Probe = %#v", probe)
	}
	if auth := got.Operations.DashboardAuth; auth.SessionTTL != 6*time.Hour || !auth.CookieSecure {
		t.Errorf("Operations.DashboardAuth = %#v", auth)
	}
	if len(got.Proxies) != 2 {
		t.Fatalf("Proxies length = %d, want 2", len(got.Proxies))
	}
	if got.Proxies[0].Name != "direct-eu" || got.Proxies[0].URL != "socks5h://127.0.0.1:9050" {
		t.Errorf("direct proxy = %#v", got.Proxies[0])
	}
	if got.Proxies[1].Host != "proxy.example.com" || got.Proxies[1].Port != 2222 {
		t.Errorf("ssh proxy = %#v", got.Proxies[1])
	}
}

func TestNewFromFileDefaultsAndOverridesChromeConfiguration(t *testing.T) {
	repository, err := NewFromFile(writeConfig(t, `
proxies:
  - name: local
    type: direct
`))
	if err != nil {
		t.Fatalf("NewFromFile() error = %v", err)
	}
	if chrome := repository.ServerConfig().Chrome; chrome.Enabled || chrome.Timeout != configDomain.DefaultChromeTimeout || chrome.MaxBrowsers != configDomain.DefaultChromeMaxBrowsers || chrome.MaxPagesPerBrowser != configDomain.DefaultChromeMaxPagesPerBrowser || chrome.IdleTimeout != configDomain.DefaultChromeIdleTimeout {
		t.Errorf("default Chrome = %#v", chrome)
	}

	t.Setenv("CHROME_ENABLED", "true")
	t.Setenv("CHROME_TIMEOUT", "17s")
	t.Setenv("CHROME_MAX_BROWSERS", "3")
	t.Setenv("CHROME_MAX_PAGES_PER_BROWSER", "4")
	t.Setenv("CHROME_IDLE_TIMEOUT", "3m")
	repository, err = NewFromFile(writeConfig(t, `
proxies:
  - name: local
    type: direct
`))
	if err != nil {
		t.Fatalf("NewFromFile() with environment overrides error = %v", err)
	}
	if chrome := repository.ServerConfig().Chrome; !chrome.Enabled || chrome.Timeout != 17*time.Second || chrome.MaxBrowsers != 3 || chrome.MaxPagesPerBrowser != 4 || chrome.IdleTimeout != 3*time.Minute {
		t.Errorf("environment Chrome = %#v", chrome)
	}
}

func TestViperRepositoryPersistsDiscoveredChromeExecutablePath(t *testing.T) {
	t.Setenv("CHROME_EXECUTABLE_PATH", "")
	path := writeConfig(t, `# Keep this comment and every unrelated setting.
chrome:
  enabled: false
  executable_path: "" # resolved on startup
  timeout: 45s
proxies:
  - name: local
    type: direct
`)
	repository, err := NewFromFile(path)
	if err != nil {
		t.Fatalf("NewFromFile() error = %v", err)
	}

	if err := repository.PersistChromeExecutablePath(context.Background(), "/usr/bin/chromium"); err != nil {
		t.Fatalf("PersistChromeExecutablePath() error = %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if got := string(contents); !strings.Contains(got, "# Keep this comment and every unrelated setting.\nchrome:\n  enabled: false\n  executable_path: \"/usr/bin/chromium\" # resolved on startup\n  timeout: 45s") {
		t.Errorf("persisted configuration = %q", got)
	}
	reloaded, err := NewFromFile(path)
	if err != nil {
		t.Fatalf("NewFromFile() after persistence error = %v", err)
	}
	if got := reloaded.ServerConfig().Chrome.ExecutablePath; got != "/usr/bin/chromium" {
		t.Errorf("persisted executable path = %q, want /usr/bin/chromium", got)
	}
}

func TestViperRepositoryDoesNotReplaceConfiguredChromeExecutablePath(t *testing.T) {
	t.Setenv("CHROME_EXECUTABLE_PATH", "")
	path := writeConfig(t, `chrome:
  executable_path: /operator/chrome
proxies:
  - name: local
    type: direct
`)
	repository, err := NewFromFile(path)
	if err != nil {
		t.Fatalf("NewFromFile() error = %v", err)
	}

	if err := repository.PersistChromeExecutablePath(context.Background(), "/usr/bin/chromium"); err != nil {
		t.Fatalf("PersistChromeExecutablePath() error = %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(contents), "/usr/bin/chromium") {
		t.Errorf("persisted configuration replaced configured path: %q", contents)
	}
}

func TestViperRepositoryAddsMissingChromeSectionWhenPersistingExecutablePath(t *testing.T) {
	t.Setenv("CHROME_EXECUTABLE_PATH", "")
	path := writeConfig(t, `service:
  port: 8080
proxies:
  - name: local
    type: direct
`)
	repository, err := NewFromFile(path)
	if err != nil {
		t.Fatalf("NewFromFile() error = %v", err)
	}

	if err := repository.PersistChromeExecutablePath(context.Background(), "/usr/bin/google-chrome"); err != nil {
		t.Fatalf("PersistChromeExecutablePath() error = %v", err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if got := string(contents); !strings.HasSuffix(got, "\nchrome:\n  executable_path: \"/usr/bin/google-chrome\"\n") {
		t.Errorf("persisted configuration = %q", got)
	}
}

func TestNewFromFileRejectsInvalidEnabledChromeConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		chrome string
	}{
		{name: "zero timeout", chrome: "timeout: 0s\n  max_browsers: 1\n  max_pages_per_browser: 1\n  idle_timeout: 1m"},
		{name: "negative timeout", chrome: "timeout: -1s\n  max_browsers: 1\n  max_pages_per_browser: 1\n  idle_timeout: 1m"},
		{name: "zero browser limit", chrome: "timeout: 1s\n  max_browsers: 0\n  max_pages_per_browser: 1\n  idle_timeout: 1m"},
		{name: "negative browser limit", chrome: "timeout: 1s\n  max_browsers: -1\n  max_pages_per_browser: 1\n  idle_timeout: 1m"},
		{name: "zero page limit", chrome: "timeout: 1s\n  max_browsers: 1\n  max_pages_per_browser: 0\n  idle_timeout: 1m"},
		{name: "negative page limit", chrome: "timeout: 1s\n  max_browsers: 1\n  max_pages_per_browser: -1\n  idle_timeout: 1m"},
		{name: "zero idle timeout", chrome: "timeout: 1s\n  max_browsers: 1\n  max_pages_per_browser: 1\n  idle_timeout: 0s"},
		{name: "negative idle timeout", chrome: "timeout: 1s\n  max_browsers: 1\n  max_pages_per_browser: 1\n  idle_timeout: -1s"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := NewFromFile(writeConfig(t, "chrome:\n  enabled: true\n  "+testCase.chrome+"\nproxies:\n  - name: local\n    type: direct\n"))
			if !errors.Is(err, configDomain.ErrInvalidConfiguration) {
				t.Errorf("NewFromFile() error = %v, want ErrInvalidConfiguration", err)
			}
		})
	}
}

func TestNewFromFileAllowsDirectConnectionWithoutProxyURL(t *testing.T) {
	repository, err := NewFromFile(writeConfig(t, `
proxies:
  - name: local
    type: direct
`))
	if err != nil {
		t.Fatalf("NewFromFile() error = %v", err)
	}

	proxies := repository.ServerConfig().Proxies
	if len(proxies) != 1 || proxies[0].URL != "" {
		t.Errorf("Proxies = %#v, want one direct connection without proxy URL", proxies)
	}
}

func TestNewFromFileDefaultsAndValidatesOperationsConfiguration(t *testing.T) {
	repository, err := NewFromFile(writeConfig(t, `
proxies:
  - name: local
    type: direct
`))
	if err != nil {
		t.Fatalf("NewFromFile() error = %v", err)
	}
	if got := repository.ServerConfig().Operations; got.DatabasePath != "" || got.Retention != configDomain.DefaultOperationsRetention || got.DashboardAuth.SessionTTL != configDomain.DefaultDashboardAuthSessionTTL {
		t.Errorf("Operations = %#v", got)
	}

	_, err = NewFromFile(writeConfig(t, `
operations:
  retention: -1h
proxies:
  - name: local
    type: direct
`))
	if !errors.Is(err, configDomain.ErrInvalidConfiguration) {
		t.Errorf("NewFromFile() error = %v, want ErrInvalidConfiguration", err)
	}

	_, err = NewFromFile(writeConfig(t, `
operations:
  dashboard_auth:
    session_ttl: -1h
proxies:
  - name: local
    type: direct
`))
	if !errors.Is(err, configDomain.ErrInvalidConfiguration) {
		t.Errorf("NewFromFile() negative dashboard session TTL error = %v, want ErrInvalidConfiguration", err)
	}

	_, err = NewFromFile(writeConfig(t, `
operations:
  dashboard_auth:
    session_ttl: 0s
proxies:
  - name: local
    type: direct
`))
	if !errors.Is(err, configDomain.ErrInvalidConfiguration) {
		t.Errorf("NewFromFile() zero dashboard session TTL error = %v, want ErrInvalidConfiguration", err)
	}
}

func TestServerConfigUsesBuildVersionWhenConfigurationOmitsIt(t *testing.T) {
	original := buildinfo.Version
	buildinfo.Version = "v1.2.3"
	t.Cleanup(func() { buildinfo.Version = original })

	repository, err := NewFromFile(writeConfig(t, `
proxies:
  - name: local
    type: direct
`))
	if err != nil {
		t.Fatalf("NewFromFile() error = %v", err)
	}

	if got := repository.ServerConfig().Service.Version; got != "v1.2.3" {
		t.Errorf("Version = %q, want v1.2.3", got)
	}
}

func TestNewFromFileRejectsMissingOrInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{
			name: "missing file",
			path: filepath.Join(t.TempDir(), "config.yaml"),
		},
		{
			name: "invalid yaml",
			path: writeConfig(t, "service: ["),
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := NewFromFile(testCase.path)
			if !errors.Is(err, configDomain.ErrInvalidConfiguration) {
				t.Errorf("NewFromFile() error = %v, want ErrInvalidConfiguration", err)
			}
		})
	}
}

func TestNewFromFileRejectsInvalidProxyConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		contents string
	}{
		{
			name: "no proxies",
			contents: `
service:
  request_timeout: 10s
`,
		},
		{
			name: "direct proxy with unsupported scheme",
			contents: `
proxies:
  - name: direct
    type: direct
    url: ftp://proxy.example.com
`,
		},
		{
			name: "SSH proxy without verified host key",
			contents: `
proxies:
  - name: tunnel
    type: ssh
    host: proxy.example.com
    user: deploy
    private_key_path: /keys/id_ed25519
`,
		},
		{
			name: "unsupported proxy type",
			contents: `
proxies:
  - name: unknown
    type: rotating
`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := NewFromFile(writeConfig(t, testCase.contents))
			if !errors.Is(err, configDomain.ErrInvalidConfiguration) {
				t.Errorf("NewFromFile() error = %v, want ErrInvalidConfiguration", err)
			}
		})
	}
}

func TestNewFromFileRejectsInvalidConfiguredAIExtraction(t *testing.T) {
	tests := []struct {
		name     string
		contents string
	}{
		{
			name: "invalid LLM base URL",
			contents: `
llm:
  api_key: key
  base_url: ftp://llm.example.com
extract_ai:
  model: gpt-4.1-mini
  timeout: 30s
proxies:
  - name: direct
    type: direct
`,
		},
		{
			name: "invalid AI temperature",
			contents: `
llm:
  api_key: key
  base_url: https://llm.example.com/v1
extract_ai:
  model: gpt-4.1-mini
  timeout: 30s
  temperature: 3
proxies:
  - name: direct
    type: direct
`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			repository, err := NewFromFile(writeConfig(t, testCase.contents))
			if err != nil {
				t.Fatalf("NewFromFile() error = %v", err)
			}
			if err := repository.ServerConfig().AIExtractionConfigurationError(); !errors.Is(err, configDomain.ErrInvalidConfiguration) {
				t.Errorf("AIExtractionConfigurationError() = %v, want ErrInvalidConfiguration", err)
			}
		})
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
