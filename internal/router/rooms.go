package router

import "sync"

type RoomManager struct {
	cfg Config

	mu      sync.RWMutex
	roomCfg map[string]Config
	rooms   map[string]*Router
}

func NewRoomManager(cfg Config, roomCfg map[string]Config) *RoomManager {
	if roomCfg == nil {
		roomCfg = make(map[string]Config)
	}
	return &RoomManager{cfg: cfg, roomCfg: roomCfg, rooms: make(map[string]*Router)}
}

func (m *RoomManager) Get(room string) *Router {
	if room == "" {
		room = "default"
	}
	m.mu.RLock()
	r := m.rooms[room]
	cfg := m.cfg
	if rc, ok := m.roomCfg[room]; ok {
		cfg = rc
	}
	m.mu.RUnlock()
	if r != nil {
		return r
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if r = m.rooms[room]; r != nil {
		return r
	}
	cfg = m.cfg
	if rc, ok := m.roomCfg[room]; ok {
		cfg = rc
	}
	r = New(cfg)
	m.rooms[room] = r
	return r
}

func (m *RoomManager) UpdateConfig(cfg Config, roomCfg map[string]Config) {
	if roomCfg == nil {
		roomCfg = make(map[string]Config)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
	m.roomCfg = roomCfg
	for name, r := range m.rooms {
		rc := cfg
		if c2, ok := roomCfg[name]; ok {
			rc = c2
		}
		r.SetConfig(rc)
	}
}
