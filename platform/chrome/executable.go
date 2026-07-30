package chrome

import (
	"context"
	"os/exec"
	"strings"
	"sync"
)

type executableFinder func() (string, bool)
type executablePersister func(string)

type ExecutableLocator struct {
	mu             sync.RWMutex
	executablePath string
	done           chan struct{}
}

func NewExecutableLocator(ctx context.Context, executablePath string, persist func(string)) *ExecutableLocator {
	return newExecutableLocator(ctx, executablePath, findChromeExecutable, persist)
}

func newExecutableLocator(ctx context.Context, executablePath string, find executableFinder, persist executablePersister) *ExecutableLocator {
	locator := &ExecutableLocator{executablePath: strings.TrimSpace(executablePath), done: make(chan struct{})}
	if locator.executablePath != "" {
		close(locator.done)
		return locator
	}
	go locator.locate(ctx, find, persist)
	return locator
}

func (l *ExecutableLocator) locate(ctx context.Context, find executableFinder, persist executablePersister) {
	defer close(l.done)
	if ctx.Err() != nil || find == nil {
		return
	}
	executablePath, found := find()
	if !found {
		return
	}
	l.mu.Lock()
	l.executablePath = executablePath
	l.mu.Unlock()
	if persist != nil && ctx.Err() == nil {
		persist(executablePath)
	}
}

func (l *ExecutableLocator) ExecutablePath() string {
	if l == nil {
		return ""
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.executablePath
}

func (l *ExecutableLocator) Done() <-chan struct{} {
	if l == nil {
		return nil
	}
	return l.done
}

func findChromeExecutable() (string, bool) {
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome", "chrome.exe", "chromium.exe"} {
		if executable, err := exec.LookPath(name); err == nil {
			return executable, true
		}
	}
	return "", false
}
