package application

import (
	"context"
	"errors"
	"strings"
	"sync"
)

var (
	ErrInvalidPool    = errors.New("invalid proxy pool")
	ErrNoHealthyProxy = errors.New("no healthy proxy available")
)

type Entry[T any] struct {
	Key   string
	Value T
}

type Lease[T any] struct {
	Key   string
	Value T
}

type Pool[T any] struct {
	mu      sync.Mutex
	entries []poolEntry[T]
	next    int
}

type poolEntry[T any] struct {
	key     string
	value   T
	healthy bool
}

func NewPool[T any](entries []Entry[T]) (*Pool[T], error) {
	if len(entries) == 0 {
		return nil, ErrInvalidPool
	}

	pool := &Pool[T]{entries: make([]poolEntry[T], len(entries))}
	keys := make(map[string]struct{}, len(entries))
	for index, entry := range entries {
		key := strings.TrimSpace(entry.Key)
		if key == "" {
			return nil, ErrInvalidPool
		}
		if _, exists := keys[key]; exists {
			return nil, ErrInvalidPool
		}
		keys[key] = struct{}{}
		pool.entries[index] = poolEntry[T]{key: key, value: entry.Value, healthy: true}
	}

	return pool, nil
}

func (p *Pool[T]) Select(ctx context.Context) (Lease[T], error) {
	if err := ctx.Err(); err != nil {
		return Lease[T]{}, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for range p.entries {
		entry := p.entries[p.next]
		p.next = (p.next + 1) % len(p.entries)
		if entry.healthy {
			return Lease[T]{Key: entry.key, Value: entry.value}, nil
		}
	}

	return Lease[T]{}, ErrNoHealthyProxy
}

func (p *Pool[T]) MarkHealthy(key string) {
	p.setHealth(key, true)
}

func (p *Pool[T]) MarkUnhealthy(key string) {
	p.setHealth(key, false)
}

func (p *Pool[T]) setHealth(key string, healthy bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for index := range p.entries {
		if p.entries[index].key == key {
			p.entries[index].healthy = healthy
			return
		}
	}
}
