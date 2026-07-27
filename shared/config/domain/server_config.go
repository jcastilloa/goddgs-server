package domain

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

type ServerConfig struct {
	Service         ServiceConfig
	AuthToken       string
	RequestTimeout  time.Duration
	MaxProxyRetries int
	Proxies         []ProxyConfig
	LLM             LLMConfig
	ExtractAI       ExtractAIConfig
}

type ProxyConfig struct {
	Name           string `mapstructure:"name"`
	Type           string `mapstructure:"type"`
	URL            string `mapstructure:"url"`
	Host           string `mapstructure:"host"`
	Port           int    `mapstructure:"port"`
	User           string `mapstructure:"user"`
	PrivateKeyPath string `mapstructure:"private_key_path"`
	HostKey        string `mapstructure:"host_key"`
}

func (c ServerConfig) Validate() error {
	if len(c.Proxies) == 0 {
		return fmt.Errorf("%w: at least one proxy is required", ErrInvalidConfiguration)
	}

	names := make(map[string]struct{}, len(c.Proxies))
	for _, proxy := range c.Proxies {
		if err := proxy.Validate(); err != nil {
			return err
		}
		if _, exists := names[proxy.Name]; exists {
			return fmt.Errorf("%w: duplicate proxy name %q", ErrInvalidConfiguration, proxy.Name)
		}
		names[proxy.Name] = struct{}{}
	}
	return nil
}

func (c ServerConfig) AIExtractionConfigurationError() error {
	if err := c.LLM.ConfigurationError(); err != nil {
		return err
	}
	return c.ExtractAI.Validate()
}

func (c ProxyConfig) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("%w: proxy name is required", ErrInvalidConfiguration)
	}

	switch strings.ToLower(strings.TrimSpace(c.Type)) {
	case "direct":
		if strings.TrimSpace(c.URL) == "" {
			return nil
		}
		if err := validateDirectProxyURL(c.URL); err != nil {
			return err
		}
	case "ssh":
		if strings.TrimSpace(c.Host) == "" || strings.TrimSpace(c.User) == "" || strings.TrimSpace(c.PrivateKeyPath) == "" {
			return fmt.Errorf("%w: SSH proxy host, user, and private key path are required", ErrInvalidConfiguration)
		}
		if strings.TrimSpace(c.HostKey) == "" {
			return fmt.Errorf("%w: SSH proxy host key is required", ErrInvalidConfiguration)
		}
	default:
		return fmt.Errorf("%w: unsupported proxy type %q", ErrInvalidConfiguration, c.Type)
	}
	return nil
}

func validateDirectProxyURL(rawURL string) error {
	if strings.TrimSpace(rawURL) == "tb" {
		return nil
	}
	parsed, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%w: direct proxy URL is required", ErrInvalidConfiguration)
	}
	switch parsed.Scheme {
	case "http", "https", "socks5", "socks5h":
		return nil
	default:
		return fmt.Errorf("%w: unsupported direct proxy scheme %q", ErrInvalidConfiguration, parsed.Scheme)
	}
}
