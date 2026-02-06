package router

import (
	"sync"
	"time"
)

// ProducerErrorTracker tracks protocol errors per producer to detect misbehaving clients
type ProducerErrorTracker struct {
	mu      sync.RWMutex
	errors  map[string]*producerErrorState
	window  time.Duration
	maxRate int // max errors per window before blocking
}

type producerErrorState struct {
	count     int
	firstSeen time.Time
	lastSeen  time.Time
	blocked   bool
}

func NewProducerErrorTracker(window time.Duration, maxRate int) *ProducerErrorTracker {
	return &ProducerErrorTracker{
		errors:  make(map[string]*producerErrorState),
		window:  window,
		maxRate: maxRate,
	}
}

// RecordError records a protocol error for a producer. Returns true if producer should be blocked.
func (t *ProducerErrorTracker) RecordError(producerKey string) bool {
	if t == nil {
		return false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	state, exists := t.errors[producerKey]

	if !exists {
		t.errors[producerKey] = &producerErrorState{
			count:     1,
			firstSeen: now,
			lastSeen:  now,
		}
		return false
	}

	// Reset if outside window
	if now.Sub(state.firstSeen) > t.window {
		state.count = 1
		state.firstSeen = now
		state.lastSeen = now
		state.blocked = false
		return false
	}

	state.count++
	state.lastSeen = now

	// Check if should block
	if state.count > t.maxRate {
		state.blocked = true
		return true
	}

	return false
}

// IsBlocked checks if a producer is currently blocked
func (t *ProducerErrorTracker) IsBlocked(producerKey string) bool {
	if t == nil {
		return false
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	state, exists := t.errors[producerKey]
	if !exists {
		return false
	}

	// Unblock if window has passed
	if time.Since(state.firstSeen) > t.window {
		return false
	}

	return state.blocked
}

// Cleanup removes old entries
func (t *ProducerErrorTracker) Cleanup() {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	for key, state := range t.errors {
		if now.Sub(state.lastSeen) > t.window*2 {
			delete(t.errors, key)
		}
	}
}
