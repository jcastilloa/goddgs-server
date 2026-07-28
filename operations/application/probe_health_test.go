package application

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	operations "github.com/jcastilloa/goddgs-server/operations/domain"
)

func TestHealthMonitorAppliesConfiguredHysteresis(t *testing.T) {
	tests := []struct {
		name            string
		observations    []ProbeObservation
		wantStates      []operations.ProxyHealth
		wantTransitions []operations.ProxyHealth
		wantAvailable   bool
	}{
		{
			name: "unknown becomes healthy after consecutive successes",
			observations: []ProbeObservation{
				successfulProbe(),
				successfulProbe(),
			},
			wantStates:      []operations.ProxyHealth{operations.ProxyHealthUnknown, operations.ProxyHealthHealthy},
			wantTransitions: []operations.ProxyHealth{operations.ProxyHealthHealthy},
			wantAvailable:   true,
		},
		{
			name: "single failure becomes degraded",
			observations: []ProbeObservation{
				failedProbe(operations.ErrorTransport),
			},
			wantStates:      []operations.ProxyHealth{operations.ProxyHealthDegraded},
			wantTransitions: []operations.ProxyHealth{operations.ProxyHealthDegraded},
			wantAvailable:   true,
		},
		{
			name: "consecutive failures become unhealthy",
			observations: []ProbeObservation{
				successfulProbe(),
				successfulProbe(),
				failedProbe(operations.ErrorTransport),
				failedProbe(operations.ErrorTransport),
			},
			wantStates: []operations.ProxyHealth{
				operations.ProxyHealthUnknown,
				operations.ProxyHealthHealthy,
				operations.ProxyHealthDegraded,
				operations.ProxyHealthUnhealthy,
			},
			wantTransitions: []operations.ProxyHealth{operations.ProxyHealthHealthy, operations.ProxyHealthDegraded, operations.ProxyHealthUnhealthy},
			wantAvailable:   false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			pool := &recordingHealthPool{}
			store := &recordingProbeStore{}
			monitor := NewHealthMonitor([]ProbeTarget{{Name: "proxy"}}, HealthMonitorConfig{SuccessThreshold: 2, FailureThreshold: 2}, pool, store, fixedNow)
			for index, observation := range testCase.observations {
				if err := monitor.ApplyProbe(context.Background(), "proxy", observation); err != nil {
					t.Fatalf("ApplyProbe(%d) error = %v", index, err)
				}
				if got := monitor.Health("proxy"); got != testCase.wantStates[index] {
					t.Errorf("health after observation %d = %q, want %q", index, got, testCase.wantStates[index])
				}
			}
			if got := store.transitionStates(); !equalHealthStates(got, testCase.wantTransitions) {
				t.Errorf("transitions = %#v, want %#v", got, testCase.wantTransitions)
			}
			if got := pool.available("proxy"); got != testCase.wantAvailable {
				t.Errorf("available = %v, want %v", got, testCase.wantAvailable)
			}
			if got, want := len(store.probes), len(testCase.observations); got != want {
				t.Errorf("persisted probes = %d, want %d", got, want)
			}
		})
	}
}

func TestHealthMonitorForcesTunnelDisconnectionAndRequiresProbeForRecovery(t *testing.T) {
	pool := &recordingHealthPool{}
	store := &recordingProbeStore{}
	monitor := NewHealthMonitor([]ProbeTarget{{Name: "tunnel", Tunnel: true}}, HealthMonitorConfig{SuccessThreshold: 1, FailureThreshold: 2}, pool, store, fixedNow)

	if err := monitor.SetTunnelConnected(context.Background(), "tunnel", false); err != nil {
		t.Fatalf("SetTunnelConnected(false) error = %v", err)
	}
	if got := monitor.Health("tunnel"); got != operations.ProxyHealthUnhealthy {
		t.Fatalf("health after disconnect = %q, want unhealthy", got)
	}
	if err := monitor.SetTunnelConnected(context.Background(), "tunnel", true); err != nil {
		t.Fatalf("SetTunnelConnected(true) error = %v", err)
	}
	if got := monitor.Health("tunnel"); got != operations.ProxyHealthUnknown {
		t.Fatalf("health after reconnect = %q, want unknown", got)
	}
	if pool.available("tunnel") {
		t.Fatal("tunnel is available after reconnect without a probe")
	}
	if err := monitor.ApplyProbe(context.Background(), "tunnel", successfulProbe()); err != nil {
		t.Fatalf("ApplyProbe() error = %v", err)
	}
	if got := monitor.Health("tunnel"); got != operations.ProxyHealthHealthy {
		t.Errorf("health after successful probe = %q, want healthy", got)
	}
	if got, want := store.transitionStates(), []operations.ProxyHealth{operations.ProxyHealthUnhealthy, operations.ProxyHealthUnknown, operations.ProxyHealthHealthy}; !equalHealthStates(got, want) {
		t.Errorf("transitions = %#v, want %#v", got, want)
	}
}

func TestHealthMonitorPersistsEachTransitionOnlyOnce(t *testing.T) {
	store := &recordingProbeStore{}
	monitor := NewHealthMonitor([]ProbeTarget{{Name: "proxy"}}, HealthMonitorConfig{SuccessThreshold: 1, FailureThreshold: 2}, &recordingHealthPool{}, store, fixedNow)
	if err := monitor.ApplyProbe(context.Background(), "proxy", successfulProbe()); err != nil {
		t.Fatal(err)
	}
	if err := monitor.ApplyProbe(context.Background(), "proxy", successfulProbe()); err != nil {
		t.Fatal(err)
	}
	if got, want := store.transitionStates(), []operations.ProxyHealth{operations.ProxyHealthHealthy}; !equalHealthStates(got, want) {
		t.Errorf("transitions = %#v, want %#v", got, want)
	}
}

func TestHealthMonitorRequiresSuccessesToRecoverAnUnhealthyProxy(t *testing.T) {
	pool := &recordingHealthPool{}
	store := &recordingProbeStore{}
	monitor := NewHealthMonitor([]ProbeTarget{{Name: "proxy"}}, HealthMonitorConfig{SuccessThreshold: 2, FailureThreshold: 2}, pool, store, fixedNow)

	for _, observation := range []ProbeObservation{
		failedProbe(operations.ErrorTransport),
		failedProbe(operations.ErrorTransport),
		failedProbe(operations.ErrorTransport),
	} {
		if err := monitor.ApplyProbe(context.Background(), "proxy", observation); err != nil {
			t.Fatal(err)
		}
	}
	if got := monitor.Health("proxy"); got != operations.ProxyHealthUnhealthy {
		t.Fatalf("health after another failed probe = %q, want unhealthy", got)
	}
	if pool.available("proxy") {
		t.Fatal("unhealthy proxy became available after another failed probe")
	}

	if err := monitor.ApplyProbe(context.Background(), "proxy", successfulProbe()); err != nil {
		t.Fatal(err)
	}
	if got := monitor.Health("proxy"); got != operations.ProxyHealthUnhealthy {
		t.Fatalf("health after one recovery success = %q, want unhealthy", got)
	}
	if err := monitor.ApplyProbe(context.Background(), "proxy", successfulProbe()); err != nil {
		t.Fatal(err)
	}
	if got := monitor.Health("proxy"); got != operations.ProxyHealthHealthy {
		t.Errorf("health after confirmed recovery = %q, want healthy", got)
	}
	if got, want := store.transitionStates(), []operations.ProxyHealth{operations.ProxyHealthDegraded, operations.ProxyHealthUnhealthy, operations.ProxyHealthHealthy}; !equalHealthStates(got, want) {
		t.Errorf("transitions = %#v, want %#v", got, want)
	}
}

func TestHealthMonitorPersistsCanceledProbeAfterContextCancellation(t *testing.T) {
	store := &contextAwareProbeStore{}
	monitor := NewHealthMonitor([]ProbeTarget{{Name: "proxy"}}, HealthMonitorConfig{SuccessThreshold: 1, FailureThreshold: 1}, &recordingHealthPool{}, store, fixedNow)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := monitor.ApplyProbe(ctx, "proxy", failedProbe(operations.ErrorCanceled)); err != nil {
		t.Fatalf("ApplyProbe() error = %v", err)
	}
	if store.probeContextCanceled || store.transitionContextCanceled {
		t.Error("probe persistence received a canceled context")
	}
}

func TestHealthMonitorIgnoresStaleTunnelConnectionSignals(t *testing.T) {
	pool := &recordingHealthPool{}
	monitor := NewHealthMonitor([]ProbeTarget{{Name: "tunnel", Tunnel: true}}, HealthMonitorConfig{SuccessThreshold: 1, FailureThreshold: 1}, pool, &recordingProbeStore{}, fixedNow)
	if err := monitor.UpdateTunnelConnection(context.Background(), "tunnel", true, 2); err != nil {
		t.Fatal(err)
	}
	if err := monitor.UpdateTunnelConnection(context.Background(), "tunnel", false, 1); err != nil {
		t.Fatal(err)
	}
	if got := monitor.Health("tunnel"); got != operations.ProxyHealthUnknown {
		t.Errorf("health after stale signal = %q, want unknown", got)
	}
	if pool.available("tunnel") {
		t.Error("tunnel became available without a successful probe")
	}
}

func TestHealthMonitorDoesNotResetProbeProgressForRepeatedTunnelSignal(t *testing.T) {
	monitor := NewHealthMonitor([]ProbeTarget{{Name: "tunnel", Tunnel: true}}, HealthMonitorConfig{SuccessThreshold: 2, FailureThreshold: 2}, &recordingHealthPool{}, &recordingProbeStore{}, fixedNow)
	if err := monitor.UpdateTunnelConnection(context.Background(), "tunnel", false, 1); err != nil {
		t.Fatal(err)
	}
	if err := monitor.UpdateTunnelConnection(context.Background(), "tunnel", true, 2); err != nil {
		t.Fatal(err)
	}
	if err := monitor.ApplyProbe(context.Background(), "tunnel", successfulProbe()); err != nil {
		t.Fatal(err)
	}
	if err := monitor.UpdateTunnelConnection(context.Background(), "tunnel", true, 3); err != nil {
		t.Fatal(err)
	}
	if err := monitor.ApplyProbe(context.Background(), "tunnel", successfulProbe()); err != nil {
		t.Fatal(err)
	}
	if got := monitor.Health("tunnel"); got != operations.ProxyHealthHealthy {
		t.Errorf("health = %q, want healthy", got)
	}
}

func TestHealthMonitorDoesNotRecoverFromProbeStartedBeforeTunnelReconnect(t *testing.T) {
	pool := &recordingHealthPool{}
	monitor := NewHealthMonitor([]ProbeTarget{{Name: "tunnel", Tunnel: true}}, HealthMonitorConfig{SuccessThreshold: 1, FailureThreshold: 1}, pool, &recordingProbeStore{}, fixedNow)
	if err := monitor.UpdateTunnelConnection(context.Background(), "tunnel", false, 1); err != nil {
		t.Fatal(err)
	}
	if err := monitor.UpdateTunnelConnection(context.Background(), "tunnel", true, 2); err != nil {
		t.Fatal(err)
	}
	if err := monitor.ApplyProbeWithTunnelVersion(context.Background(), "tunnel", ProbeObservation{Success: true, HTTPStatus: http.StatusNoContent, ObservedAt: fixedNow()}, 1); err != nil {
		t.Fatal(err)
	}
	if got := monitor.Health("tunnel"); got != operations.ProxyHealthUnknown {
		t.Errorf("health after stale probe = %q, want unknown", got)
	}
	if pool.available("tunnel") {
		t.Error("tunnel became available from a stale probe")
	}
}

func TestHealthSupervisorRejectsProbeStartedBeforeTunnelStateChanged(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	persisted := make(chan struct{}, 1)
	pool := &recordingHealthPool{}
	monitor := NewHealthMonitor([]ProbeTarget{{Name: "tunnel", Tunnel: true}}, HealthMonitorConfig{SuccessThreshold: 1, FailureThreshold: 1}, pool, notifyingProbeStore{recorded: persisted}, fixedNow)
	if err := monitor.UpdateTunnelConnection(context.Background(), "tunnel", false, 1); err != nil {
		t.Fatal(err)
	}
	if err := monitor.UpdateTunnelConnection(context.Background(), "tunnel", true, 2); err != nil {
		t.Fatal(err)
	}
	supervisor := StartHealthSupervisor(context.Background(), HealthSupervisorConfig{
		Interval: time.Hour,
		Targets:  []ProbeTarget{{Name: "tunnel", Tunnel: true}},
		Prober:   blockingSuccessProber{started: started, release: release},
		Monitor:  monitor,
	})
	defer supervisor.Stop()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("probe did not start")
	}
	if err := monitor.UpdateTunnelConnection(context.Background(), "tunnel", false, 3); err != nil {
		t.Fatal(err)
	}
	if err := monitor.UpdateTunnelConnection(context.Background(), "tunnel", true, 4); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case <-persisted:
	case <-time.After(time.Second):
		t.Fatal("stale probe was not persisted")
	}
	if got := monitor.Health("tunnel"); got != operations.ProxyHealthUnknown {
		t.Errorf("health after stale probe = %q, want unknown", got)
	}
	if pool.available("tunnel") {
		t.Error("tunnel became available from a stale probe")
	}
}

func TestHealthMonitorKeepsPoolConsistentWhenTunnelFailsDuringProbeUpdate(t *testing.T) {
	pool := newBlockingHealthPool()
	monitor := NewHealthMonitor([]ProbeTarget{{Name: "tunnel", Tunnel: true}}, HealthMonitorConfig{SuccessThreshold: 1, FailureThreshold: 1}, pool, &recordingProbeStore{}, fixedNow)
	if err := monitor.UpdateTunnelConnection(context.Background(), "tunnel", false, 1); err != nil {
		t.Fatal(err)
	}
	if err := monitor.UpdateTunnelConnection(context.Background(), "tunnel", true, 2); err != nil {
		t.Fatal(err)
	}

	probeDone := make(chan error, 1)
	go func() {
		probeDone <- monitor.ApplyProbe(context.Background(), "tunnel", successfulProbe())
	}()
	select {
	case <-pool.markHealthyStarted:
	case <-time.After(time.Second):
		t.Fatal("probe did not begin its pool update")
	}

	tunnelDone := make(chan error, 1)
	go func() {
		tunnelDone <- monitor.UpdateTunnelConnection(context.Background(), "tunnel", false, 3)
	}()
	select {
	case err := <-tunnelDone:
		t.Fatalf("tunnel update completed before the in-flight pool update: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(pool.releaseMarkHealthy)
	if err := <-probeDone; err != nil {
		t.Fatal(err)
	}
	if err := <-tunnelDone; err != nil {
		t.Fatal(err)
	}
	if got := monitor.Health("tunnel"); got != operations.ProxyHealthUnhealthy {
		t.Errorf("health = %q, want unhealthy", got)
	}
	if pool.available("tunnel") {
		t.Error("pool kept the tunnel available after its failure")
	}
}

func TestHealthSupervisorCancelsActiveRoundOnStop(t *testing.T) {
	started := make(chan struct{})
	monitor := NewHealthMonitor([]ProbeTarget{{Name: "proxy"}}, HealthMonitorConfig{SuccessThreshold: 1, FailureThreshold: 1}, &recordingHealthPool{}, &recordingProbeStore{}, fixedNow)
	supervisor := StartHealthSupervisor(context.Background(), HealthSupervisorConfig{
		Interval: time.Hour,
		Targets:  []ProbeTarget{{Name: "proxy"}},
		Prober:   blockingProber{started: started},
		Monitor:  monitor,
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("probe round did not start")
	}
	done := make(chan struct{})
	go func() {
		supervisor.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop() did not wait for the active probe round")
	}
}

func successfulProbe() ProbeObservation {
	return ProbeObservation{Success: true, HTTPStatus: http.StatusNoContent, ObservedAt: fixedNow()}
}

func failedProbe(category operations.ErrorCategory) ProbeObservation {
	return ProbeObservation{ErrorCategory: category, ObservedAt: fixedNow()}
}

func fixedNow() time.Time {
	return time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
}

type recordingHealthPool struct {
	mu        sync.Mutex
	healthMap map[string]bool
}

func (p *recordingHealthPool) MarkHealthy(name string) {
	p.set(name, true)
}

func (p *recordingHealthPool) MarkUnhealthy(name string) {
	p.set(name, false)
}

func (p *recordingHealthPool) set(name string, available bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.healthMap == nil {
		p.healthMap = make(map[string]bool)
	}
	p.healthMap[name] = available
}

func (p *recordingHealthPool) available(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.healthMap[name]
}

type recordingProbeStore struct {
	mu          sync.Mutex
	probes      []operations.ProxyProbe
	transitions []operations.ProxyHealthTransition
}

type contextAwareProbeStore struct {
	probeContextCanceled      bool
	transitionContextCanceled bool
}

func (s *contextAwareProbeStore) RecordProbe(ctx context.Context, _ operations.ProxyProbe) error {
	s.probeContextCanceled = ctx.Err() != nil
	return nil
}

func (s *contextAwareProbeStore) RecordHealthTransition(ctx context.Context, _ operations.ProxyHealthTransition) error {
	s.transitionContextCanceled = ctx.Err() != nil
	return nil
}

func (s *recordingProbeStore) RecordProbe(_ context.Context, probe operations.ProxyProbe) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probes = append(s.probes, probe)
	return nil
}

func (s *recordingProbeStore) RecordHealthTransition(_ context.Context, transition operations.ProxyHealthTransition) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transitions = append(s.transitions, transition)
	return nil
}

func (s *recordingProbeStore) transitionStates() []operations.ProxyHealth {
	s.mu.Lock()
	defer s.mu.Unlock()
	states := make([]operations.ProxyHealth, len(s.transitions))
	for index, transition := range s.transitions {
		states[index] = transition.Status
	}
	return states
}

type blockingProber struct{ started chan<- struct{} }

func (p blockingProber) Probe(ctx context.Context, _ ProbeTarget, _ string) ProbeObservation {
	p.started <- struct{}{}
	<-ctx.Done()
	return ProbeObservation{ErrorCategory: operations.ErrorCanceled, ObservedAt: fixedNow()}
}

type blockingSuccessProber struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (p blockingSuccessProber) Probe(_ context.Context, _ ProbeTarget, _ string) ProbeObservation {
	p.started <- struct{}{}
	<-p.release
	return successfulProbe()
}

type notifyingProbeStore struct{ recorded chan<- struct{} }

func (s notifyingProbeStore) RecordProbe(context.Context, operations.ProxyProbe) error {
	s.recorded <- struct{}{}
	return nil
}

func (notifyingProbeStore) RecordHealthTransition(context.Context, operations.ProxyHealthTransition) error {
	return nil
}

type blockingHealthPool struct {
	recordingHealthPool
	markHealthyStarted chan struct{}
	releaseMarkHealthy chan struct{}
}

func newBlockingHealthPool() *blockingHealthPool {
	return &blockingHealthPool{
		markHealthyStarted: make(chan struct{}),
		releaseMarkHealthy: make(chan struct{}),
	}
}

func (p *blockingHealthPool) MarkHealthy(name string) {
	close(p.markHealthyStarted)
	<-p.releaseMarkHealthy
	p.recordingHealthPool.MarkHealthy(name)
}

func equalHealthStates(got, want []operations.ProxyHealth) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
