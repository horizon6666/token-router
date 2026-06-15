package allocator

import (
	"runtime"
	"sort"

	"token-router/global/berror"
	"token-router/global/consts"
	"token-router/repository/store"
)

// Result is the outcome of a successful Alloc call.
type Result struct {
	NodeID    int
	Remaining int64
	Duplicate bool
}

// Allocator is the small interface used by the controller layer. Defined here
// so callers can mock it in tests; the concrete type below is the only
// production implementation.
type Allocator interface {
	Alloc(reqID string, tokens int64) (Result, berror.Error)
	Free(reqID string) (nodeID int, err berror.Error)
	Status() Status
}

// Status is what /debug/status returns.
type Status struct {
	Nodes    []NodeStatus
	InFlight int
	Budget   int64
}

type NodeStatus struct {
	ID        int
	Remaining int64
}

type bestFitAllocator struct {
	store store.Store
}

func New(s store.Store) Allocator {
	return &bestFitAllocator{store: s}
}

// Alloc places `tokens` for `reqID` using a best-fit policy.
//
// Idempotency: if reqID was already allocated, returns the original placement
// with Duplicate=true (no new deduction). The controller signals this via the
// X-Allocation-Duplicate response header so the JSON body stays compliant with
// the prompt contract.
//
// Concurrency: per-node remaining is updated via CAS; if all best-fit
// candidates lose their CAS race, we re-snapshot and retry up to
// consts.MaxAllocRetry times. Duplicate request_ids that race in concurrently
// are caught by LoadOrStoreAllocation: the loser refunds its deduction and
// returns the winner's placement as duplicate.
func (a *bestFitAllocator) Alloc(reqID string, tokens int64) (Result, berror.Error) {
	if tokens <= 0 || tokens > a.store.Budget() {
		return Result{}, berror.ErrInvalid
	}

	if existing, ok := a.store.LoadAllocation(reqID); ok {
		return Result{
			NodeID:    existing.NodeID,
			Remaining: a.store.RemainingOf(existing.NodeID),
			Duplicate: true,
		}, nil
	}

	type candidate struct {
		id        int
		remaining int64
	}
	cands := make([]candidate, 0, a.store.NodeCount())

	for retry := 0; retry < consts.MaxAllocRetry; retry++ {
		cands = cands[:0]
		for _, rem := range withIndex(a.store.SnapshotRemaining()) {
			if rem.value >= tokens {
				cands = append(cands, candidate{id: rem.idx, remaining: rem.value})
			}
		}
		if len(cands) == 0 {
			return Result{}, berror.ErrOverloaded
		}
		// Best-fit: prefer the smallest remaining that still fits, leaving
		// large contiguous capacity for later large requests.
		sort.Slice(cands, func(i, j int) bool {
			return cands[i].remaining < cands[j].remaining
		})

		for _, c := range cands {
			newRem, ok := a.store.ReserveOn(c.id, c.remaining, tokens)
			if !ok {
				continue
			}
			existing, loaded := a.store.LoadOrStoreAllocation(reqID, store.Allocation{
				NodeID: c.id,
				Tokens: tokens,
			})
			if loaded {
				// Same reqID raced in. Refund our deduction and surface the
				// existing placement.
				a.store.ReleaseTo(c.id, tokens)
				return Result{
					NodeID:    existing.NodeID,
					Remaining: a.store.RemainingOf(existing.NodeID),
					Duplicate: true,
				}, nil
			}
			return Result{NodeID: c.id, Remaining: newRem, Duplicate: false}, nil
		}
		// All CAS candidates lost their race; yield once to reduce thrashing
		// then refresh snapshot and retry.
		runtime.Gosched()
	}
	return Result{}, berror.ErrOverloaded
}

// Free releases the reservation for reqID.
func (a *bestFitAllocator) Free(reqID string) (int, berror.Error) {
	alloc, ok := a.store.LoadAndDeleteAllocation(reqID)
	if !ok {
		return 0, berror.ErrNotFound
	}
	a.store.ReleaseTo(alloc.NodeID, alloc.Tokens)
	return alloc.NodeID, nil
}

func (a *bestFitAllocator) Status() Status {
	snap := a.store.SnapshotRemaining()
	nodes := make([]NodeStatus, len(snap))
	for i, rem := range snap {
		nodes[i] = NodeStatus{ID: i, Remaining: rem}
	}
	return Status{Nodes: nodes, InFlight: a.store.InFlight(), Budget: a.store.Budget()}
}

type indexed struct {
	idx   int
	value int64
}

func withIndex(s []int64) []indexed {
	out := make([]indexed, len(s))
	for i, v := range s {
		out[i] = indexed{idx: i, value: v}
	}
	return out
}
