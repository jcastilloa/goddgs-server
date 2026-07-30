package domain

import (
	"fmt"
	"time"
)

const (
	DefaultChromeTimeout            = 45 * time.Second
	DefaultChromeMaxBrowsers        = 2
	DefaultChromeMaxPagesPerBrowser = 3
	DefaultChromeIdleTimeout        = time.Minute
)

type ChromeConfig struct {
	Enabled            bool
	ExecutablePath     string
	Timeout            time.Duration
	MaxBrowsers        int
	MaxPagesPerBrowser int
	IdleTimeout        time.Duration
}

func (c ChromeConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("%w: chrome timeout must be positive", ErrInvalidConfiguration)
	}
	if c.MaxBrowsers <= 0 {
		return fmt.Errorf("%w: chrome max browsers must be positive", ErrInvalidConfiguration)
	}
	if c.MaxPagesPerBrowser <= 0 {
		return fmt.Errorf("%w: chrome max pages per browser must be positive", ErrInvalidConfiguration)
	}
	if c.IdleTimeout <= 0 {
		return fmt.Errorf("%w: chrome idle timeout must be positive", ErrInvalidConfiguration)
	}
	return nil
}
