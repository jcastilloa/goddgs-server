package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRetentionWorkerStopsAfterContextCancellation(t *testing.T) {
	cleaner := &recordingCleaner{called: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	worker := StartRetentionWorker(ctx, cleaner, RetentionWorkerConfig{Retention: 24 * time.Hour, CheckInterval: time.Millisecond})

	select {
	case <-cleaner.called:
	case <-time.After(time.Second):
		t.Fatal("retention worker did not run")
	}

	cancel()
	select {
	case <-worker.Done():
	case <-time.After(time.Second):
		t.Fatal("retention worker did not stop")
	}
	worker.Stop()
}

func TestRetentionWorkerReportsCleanupErrors(t *testing.T) {
	want := errors.New("database unavailable")
	cleaner := &recordingCleaner{called: make(chan struct{}, 1), err: want}
	reported := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker := StartRetentionWorker(ctx, cleaner, RetentionWorkerConfig{
		Retention:     24 * time.Hour,
		CheckInterval: time.Millisecond,
		ReportError:   func(err error) { reported <- err },
	})
	defer worker.Stop()

	select {
	case got := <-reported:
		if !errors.Is(got, want) {
			t.Errorf("reported error = %v, want %v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("retention worker did not report the cleanup error")
	}
}

type recordingCleaner struct {
	mu     sync.Mutex
	called chan struct{}
	err    error
}

func (c *recordingCleaner) DeleteExpired(context.Context, time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case c.called <- struct{}{}:
	default:
	}
	return c.err
}
