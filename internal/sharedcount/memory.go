package sharedcount

import (
	"context"
	"sync"
	"time"
)

// MemoryCounter is the default single-process implementation.
type MemoryCounter struct {
	mu   sync.Mutex
	keys map[string]*memWindow
	now  func() time.Time
}

type memWindow struct {
	times  []int64    // unix ms — unit Incr events (RPM)
	points []memPoint // weighted IncrBy events (TPM)
}

type memPoint struct {
	atMs  int64
	delta int64
}

func (w *memWindow) pruneUnitEvents(start int64) {
	i := 0
	for i < len(w.times) && w.times[i] < start {
		i++
	}
	if i > 0 {
		w.times = append([]int64(nil), w.times[i:]...)
	}
}

func NewMemoryCounter() *MemoryCounter {
	return &MemoryCounter{keys: make(map[string]*memWindow), now: time.Now}
}

func (m *MemoryCounter) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	_ = ctx
	if window <= 0 {
		window = time.Minute
	}
	nowMs := m.now().UnixMilli()
	start := nowMs - window.Milliseconds()
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.keys[key]
	if w == nil {
		w = &memWindow{}
		m.keys[key] = w
	}
	w.pruneUnitEvents(start)
	w.times = append(w.times, nowMs)
	return int64(len(w.times)), nil
}

func (m *MemoryCounter) Decr(ctx context.Context, key string, window time.Duration) (int64, error) {
	_ = ctx
	if window <= 0 {
		window = time.Minute
	}
	nowMs := m.now().UnixMilli()
	start := nowMs - window.Milliseconds()
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.keys[key]
	if w == nil {
		return 0, nil
	}
	w.pruneUnitEvents(start)
	// Compensating rollback: drop the most recent unit event when present.
	if len(w.times) > 0 {
		w.times = w.times[:len(w.times)-1]
	}
	return int64(len(w.times)), nil
}

func (m *MemoryCounter) IncrBy(ctx context.Context, key string, delta int64, window time.Duration) (int64, error) {
	_ = ctx
	if window <= 0 {
		window = time.Minute
	}
	nowMs := m.now().UnixMilli()
	start := nowMs - window.Milliseconds()
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.keys[key]
	if w == nil {
		w = &memWindow{}
		m.keys[key] = w
	}
	// prune
	i := 0
	for i < len(w.points) && w.points[i].atMs < start {
		i++
	}
	if i > 0 {
		w.points = append([]memPoint(nil), w.points[i:]...)
	}
	// delta==0 is a read-only peek; positive reserves; negative rolls back.
	if delta != 0 {
		w.points = append(w.points, memPoint{atMs: nowMs, delta: delta})
	}
	var sum int64
	for _, p := range w.points {
		sum += p.delta
	}
	if sum < 0 {
		// Keep memory totals non-negative for admission math.
		sum = 0
		w.points = nil
	}
	return sum, nil
}

func (m *MemoryCounter) Get(ctx context.Context, key string) (int64, error) {
	_ = ctx
	nowMs := m.now().UnixMilli()
	start := nowMs - 60_000
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.keys[key]
	if w == nil {
		return 0, nil
	}
	n := 0
	for _, t := range w.times {
		if t >= start {
			n++
		}
	}
	return int64(n), nil
}
