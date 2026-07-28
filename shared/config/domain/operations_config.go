package domain

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultOperationsRetention     = 30 * 24 * time.Hour
	DefaultDashboardAuthSessionTTL = 12 * time.Hour
)

type OperationsConfig struct {
	DatabasePath  string
	Retention     time.Duration
	Probe         ProbeConfig
	DashboardAuth DashboardAuthConfig
}

type DashboardAuthConfig struct {
	SessionTTL   time.Duration
	CookieSecure bool
}

type ProbeConfig struct {
	Enabled          bool
	URL              string
	Interval         time.Duration
	Timeout          time.Duration
	SuccessThreshold int
	FailureThreshold int
}

func (c OperationsConfig) Validate() error {
	if c.Retention <= 0 {
		return fmt.Errorf("%w: operations retention must be positive", ErrInvalidConfiguration)
	}
	if c.DashboardAuth.SessionTTL <= 0 {
		return fmt.Errorf("%w: operations dashboard auth session TTL must be positive", ErrInvalidConfiguration)
	}
	return c.Probe.Validate()
}

func (c ProbeConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	parsedURL, err := url.ParseRequestURI(strings.TrimSpace(c.URL))
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return fmt.Errorf("%w: operations probe URL must be HTTP(S)", ErrInvalidConfiguration)
	}
	if c.Interval <= 0 {
		return fmt.Errorf("%w: operations probe interval must be positive", ErrInvalidConfiguration)
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("%w: operations probe timeout must be positive", ErrInvalidConfiguration)
	}
	if c.SuccessThreshold <= 0 {
		return fmt.Errorf("%w: operations probe success threshold must be positive", ErrInvalidConfiguration)
	}
	if c.FailureThreshold <= 0 {
		return fmt.Errorf("%w: operations probe failure threshold must be positive", ErrInvalidConfiguration)
	}
	return nil
}
