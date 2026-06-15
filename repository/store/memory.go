package store

import (
	"sync"
	"sync/atomic"
)

// MemoryStore is the in-process Store implementation.
//
// Per-node remaining is held in []atomic.Int64 (one cache line each in
// practice), so reservations on different nodes do not contend. The
// request_id ledger is a sync.Map; LoadOrStore + LoadAndDelete give us the
// atomic insert/remove semantics we need without a global lock.
type MemoryStore struct {
	nodes       []atomic.Int64
	allocations sync.Map
	budget      int64
}

func NewMemoryStore(n int, budget int64) *MemoryStore {
	s := &MemoryStore{
		nodes:  make([]atomic.Int64, n),
		budget: budget,
	}
	for i := range s.nodes {
		s.nodes[i].Store(budget)
	}
	return s
}

func (s *MemoryStore) NodeCount() int { return len(s.nodes) }
func (s *MemoryStore) Budget() int64  { return s.budget }

func (s *MemoryStore) SnapshotRemaining() []int64 {
	out := make([]int64, len(s.nodes))
	for i := range s.nodes {
		out[i] = s.nodes[i].Load()
	}
	return out
}

func (s *MemoryStore) ReserveOn(id int, expected, tokens int64) (int64, bool) {
	if !s.nodes[id].CompareAndSwap(expected, expected-tokens) {
		return 0, false
	}
	return expected - tokens, true
}

func (s *MemoryStore) ReleaseTo(id int, tokens int64) {
	s.nodes[id].Add(tokens)
}

func (s *MemoryStore) RemainingOf(id int) int64 {
	return s.nodes[id].Load()
}

func (s *MemoryStore) LoadAllocation(reqID string) (Allocation, bool) {
	v, ok := s.allocations.Load(reqID)
	if !ok {
		return Allocation{}, false
	}
	return v.(Allocation), true
}

func (s *MemoryStore) LoadOrStoreAllocation(reqID string, a Allocation) (Allocation, bool) {
	existing, loaded := s.allocations.LoadOrStore(reqID, a)
	if loaded {
		return existing.(Allocation), true
	}
	return a, false
}

func (s *MemoryStore) LoadAndDeleteAllocation(reqID string) (Allocation, bool) {
	v, ok := s.allocations.LoadAndDelete(reqID)
	if !ok {
		return Allocation{}, false
	}
	return v.(Allocation), true
}

func (s *MemoryStore) InFlight() int {
	n := 0
	s.allocations.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}
