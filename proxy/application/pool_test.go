package application

import (
	"context"
	"errors"
	"testing"
)

func TestPoolSelectsHealthyEntriesRoundRobin(t *testing.T) {
	pool := newStringPool(t, "direct-a", "direct-b")

	for _, want := range []string{"direct-a", "direct-b", "direct-a"} {
		lease, err := pool.Select(context.Background())
		if err != nil {
			t.Fatalf("Select() error = %v", err)
		}
		if lease.Key != want {
			t.Errorf("Select() key = %q, want %q", lease.Key, want)
		}
		if lease.Value != want+"-client" {
			t.Errorf("Select() value = %q, want %q", lease.Value, want+"-client")
		}
	}
}

func TestPoolSkipsUnhealthyEntryAndCanRestoreIt(t *testing.T) {
	pool := newStringPool(t, "direct-a", "tunnel-b")

	pool.MarkUnhealthy("direct-a")
	if pool.IsHealthy("direct-a") {
		t.Fatal("IsHealthy(direct-a) = true, want false")
	}
	lease, err := pool.Select(context.Background())
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if lease.Key != "tunnel-b" {
		t.Fatalf("Select() key = %q, want tunnel-b", lease.Key)
	}

	pool.MarkHealthy("direct-a")
	if !pool.IsHealthy("direct-a") {
		t.Fatal("IsHealthy(direct-a) = false, want true")
	}
	lease, err = pool.Select(context.Background())
	if err != nil {
		t.Fatalf("Select() after restore error = %v", err)
	}
	if lease.Key != "direct-a" {
		t.Errorf("Select() after restore key = %q, want direct-a", lease.Key)
	}
}

func TestPoolReportsUnknownEntryAsUnhealthy(t *testing.T) {
	pool := newStringPool(t, "direct-a")
	if pool.IsHealthy("missing") {
		t.Error("IsHealthy(missing) = true, want false")
	}
}

func TestPoolReturnsNoHealthyProxyWhenEveryEntryIsUnhealthy(t *testing.T) {
	pool := newStringPool(t, "direct-a", "tunnel-b")
	pool.MarkUnhealthy("direct-a")
	pool.MarkUnhealthy("tunnel-b")

	_, err := pool.Select(context.Background())
	if !errors.Is(err, ErrNoHealthyProxy) {
		t.Errorf("Select() error = %v, want ErrNoHealthyProxy", err)
	}
}

func TestPoolRejectsInvalidEntries(t *testing.T) {
	tests := []struct {
		name    string
		entries []Entry[string]
	}{
		{
			name: "empty pool",
		},
		{
			name: "blank key",
			entries: []Entry[string]{
				{Key: " ", Value: "client"},
			},
		},
		{
			name: "duplicate key",
			entries: []Entry[string]{
				{Key: "direct-a", Value: "first"},
				{Key: "direct-a", Value: "second"},
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := NewPool(testCase.entries)
			if !errors.Is(err, ErrInvalidPool) {
				t.Errorf("NewPool() error = %v, want ErrInvalidPool", err)
			}
		})
	}
}

func TestPoolHonorsCanceledContext(t *testing.T) {
	pool := newStringPool(t, "direct-a")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := pool.Select(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Select() error = %v, want context.Canceled", err)
	}
}

func newStringPool(t *testing.T, keys ...string) *Pool[string] {
	t.Helper()
	entries := make([]Entry[string], 0, len(keys))
	for _, key := range keys {
		entries = append(entries, Entry[string]{Key: key, Value: key + "-client"})
	}
	pool, err := NewPool(entries)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	return pool
}
