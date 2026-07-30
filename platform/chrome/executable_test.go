package chrome

import (
	"context"
	"testing"
)

func TestExecutableLocatorFindsAndPersistsUnsetPath(t *testing.T) {
	persisted := make(chan string, 1)
	locator := newExecutableLocator(
		context.Background(),
		"",
		func() (string, bool) { return "/usr/bin/chromium", true },
		func(path string) { persisted <- path },
	)

	<-locator.Done()
	if got := locator.ExecutablePath(); got != "/usr/bin/chromium" {
		t.Errorf("ExecutablePath() = %q, want /usr/bin/chromium", got)
	}
	if got := <-persisted; got != "/usr/bin/chromium" {
		t.Errorf("persisted executable = %q, want /usr/bin/chromium", got)
	}
}

func TestExecutableLocatorKeepsConfiguredPathWithoutSearching(t *testing.T) {
	locator := newExecutableLocator(
		context.Background(),
		"/configured/chrome",
		func() (string, bool) {
			t.Fatal("finder must not run for a configured path")
			return "", false
		},
		func(string) { t.Fatal("persist must not run for a configured path") },
	)

	<-locator.Done()
	if got := locator.ExecutablePath(); got != "/configured/chrome" {
		t.Errorf("ExecutablePath() = %q, want /configured/chrome", got)
	}
}

func TestExecutableLocatorDoesNotSearchAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	locator := newExecutableLocator(
		ctx,
		"",
		func() (string, bool) {
			t.Fatal("finder must not run after cancellation")
			return "", false
		},
		nil,
	)

	<-locator.Done()
	if got := locator.ExecutablePath(); got != "" {
		t.Errorf("ExecutablePath() = %q, want empty", got)
	}
}
