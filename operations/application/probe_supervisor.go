package application

import (
	"context"
	"sync"
	"time"
)

type HealthSupervisorConfig struct {
	Interval    time.Duration
	ProbeURL    string
	Targets     []ProbeTarget
	Prober      ProbeClient
	Monitor     *HealthMonitor
	ReportError func(error)
}

type HealthSupervisor struct {
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func StartHealthSupervisor(ctx context.Context, config HealthSupervisorConfig) *HealthSupervisor {
	if ctx == nil {
		ctx = context.Background()
	}
	if config.Interval <= 0 {
		config.Interval = time.Minute
	}
	runContext, cancel := context.WithCancel(ctx)
	supervisor := &HealthSupervisor{cancel: cancel, done: make(chan struct{})}
	go supervisor.run(runContext, config)
	return supervisor
}

func (s *HealthSupervisor) Stop() {
	s.once.Do(func() {
		s.cancel()
		<-s.done
	})
}

func (s *HealthSupervisor) run(ctx context.Context, config HealthSupervisorConfig) {
	defer close(s.done)
	s.runRound(ctx, config)
	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runRound(ctx, config)
		}
	}
}

func (s *HealthSupervisor) runRound(ctx context.Context, config HealthSupervisorConfig) {
	if config.Prober == nil || config.Monitor == nil {
		return
	}
	var group sync.WaitGroup
	for _, target := range config.Targets {
		group.Add(1)
		go func(target ProbeTarget) {
			defer group.Done()
			tunnelVersion := config.Monitor.TunnelVersion(target.Name)
			observation := config.Prober.Probe(ctx, target, config.ProbeURL)
			if err := config.Monitor.ApplyProbeWithTunnelVersion(ctx, target.Name, observation, tunnelVersion); err != nil && config.ReportError != nil {
				config.ReportError(err)
			}
		}(target)
	}
	group.Wait()
}
