package application

import (
	"context"
	"errors"
	"sync"
	"time"

	operations "github.com/jcastilloa/goddgs-server/operations/domain"
)

var ErrUnknownProbeTarget = errors.New("unknown probe target")

type HealthMonitorConfig struct {
	SuccessThreshold int
	FailureThreshold int
}

type HealthPool interface {
	MarkHealthy(string)
	MarkUnhealthy(string)
}

type ProbeStore interface {
	RecordProbe(context.Context, operations.ProxyProbe) error
	RecordHealthTransition(context.Context, operations.ProxyHealthTransition) error
}

type HealthMonitor struct {
	mu     sync.Mutex
	states map[string]probeHealthState
	config HealthMonitorConfig
	pool   HealthPool
	store  ProbeStore
	now    func() time.Time
}

type probeHealthState struct {
	health          operations.ProxyHealth
	successes       int
	failures        int
	tunnel          bool
	tunnelConnected bool
	tunnelVersion   uint64
}

func NewHealthMonitor(targets []ProbeTarget, config HealthMonitorConfig, pool HealthPool, store ProbeStore, now func() time.Time) *HealthMonitor {
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = 1
	}
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 1
	}
	if now == nil {
		now = time.Now
	}
	monitor := &HealthMonitor{states: make(map[string]probeHealthState, len(targets)), config: config, pool: pool, store: store, now: now}
	for _, target := range targets {
		monitor.states[target.Name] = probeHealthState{health: operations.ProxyHealthUnknown, tunnel: target.Tunnel, tunnelConnected: true}
		monitor.updatePool(target.Name, operations.ProxyHealthUnknown)
	}
	return monitor
}

func (m *HealthMonitor) ApplyProbe(ctx context.Context, name string, observation ProbeObservation) error {
	return m.applyProbe(ctx, name, observation, 0)
}

func (m *HealthMonitor) ApplyProbeWithTunnelVersion(ctx context.Context, name string, observation ProbeObservation, tunnelVersion uint64) error {
	return m.applyProbe(ctx, name, observation, tunnelVersion)
}

func (m *HealthMonitor) applyProbe(ctx context.Context, name string, observation ProbeObservation, tunnelVersion uint64) error {
	m.mu.Lock()
	state, exists := m.states[name]
	if !exists {
		m.mu.Unlock()
		return ErrUnknownProbeTarget
	}
	if observation.ObservedAt.IsZero() {
		observation.ObservedAt = m.now().UTC()
	}
	if state.tunnel && tunnelVersion != 0 && tunnelVersion != state.tunnelVersion {
		m.mu.Unlock()
		return m.recordProbe(ctx, name, state.health, observation)
	}
	previous := state.health
	state = m.applyObservation(state, observation)
	m.states[name] = state
	m.updatePool(name, state.health)
	m.mu.Unlock()

	if err := m.recordProbe(ctx, name, state.health, observation); err != nil {
		return err
	}
	return m.recordTransition(ctx, name, previous, state.health, observation.ObservedAt)
}

func (m *HealthMonitor) recordProbe(ctx context.Context, name string, health operations.ProxyHealth, observation ProbeObservation) error {
	if m.store == nil {
		return nil
	}
	probe := operations.ProxyProbe{
		ProxyName:     name,
		Healthy:       isAvailable(health),
		Status:        health,
		Result:        probeResult(observation),
		HTTPStatus:    observation.HTTPStatus,
		ErrorCategory: observation.ErrorCategory,
		Duration:      observation.Duration,
		ObservedAt:    observation.ObservedAt,
	}
	return m.store.RecordProbe(writeContext(ctx), probe)
}

func (m *HealthMonitor) SetTunnelConnected(ctx context.Context, name string, connected bool) error {
	return m.setTunnelConnected(ctx, name, connected, 0)
}

func (m *HealthMonitor) UpdateTunnelConnection(ctx context.Context, name string, connected bool, version uint64) error {
	return m.setTunnelConnected(ctx, name, connected, version)
}

func (m *HealthMonitor) setTunnelConnected(ctx context.Context, name string, connected bool, version uint64) error {
	m.mu.Lock()
	state, exists := m.states[name]
	if !exists {
		m.mu.Unlock()
		return ErrUnknownProbeTarget
	}
	if !state.tunnel {
		m.mu.Unlock()
		return nil
	}
	if version != 0 && version <= state.tunnelVersion {
		m.mu.Unlock()
		return nil
	}
	if state.tunnelConnected == connected {
		if version != 0 {
			state.tunnelVersion = version
			m.states[name] = state
		}
		m.mu.Unlock()
		return nil
	}
	previous := state.health
	if version != 0 {
		state.tunnelVersion = version
	}
	state.tunnelConnected = connected
	state.successes = 0
	state.failures = 0
	if connected {
		state.health = operations.ProxyHealthUnknown
	} else {
		state.health = operations.ProxyHealthUnhealthy
	}
	m.states[name] = state
	m.updatePool(name, state.health)
	m.mu.Unlock()

	return m.recordTransition(ctx, name, previous, state.health, m.now().UTC())
}

func (m *HealthMonitor) Health(name string) operations.ProxyHealth {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.states[name].health
}

func (m *HealthMonitor) TunnelVersion(name string) uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.states[name].tunnelVersion
}

func (m *HealthMonitor) applyObservation(state probeHealthState, observation ProbeObservation) probeHealthState {
	if !state.tunnelConnected {
		state.health = operations.ProxyHealthUnhealthy
		return state
	}
	if observation.Success {
		state.successes++
		state.failures = 0
		if state.successes >= m.config.SuccessThreshold {
			state.health = operations.ProxyHealthHealthy
		}
		return state
	}
	state.successes = 0
	state.failures++
	if state.failures >= m.config.FailureThreshold {
		state.health = operations.ProxyHealthUnhealthy
		return state
	}
	state.health = operations.ProxyHealthDegraded
	return state
}

func (m *HealthMonitor) recordTransition(ctx context.Context, name string, previous, current operations.ProxyHealth, occurredAt time.Time) error {
	if previous == current || m.store == nil {
		return nil
	}
	return m.store.RecordHealthTransition(writeContext(ctx), operations.ProxyHealthTransition{
		ProxyName:  name,
		Healthy:    isAvailable(current),
		Status:     current,
		OccurredAt: occurredAt,
	})
}

func (m *HealthMonitor) updatePool(name string, health operations.ProxyHealth) {
	if m.pool == nil {
		return
	}
	if isAvailable(health) {
		m.pool.MarkHealthy(name)
		return
	}
	m.pool.MarkUnhealthy(name)
}

func isAvailable(health operations.ProxyHealth) bool {
	return health == operations.ProxyHealthHealthy || health == operations.ProxyHealthDegraded
}

func probeResult(observation ProbeObservation) operations.Result {
	if observation.Success {
		return operations.ResultSucceeded
	}
	switch observation.ErrorCategory {
	case operations.ErrorCanceled:
		return operations.ResultCanceled
	case operations.ErrorTimeout:
		return operations.ResultTimeout
	default:
		return operations.ResultFailed
	}
}
