package channellimit

import (
	"container/list"
	"context"
	"errors"
	"math"
	"sync"
	"time"
)

var (
	ErrQueueFull    = errors.New("channel_queue_full")
	ErrQueueTimeout = errors.New("channel_queue_timeout")
	ErrRPMExhausted = errors.New("channel_rpm_exhausted")
)

// Default is the process-local owner shared by routing and dispatch. The
// composition root can inject a separate Manager for an isolated gateway.
var Default = NewManager()

type Manager struct {
	mu          sync.Mutex
	states      map[int64]*state
	now         func() time.Time
	sweptMinute int64
}

type state struct {
	config   Config
	active   int64
	waiters  list.List
	minute   int64
	requests int64
	tokens   int64
	unknown  int64
}

type waiter struct {
	ready chan struct{}
}

// Permit covers the full logical channel attempt, including streaming. It
// owns no credential slot: callers acquire that separate guard after waiting.
type Permit struct {
	once   sync.Once
	m      *Manager
	id     int64
	Waited bool
}

type Snapshot struct {
	Config       Config  `json:"config"`
	Active       int64   `json:"active"`
	Waiting      int64   `json:"waiting"`
	Requests     int64   `json:"requests"`
	Tokens       int64   `json:"tokens"`
	UnknownUsage int64   `json:"unknownUsage"`
	Score        float64 `json:"score"`
	ExhaustedBy  string  `json:"exhaustedBy,omitempty"`
}

func NewManager() *Manager {
	return NewManagerWithClock(time.Now)
}

// NewManagerWithClock uses now for natural-minute accounting. Tests that need
// exact counters can hold a window steady or advance it explicitly.
func NewManagerWithClock(now func() time.Time) *Manager {
	return &Manager{states: make(map[int64]*state), now: now}
}

func (m *Manager) get(id int64) *state {
	// Lazy expiry bounds stale/deleted-channel counters without a background
	// goroutine. Live permits and queued requests retain their single owner.
	minute := m.now().Unix() / 60
	if m.sweptMinute != minute {
		for key, previous := range m.states {
			if previous.active == 0 && previous.waiters.Len() == 0 && previous.minute < minute {
				delete(m.states, key)
			}
		}
		m.sweptMinute = minute
	}
	s := m.states[id]
	if s == nil {
		s = &state{}
		m.states[id] = s
	}
	return s
}

func (m *Manager) window(s *state) {
	minute := m.now().Unix() / 60
	if s.minute != minute {
		s.minute, s.requests, s.tokens, s.unknown = minute, 0, 0, 0
	}
}

// Acquire updates policy without replacing live counters or waiters. A limit
// reduction takes effect as existing permits finish; it never evicts them.
func (m *Manager) Acquire(ctx context.Context, id int64, config Config) (*Permit, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	s := m.get(id)
	s.config = config
	m.grant(s)
	if s.waiters.Len() == 0 && (config.MaxConcurrent == 0 || s.active < config.MaxConcurrent) {
		s.active++
		m.mu.Unlock()
		return &Permit{m: m, id: id}, nil
	}
	if config.QueueSize > 0 && int64(s.waiters.Len()) >= config.QueueSize {
		m.mu.Unlock()
		return nil, ErrQueueFull
	}
	w := &waiter{ready: make(chan struct{})}
	element := s.waiters.PushBack(w)
	m.mu.Unlock()

	waitCtx := ctx
	if config.QueueTimeoutMs > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, time.Duration(config.QueueTimeoutMs)*time.Millisecond)
		defer cancel()
	}
	select {
	case <-w.ready:
		permit := &Permit{m: m, id: id, Waited: true}
		if err := ctx.Err(); err != nil {
			permit.Release()
			return nil, err
		}
		return permit, nil
	case <-waitCtx.Done():
		m.mu.Lock()
		select {
		case <-w.ready:
			s.active--
		default:
			s.waiters.Remove(element)
		}
		m.grant(s)
		m.mu.Unlock()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, ErrQueueTimeout
	}
}

// grant transfers ownership in FIFO order under the manager lock. Updating
// capacity may grant several waiters, without introducing worker goroutines.
func (m *Manager) grant(s *state) {
	for s.waiters.Len() > 0 && (s.config.MaxConcurrent == 0 || s.active < s.config.MaxConcurrent) {
		element := s.waiters.Front()
		s.waiters.Remove(element)
		s.active++
		close(element.Value.(*waiter).ready)
	}
}

func (p *Permit) Release() {
	if p == nil || p.m == nil {
		return
	}
	p.once.Do(func() {
		p.m.mu.Lock()
		defer p.m.mu.Unlock()
		if s := p.m.states[p.id]; s != nil {
			s.active--
			p.m.grant(s)
		}
	})
}

// TryRequest is the atomic RPM boundary immediately before an upstream
// attempt. TPM deliberately does not reject here: it is a load signal based
// on completed usage, matching the source contract.
func (m *Manager) TryRequest(ctx context.Context, id int64, config Config) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.get(id)
	m.window(s)
	if config.RPM > 0 && s.requests >= config.RPM {
		return ErrRPMExhausted
	}
	if config.RPM > 0 {
		s.requests++
	}
	return nil
}

// RecordUsage is called once with the final measured usage of an HTTP
// attempt, including partial/error responses. Unknown usage remains visible.
func (m *Manager) RecordUsage(id, tokens int64, known bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.get(id)
	m.window(s)
	if !known {
		s.unknown++
		return
	}
	if tokens > 0 {
		if tokens > math.MaxInt64-s.tokens {
			s.tokens = math.MaxInt64
		} else {
			s.tokens += tokens
		}
	}
}

// Snapshot is observational: it never reserves capacity or consumes RPM.
func (m *Manager) Snapshot(id int64, config Config) Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := Snapshot{Config: config, Score: 1}
	if s := m.states[id]; s != nil {
		result.Active, result.Waiting = s.active, int64(s.waiters.Len())
		if s.minute == m.now().Unix()/60 {
			result.Requests, result.Tokens, result.UnknownUsage = s.requests, s.tokens, s.unknown
		}
	}
	for _, dimension := range []struct {
		count, limit int64
		reason       string
	}{{result.Requests, config.RPM, "rpm"}, {result.Tokens, config.TPM, "tpm"}} {
		if dimension.limit <= 0 {
			continue
		}
		if dimension.count >= dimension.limit {
			result.Score, result.ExhaustedBy = 0, dimension.reason
			return result
		}
		result.Score = min(result.Score, 1-float64(dimension.count)/float64(dimension.limit))
	}
	if config.MaxConcurrent > 0 {
		switch {
		case result.Active < config.MaxConcurrent:
			result.Score = min(result.Score, 0.3+0.7*(1-float64(result.Active)/float64(config.MaxConcurrent)))
		case config.QueueSize > 0:
			if result.Waiting >= config.QueueSize {
				result.Score, result.ExhaustedBy = 0, "queue"
			} else {
				result.Score = min(result.Score, 0.3*(1-float64(result.Waiting)/float64(config.QueueSize)))
			}
		default:
			result.Score = min(result.Score, 0.3/(1+float64(result.Waiting)))
		}
	}
	return result
}
