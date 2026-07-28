package application

import (
	"context"
	"sync"
	"time"

	operations "github.com/jcastilloa/goddgs-server/operations/domain"
)

const defaultRetentionCheckInterval = time.Hour

type RetentionWorker struct {
	done   chan struct{}
	cancel context.CancelFunc
	once   sync.Once
}

type RetentionWorkerConfig struct {
	Retention     time.Duration
	CheckInterval time.Duration
	ReportError   func(error)
}

func StartRetentionWorker(ctx context.Context, repository operations.RetentionRepository, config RetentionWorkerConfig) *RetentionWorker {
	if config.CheckInterval <= 0 {
		config.CheckInterval = defaultRetentionCheckInterval
	}
	workerContext, cancel := context.WithCancel(ctx)
	worker := &RetentionWorker{done: make(chan struct{}), cancel: cancel}
	go worker.run(workerContext, repository, config)
	return worker
}

func (w *RetentionWorker) Done() <-chan struct{} { return w.done }

func (w *RetentionWorker) Stop() {
	w.once.Do(func() {
		w.cancel()
		<-w.done
	})
}

func (w *RetentionWorker) run(ctx context.Context, repository operations.RetentionRepository, config RetentionWorkerConfig) {
	defer close(w.done)
	ticker := time.NewTicker(config.CheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := repository.DeleteExpired(ctx, now.Add(-config.Retention)); err != nil && config.ReportError != nil {
				config.ReportError(err)
			}
		}
	}
}
