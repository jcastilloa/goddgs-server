package chrome

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestManagerCreatesBrowsersLazilyAndReusesProxyScopedCapacity(t *testing.T) {
	factory := &recordingBrowserFactory{}
	manager := NewManager(ManagerConfig{MaxBrowsers: 2, MaxPagesPerBrowser: 2, IdleTimeout: time.Minute, Factory: factory.New})
	t.Cleanup(func() { _ = manager.Close() })

	first, err := manager.Acquire(context.Background(), "proxy-a", "socks5h://127.0.0.1:9050")
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	second, err := manager.Acquire(context.Background(), "proxy-a", "socks5h://127.0.0.1:9050")
	if err != nil {
		t.Fatalf("second Acquire() error = %v", err)
	}
	if factory.Count() != 1 {
		t.Fatalf("browser creations = %d, want 1", factory.Count())
	}
	if first.Browser() != second.Browser() {
		t.Error("same proxy leases use different browser instances")
	}

	acquired := make(chan *Lease, 1)
	errs := make(chan error, 1)
	go func() {
		lease, acquireErr := manager.Acquire(context.Background(), "proxy-a", "socks5h://127.0.0.1:9050")
		if acquireErr != nil {
			errs <- acquireErr
			return
		}
		acquired <- lease
	}()
	assertBlocked(t, acquired, errs)

	first.Release()
	third := receiveLease(t, acquired, errs)
	if third.Browser() != second.Browser() {
		t.Error("available same-proxy capacity did not reuse browser")
	}
	second.Release()
	third.Release()

	fourth, err := manager.Acquire(context.Background(), "proxy-b", "socks5h://127.0.0.1:9051")
	if err != nil {
		t.Fatalf("different proxy Acquire() error = %v", err)
	}
	defer fourth.Release()
	if factory.Count() != 2 {
		t.Errorf("browser creations = %d, want 2", factory.Count())
	}
}

func TestManagerEvictsOnlyIdleBrowserAndHonorsWaitingCancellation(t *testing.T) {
	factory := &recordingBrowserFactory{}
	manager := NewManager(ManagerConfig{MaxBrowsers: 1, MaxPagesPerBrowser: 1, IdleTimeout: time.Minute, Factory: factory.New})
	t.Cleanup(func() { _ = manager.Close() })

	idle, err := manager.Acquire(context.Background(), "proxy-a", "")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	idleBrowser := idle.Browser().(*recordingBrowser)
	idle.Release()

	other, err := manager.Acquire(context.Background(), "proxy-b", "")
	if err != nil {
		t.Fatalf("Acquire() after idle eviction error = %v", err)
	}
	waitForClose(t, idleBrowser)
	if idleBrowser.CloseCount() != 1 {
		t.Errorf("idle browser closes = %d, want 1", idleBrowser.CloseCount())
	}
	if factory.Count() != 2 {
		t.Errorf("browser creations = %d, want 2", factory.Count())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errs := make(chan error, 1)
	go func() {
		_, acquireErr := manager.Acquire(ctx, "proxy-a", "")
		errs <- acquireErr
	}()
	assertBlocked(t, nil, errs)
	cancel()
	if err := <-errs; !errors.Is(err, context.Canceled) {
		t.Errorf("waiting Acquire() error = %v, want context.Canceled", err)
	}
	if factory.Count() != 2 {
		t.Errorf("browser creations after canceled wait = %d, want 2", factory.Count())
	}
	other.Release()
}

func waitForClose(t *testing.T, browser *recordingBrowser) {
	t.Helper()
	select {
	case <-browser.closed:
	case <-time.After(time.Second):
		t.Fatal("browser was not closed")
	}
}

func TestManagerEvictsLeastRecentlyIdleBrowser(t *testing.T) {
	factory := &recordingBrowserFactory{}
	manager := NewManager(ManagerConfig{MaxBrowsers: 2, MaxPagesPerBrowser: 1, IdleTimeout: time.Minute, Factory: factory.New})
	t.Cleanup(func() { _ = manager.Close() })

	first, err := manager.Acquire(context.Background(), "proxy-a", "")
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	firstBrowser := first.Browser().(*recordingBrowser)
	first.Release()

	second, err := manager.Acquire(context.Background(), "proxy-b", "")
	if err != nil {
		t.Fatalf("second Acquire() error = %v", err)
	}
	secondBrowser := second.Browser().(*recordingBrowser)
	second.Release()

	third, err := manager.Acquire(context.Background(), "proxy-c", "")
	if err != nil {
		t.Fatalf("third Acquire() error = %v", err)
	}
	defer third.Release()
	if firstBrowser.CloseCount() != 1 {
		t.Errorf("least recently idle browser closes = %d, want 1", firstBrowser.CloseCount())
	}
	if secondBrowser.CloseCount() != 0 {
		t.Errorf("most recently idle browser closes = %d, want 0", secondBrowser.CloseCount())
	}
}

func TestManagerExpiresIdleBrowsersAndClosesWaiters(t *testing.T) {
	clock := &manualClock{}
	factory := &recordingBrowserFactory{}
	manager := NewManager(ManagerConfig{
		MaxBrowsers:        1,
		MaxPagesPerBrowser: 1,
		IdleTimeout:        time.Minute,
		Factory:            factory.New,
		AfterFunc:          clock.AfterFunc,
	})

	lease, err := manager.Acquire(context.Background(), "proxy-a", "")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	browser := lease.Browser().(*recordingBrowser)
	lease.Release()
	clock.FireAll()
	if browser.CloseCount() != 1 {
		t.Errorf("idle browser closes = %d, want 1", browser.CloseCount())
	}

	busy, err := manager.Acquire(context.Background(), "proxy-a", "")
	if err != nil {
		t.Fatalf("Acquire() after expiration error = %v", err)
	}
	errs := make(chan error, 1)
	go func() {
		_, acquireErr := manager.Acquire(context.Background(), "proxy-b", "")
		errs <- acquireErr
	}()
	assertBlocked(t, nil, errs)
	if err := manager.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if err := <-errs; !errors.Is(err, ErrUnavailable) {
		t.Errorf("waiting Acquire() after Close error = %v, want ErrUnavailable", err)
	}
	if busy.Browser().(*recordingBrowser).CloseCount() != 1 {
		t.Error("active browser was not closed on shutdown")
	}
}

func TestManagerCloseWaitsForCreationAndClosesCreatedBrowser(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	browser := &recordingBrowser{}
	manager := NewManager(ManagerConfig{
		MaxBrowsers:        1,
		MaxPagesPerBrowser: 1,
		IdleTimeout:        time.Minute,
		Factory: func(context.Context, string) (Browser, error) {
			close(started)
			<-release
			return browser, nil
		},
	})

	errs := make(chan error, 1)
	go func() {
		_, err := manager.Acquire(context.Background(), "proxy-a", "")
		errs <- err
	}()
	<-started
	closed := make(chan error, 1)
	go func() { closed <- manager.Close() }()
	assertBlocked(t, nil, errs)
	select {
	case err := <-closed:
		t.Fatalf("Close() returned before creation finished: %v", err)
	default:
	}
	close(release)
	if err := <-closed; err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if browser.CloseCount() != 1 {
		t.Errorf("created browser closes = %d, want 1", browser.CloseCount())
	}
	if err := <-errs; err != nil && !errors.Is(err, ErrUnavailable) {
		t.Errorf("Acquire() error = %v, want nil or ErrUnavailable", err)
	}
}

func TestManagerExpiresBrowserWhenCreatorCancelsBeforeLaunchCompletes(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	clock := &manualClock{}
	browser := &recordingBrowser{closed: make(chan struct{})}
	manager := NewManager(ManagerConfig{
		MaxBrowsers:        1,
		MaxPagesPerBrowser: 1,
		IdleTimeout:        time.Minute,
		AfterFunc:          clock.AfterFunc,
		Factory: func(context.Context, string) (Browser, error) {
			close(started)
			<-release
			return browser, nil
		},
	})
	t.Cleanup(func() { _ = manager.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	errs := make(chan error, 1)
	go func() {
		_, err := manager.Acquire(ctx, "proxy-a", "")
		errs <- err
	}()
	<-started
	cancel()
	if err := <-errs; !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire() error = %v, want context.Canceled", err)
	}
	close(release)
	for clock.Count() == 0 {
		runtime.Gosched()
	}
	clock.FireAll()
	waitForClose(t, browser)
}

func TestManagerDoesNotExceedProcessCapacityWhileClosingAnEvictedBrowser(t *testing.T) {
	closeGate := make(chan struct{})
	first := &recordingBrowser{closed: make(chan struct{}), closeGate: closeGate}
	second := &recordingBrowser{closed: make(chan struct{})}
	creations := 0
	manager := NewManager(ManagerConfig{
		MaxBrowsers:        1,
		MaxPagesPerBrowser: 1,
		IdleTimeout:        time.Minute,
		Factory: func(context.Context, string) (Browser, error) {
			creations++
			if creations == 1 {
				return first, nil
			}
			return second, nil
		},
	})
	t.Cleanup(func() { _ = manager.Close() })

	lease, err := manager.Acquire(context.Background(), "proxy-a", "")
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	lease.Release()

	acquired := make(chan *Lease, 1)
	errs := make(chan error, 1)
	go func() {
		lease, acquireErr := manager.Acquire(context.Background(), "proxy-b", "")
		if acquireErr != nil {
			errs <- acquireErr
			return
		}
		acquired <- lease
	}()
	assertBlocked(t, acquired, errs)
	if creations != 1 {
		t.Fatalf("browser creations while eviction is closing = %d, want 1", creations)
	}

	close(closeGate)
	replacement := receiveLease(t, acquired, errs)
	replacement.Release()
	if creations != 2 {
		t.Errorf("browser creations after eviction closes = %d, want 2", creations)
	}
}

func TestManagerReturnsUnavailableWhenBrowserCannotStart(t *testing.T) {
	cause := errors.New("/private/chrome --proxy-server=socks5://user:password@example.test")
	manager := NewManager(ManagerConfig{
		MaxBrowsers:        1,
		MaxPagesPerBrowser: 1,
		IdleTimeout:        time.Minute,
		Factory: func(context.Context, string) (Browser, error) {
			return nil, cause
		},
	})
	t.Cleanup(func() { _ = manager.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := manager.Acquire(ctx, "proxy-a", "")
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("Acquire() error = %v, want ErrUnavailable", err)
	}
	if err != nil && err.Error() != ErrUnavailable.Error() {
		t.Errorf("Acquire() exposed runtime detail: %v", err)
	}
	if !errors.Is(err, cause) {
		t.Errorf("Acquire() error = %v, does not preserve launch cause", err)
	}
}

func TestManagerReclaimsFailedCreationAfterCreatorCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	creations := 0
	manager := NewManager(ManagerConfig{
		MaxBrowsers:        1,
		MaxPagesPerBrowser: 1,
		IdleTimeout:        time.Minute,
		Factory: func(context.Context, string) (Browser, error) {
			creations++
			if creations == 1 {
				close(started)
				<-release
				return nil, errors.New("browser did not start")
			}
			return &recordingBrowser{closed: make(chan struct{})}, nil
		},
	})
	t.Cleanup(func() { _ = manager.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	errs := make(chan error, 1)
	go func() {
		_, err := manager.Acquire(ctx, "proxy-a", "")
		errs <- err
	}()
	<-started
	cancel()
	if err := <-errs; !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire() error = %v, want context.Canceled", err)
	}
	close(release)

	lease, err := manager.Acquire(context.Background(), "proxy-b", "")
	if err != nil {
		t.Fatalf("Acquire() after failed creation error = %v", err)
	}
	lease.Release()
	if creations != 2 {
		t.Errorf("browser creations = %d, want 2", creations)
	}
}

func assertBlocked(t *testing.T, leases <-chan *Lease, errs <-chan error) {
	t.Helper()
	select {
	case lease := <-leases:
		t.Fatalf("Acquire() completed early with %#v", lease)
	case err := <-errs:
		t.Fatalf("Acquire() completed early with error %v", err)
	default:
	}
}

func receiveLease(t *testing.T, leases <-chan *Lease, errs <-chan error) *Lease {
	t.Helper()
	select {
	case lease := <-leases:
		return lease
	case err := <-errs:
		t.Fatalf("Acquire() error = %v", err)
		return nil
	case <-time.After(time.Second):
		t.Fatal("Acquire() did not resume")
		return nil
	}
}

type recordingBrowserFactory struct {
	mu       sync.Mutex
	browsers []*recordingBrowser
}

func (f *recordingBrowserFactory) New(_ context.Context, _ string) (Browser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	browser := &recordingBrowser{closed: make(chan struct{})}
	f.browsers = append(f.browsers, browser)
	return browser, nil
}

func (f *recordingBrowserFactory) Count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.browsers)
}

type recordingBrowser struct {
	mu        sync.Mutex
	closes    int
	closed    chan struct{}
	closeGate <-chan struct{}
	closeOnce sync.Once
}

func (b *recordingBrowser) Close() error {
	if b.closeGate != nil {
		<-b.closeGate
	}
	b.mu.Lock()
	b.closes++
	b.mu.Unlock()
	if b.closed != nil {
		b.closeOnce.Do(func() { close(b.closed) })
	}
	return nil
}

func (b *recordingBrowser) CloseCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closes
}

type manualClock struct {
	mu     sync.Mutex
	timers []*manualTimer
}

func (c *manualClock) AfterFunc(_ time.Duration, callback func()) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	timer := &manualTimer{callback: callback}
	c.timers = append(c.timers, timer)
	return timer
}

func (c *manualClock) FireAll() {
	c.mu.Lock()
	timers := append([]*manualTimer(nil), c.timers...)
	c.mu.Unlock()
	for _, timer := range timers {
		timer.Fire()
	}
}

func (c *manualClock) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

type manualTimer struct {
	mu       sync.Mutex
	stopped  bool
	callback func()
}

func (t *manualTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return false
	}
	t.stopped = true
	return true
}

func (t *manualTimer) Fire() {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return
	}
	t.stopped = true
	callback := t.callback
	t.mu.Unlock()
	callback()
}
