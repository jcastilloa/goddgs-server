package chrome

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jcastilloa/goddgs-server/search/domain"
)

var ErrUnavailable = domain.ErrHTMLLoaderUnavailable

type Browser interface {
	Close() error
}

type BrowserFactory func(context.Context, string) (Browser, error)

type Timer interface {
	Stop() bool
}

type AfterFunc func(time.Duration, func()) Timer

type ManagerConfig struct {
	MaxBrowsers        int
	MaxPagesPerBrowser int
	IdleTimeout        time.Duration
	Factory            BrowserFactory
	AfterFunc          AfterFunc
}

type Manager struct {
	mu        sync.Mutex
	config    ManagerConfig
	valid     bool
	entries   map[string]*browserEntry
	context   context.Context
	cancel    context.CancelCauseFunc
	closed    bool
	wait      chan struct{}
	nextIdle  uint64
	creating  sync.WaitGroup
	closing   sync.WaitGroup
	closeOnce sync.Once
	closeDone chan struct{}
	closeErr  error
}

type browserEntry struct {
	browser  Browser
	pages    int
	idle     uint64
	timer    Timer
	creating bool
	closing  bool
	err      error
}

type Lease struct {
	manager *Manager
	key     string
	browser Browser
	once    sync.Once
}

func NewManager(config ManagerConfig) *Manager {
	if config.AfterFunc == nil {
		config.AfterFunc = func(timeout time.Duration, callback func()) Timer {
			return time.AfterFunc(timeout, callback)
		}
	}
	managerContext, cancel := context.WithCancelCause(context.Background())
	return &Manager{
		config:    config,
		valid:     config.MaxBrowsers > 0 && config.MaxPagesPerBrowser > 0 && config.IdleTimeout > 0,
		entries:   make(map[string]*browserEntry),
		context:   managerContext,
		cancel:    cancel,
		wait:      make(chan struct{}),
		closeDone: make(chan struct{}),
	}
}

func (m *Manager) Acquire(ctx context.Context, key, proxyURL string) (*Lease, error) {
	for {
		browser, wait, err := m.acquire(ctx, key, proxyURL)
		if err != nil {
			return nil, err
		}
		if browser != nil {
			return &Lease{manager: m, key: key, browser: browser}, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-wait:
		}
	}
}

func (m *Manager) acquire(ctx context.Context, key, proxyURL string) (Browser, <-chan struct{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if m.closed {
		return nil, nil, ErrUnavailable
	}
	if !m.valid {
		return nil, nil, ErrUnavailable
	}
	if entry := m.entries[key]; entry != nil {
		if entry.err != nil {
			err := entry.err
			delete(m.entries, key)
			m.signal()
			return nil, nil, err
		}
		if entry.creating || entry.closing || entry.pages >= m.config.MaxPagesPerBrowser {
			return nil, m.wait, nil
		}
		m.stopTimer(entry)
		entry.pages++
		return entry.browser, nil, nil
	}

	var eviction *browserEntry
	if len(m.entries) >= m.config.MaxBrowsers {
		eviction = m.evictIdle()
	}
	if len(m.entries) >= m.config.MaxBrowsers {
		if eviction != nil {
			m.closing.Add(1)
			go m.closeEvicted(eviction)
		}
		return nil, m.wait, nil
	}
	entry := &browserEntry{creating: true}
	m.entries[key] = entry
	m.creating.Add(1)
	go m.create(key, proxyURL, entry)
	return nil, m.wait, nil
}

func (m *Manager) closeEvicted(entry *browserEntry) {
	defer m.closing.Done()
	var err error
	if entry.browser != nil {
		err = entry.browser.Close()
	}
	m.mu.Lock()
	for key, current := range m.entries {
		if current == entry {
			delete(m.entries, key)
			break
		}
	}
	m.signal()
	m.mu.Unlock()
	m.recordCloseError(err)
}

func (m *Manager) create(key, proxyURL string, expected *browserEntry) {
	defer m.creating.Done()
	if m.config.Factory == nil {
		m.finishCreation(key, expected, nil, ErrUnavailable)
		return
	}
	browser, err := m.config.Factory(m.context, proxyURL)
	if err == nil && browser == nil {
		err = ErrUnavailable
	}
	m.finishCreation(key, expected, browser, err)
}

func (m *Manager) finishCreation(key string, expected *browserEntry, browser Browser, creationErr error) {
	m.mu.Lock()
	entry := m.entries[key]
	shouldClose := m.closed || entry != expected
	if !shouldClose && creationErr != nil {
		entry.creating = false
		entry.err = classifyError(ErrUnavailable, creationErr)
	} else if !shouldClose {
		entry.creating = false
		entry.browser = browser
		m.startIdleTimer(key, entry)
	}
	m.signal()
	m.mu.Unlock()

	if shouldClose && browser != nil {
		m.recordCloseError(browser.Close())
	}
}

func (m *Manager) evictIdle() *browserEntry {
	var evictionKey string
	var oldest uint64
	for key, entry := range m.entries {
		if entry.err != nil {
			delete(m.entries, key)
			continue
		}
		if entry.creating || entry.closing || entry.pages != 0 {
			continue
		}
		if evictionKey == "" || entry.idle < oldest {
			evictionKey = key
			oldest = entry.idle
		}
	}
	if evictionKey == "" {
		return nil
	}
	entry := m.entries[evictionKey]
	m.stopTimer(entry)
	entry.closing = true
	m.signal()
	return entry
}

func (l *Lease) Browser() Browser {
	return l.browser
}

func (l *Lease) Release() {
	l.once.Do(func() { l.manager.release(l.key, l.browser) })
}

func (m *Manager) release(key string, browser Browser) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.entries[key]
	if entry == nil || entry.browser != browser || entry.pages == 0 {
		return
	}
	entry.pages--
	if entry.pages == 0 && !m.closed {
		m.startIdleTimer(key, entry)
	}
	m.signal()
}

func (m *Manager) startIdleTimer(key string, entry *browserEntry) {
	m.stopTimer(entry)
	m.nextIdle++
	entry.idle = m.nextIdle
	idle := entry.idle
	entry.timer = m.config.AfterFunc(m.config.IdleTimeout, func() { m.expire(key, entry, idle) })
}

func (m *Manager) expire(key string, expected *browserEntry, idle uint64) {
	m.mu.Lock()
	entry := m.entries[key]
	if m.closed || entry != expected || entry.creating || entry.closing || entry.pages != 0 || entry.idle != idle {
		m.mu.Unlock()
		return
	}
	m.stopTimer(entry)
	entry.closing = true
	m.closing.Add(1)
	m.signal()
	m.mu.Unlock()
	m.closeEvicted(entry)
}

func (m *Manager) Close() error {
	m.closeOnce.Do(m.close)
	<-m.closeDone
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closeErr
}

func (m *Manager) close() {
	m.mu.Lock()
	m.closed = true
	m.cancel(ErrUnavailable)
	browsers := make([]Browser, 0, len(m.entries))
	for key, entry := range m.entries {
		m.stopTimer(entry)
		if entry.browser != nil && !entry.closing {
			browsers = append(browsers, entry.browser)
		}
		delete(m.entries, key)
	}
	m.signal()
	m.mu.Unlock()

	for _, browser := range browsers {
		m.recordCloseError(browser.Close())
	}
	m.creating.Wait()
	m.closing.Wait()
	close(m.closeDone)
}

func (m *Manager) stopTimer(entry *browserEntry) {
	if entry.timer == nil {
		return
	}
	entry.timer.Stop()
	entry.timer = nil
}

func (m *Manager) signal() {
	close(m.wait)
	m.wait = make(chan struct{})
}

func (m *Manager) recordCloseError(err error) {
	if err == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeErr = errors.Join(m.closeErr, err)
}
