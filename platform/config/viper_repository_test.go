package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

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

research:
  timeout: 8m
  query_ai:
    model: gpt-4.1-mini
    timeout: 25s
    temperature: 0.2
    retries: 1
  report_ai:
    model: gpt-4.1
    timeout: 55s
    temperature: 0.3
    retries: 3

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
	if got.Research.Timeout.String() != "8m0s" || got.Research.QueryAI.Model != "gpt-4.1-mini" || got.Research.QueryAI.Timeout.String() != "25s" || got.Research.QueryAI.Temperature != 0.2 || got.Research.QueryAI.Retries != 1 || got.Research.ReportAI.Model != "gpt-4.1" || got.Research.ReportAI.Timeout.String() != "55s" || got.Research.ReportAI.Temperature != 0.3 || got.Research.ReportAI.Retries != 3 {
		t.Errorf("Research = %#v", got.Research)
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
