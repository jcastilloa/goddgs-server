package domain

import (
	"errors"
	"testing"
	"time"
)

func TestChromeConfigValidatesOnlyWhenEnabled(t *testing.T) {
	tests := []struct {
		name    string
		config  ChromeConfig
		wantErr bool
	}{
		{
			name:   "disabled accepts zero values",
			config: ChromeConfig{},
		},
		{
			name: "enabled accepts positive limits",
			config: ChromeConfig{
				Enabled:            true,
				Timeout:            time.Second,
				MaxBrowsers:        1,
				MaxPagesPerBrowser: 1,
				IdleTimeout:        time.Second,
			},
		},
		{
			name: "enabled rejects non-positive timeout",
			config: ChromeConfig{
				Enabled:            true,
				MaxBrowsers:        1,
				MaxPagesPerBrowser: 1,
				IdleTimeout:        time.Second,
			},
			wantErr: true,
		},
		{
			name: "enabled rejects non-positive browser limit",
			config: ChromeConfig{
				Enabled:            true,
				Timeout:            time.Second,
				MaxPagesPerBrowser: 1,
				IdleTimeout:        time.Second,
			},
			wantErr: true,
		},
		{
			name: "enabled rejects non-positive page limit",
			config: ChromeConfig{
				Enabled:     true,
				Timeout:     time.Second,
				MaxBrowsers: 1,
				IdleTimeout: time.Second,
			},
			wantErr: true,
		},
		{
			name: "enabled rejects non-positive idle timeout",
			config: ChromeConfig{
				Enabled:            true,
				Timeout:            time.Second,
				MaxBrowsers:        1,
				MaxPagesPerBrowser: 1,
			},
			wantErr: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.config.Validate()
			if testCase.wantErr && !errors.Is(err, ErrInvalidConfiguration) {
				t.Errorf("Validate() error = %v, want ErrInvalidConfiguration", err)
			}
			if !testCase.wantErr && err != nil {
				t.Errorf("Validate() error = %v", err)
			}
		})
	}
}
