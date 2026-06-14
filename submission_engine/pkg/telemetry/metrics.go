package telemetry

import "sync"

type CounterSet struct {
	mu       sync.RWMutex
	counters map[string]int64
}

func NewCounterSet() *CounterSet {
	return &CounterSet{counters: make(map[string]int64)}
}

func (m *CounterSet) Inc(name string) {
	m.Add(name, 1)
}

func (m *CounterSet) Add(name string, delta int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name] += delta
}

func (m *CounterSet) Snapshot() map[string]int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]int64, len(m.counters))
	for k, v := range m.counters {
		out[k] = v
	}
	return out
}
