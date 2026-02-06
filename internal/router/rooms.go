package router

import (
	"sync"
	"time"
)

type RoomManager struct {
	cfg Config

	mu           sync.RWMutex
	rooms        map[string]*Router
	errorTracker *ProducerErrorTracker
}

func NewRoomManager(cfg Config) *RoomManager {
	// Track up to 10 protocol errors per producer per 5 minutes before blocking
	errorTracker := NewProducerErrorTracker(5*time.Minute, 10)
	return &RoomManager{
		cfg:          cfg,
		rooms:        make(map[string]*Router),
		errorTracker: errorTracker,
	}
}

func (m *RoomManager) Get(room string) *Router {
	if room == "" {
		room = "default"
	}
	m.mu.RLock()
	r := m.rooms[room]
	m.mu.RUnlock()
	if r != nil {
		return r
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if r = m.rooms[room]; r != nil {
		return r
	}
	r = New(m.cfg)
	m.rooms[room] = r
	return r
}

func (m *RoomManager) UpdateConfig(cfg Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
	for _, r := range m.rooms {
		r.SetConfig(cfg)
	}
}
