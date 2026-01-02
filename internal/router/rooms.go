package router

import "sync"

type RoomManager struct {
	cfg Config

	mu    sync.RWMutex
	rooms map[string]*Router
}

func NewRoomManager(cfg Config) *RoomManager {
	return &RoomManager{cfg: cfg, rooms: make(map[string]*Router)}
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
