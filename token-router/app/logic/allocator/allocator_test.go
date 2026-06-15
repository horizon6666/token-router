package allocator

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"

	"token-router/global/berror"
	"token-router/repository/store"
)

func newTestAllocator(n int, m int64) Allocator {
	return New(store.NewMemoryStore(n, m))
}

func TestAlloc_BasicAndOverload(t *testing.T) {
	a := newTestAllocator(1, 100)
	r, err := a.Alloc("r1", 100)
	if err != nil || r.NodeID != 0 || r.Remaining != 0 || r.Duplicate {
		t.Fatalf("alloc: %+v err=%v", r, err)
	}
	if _, err := a.Alloc("r2", 1); !errors.Is(err, berror.ErrOverloaded) {
		t.Fatalf("expected overloaded, got %v", err)
	}
	if id, err := a.Free("r1"); err != nil || id != 0 {
		t.Fatalf("free: id=%d err=%v", id, err)
	}
}

func TestAlloc_BestFit(t *testing.T) {
	a := newTestAllocator(3, 100)
	r1, _ := a.Alloc("r1", 30)
	// Best-fit: alloc 70 should land on the same node (rem=70) rather than a fresh 100.
	r2, err := a.Alloc("r2", 70)
	if err != nil {
		t.Fatal(err)
	}
	if r2.NodeID != r1.NodeID {
		t.Fatalf("best-fit failed: r1 on %d, r2 should be %d, got %d", r1.NodeID, r1.NodeID, r2.NodeID)
	}
	if r2.Remaining != 0 {
		t.Fatalf("expected remaining 0, got %d", r2.Remaining)
	}
	// Third alloc 90: only the two untouched nodes (rem=100) qualify.
	if _, err := a.Alloc("r3", 90); err != nil {
		t.Fatal(err)
	}
}

func TestAlloc_Invalid(t *testing.T) {
	a := newTestAllocator(2, 100)
	cases := []struct {
		name   string
		tokens int64
	}{
		{"zero", 0},
		{"negative", -1},
		{"over_budget", 101},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := a.Alloc("x", c.tokens); !errors.Is(err, berror.ErrInvalid) {
				t.Errorf("tokens=%d expected ErrInvalid, got %v", c.tokens, err)
			}
		})
	}
}

func TestAlloc_Idempotent(t *testing.T) {
	a := newTestAllocator(2, 100)
	r1, err := a.Alloc("r1", 40)
	if err != nil || r1.Duplicate {
		t.Fatalf("first: %+v err=%v", r1, err)
	}
	r2, err := a.Alloc("r1", 40)
	if err != nil || !r2.Duplicate {
		t.Fatalf("second: %+v err=%v", r2, err)
	}
	if r2.NodeID != r1.NodeID {
		t.Fatalf("dup should return original node: %d vs %d", r2.NodeID, r1.NodeID)
	}
	st := a.Status()
	total := int64(0)
	for _, n := range st.Nodes {
		total += n.Remaining
	}
	if total != 2*100-40 {
		t.Fatalf("expected total %d, got %d", 2*100-40, total)
	}
}

func TestFree_NotFound(t *testing.T) {
	a := newTestAllocator(1, 10)
	if _, err := a.Free("ghost"); !errors.Is(err, berror.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFree_Double(t *testing.T) {
	a := newTestAllocator(1, 10)
	if _, err := a.Alloc("r1", 5); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Free("r1"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Free("r1"); !errors.Is(err, berror.ErrNotFound) {
		t.Fatalf("second free: %v", err)
	}
	if rem := a.Status().Nodes[0].Remaining; rem != 10 {
		t.Fatalf("remaining wrong after double-free: %d", rem)
	}
}

func TestAlloc_NoOversellConcurrent(t *testing.T) {
	a := newTestAllocator(1, 100)
	const workers = 200
	var (
		wg      sync.WaitGroup
		success atomic.Int64
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("r-%d", i)
			if _, err := a.Alloc(id, 1); err == nil {
				success.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if success.Load() != 100 {
		t.Fatalf("expected 100 successes, got %d", success.Load())
	}
	if rem := a.Status().Nodes[0].Remaining; rem != 0 {
		t.Fatalf("expected 0 remaining, got %d", rem)
	}
}

// TestAlloc_SameIDConcurrent verifies idempotency under concurrent retries:
// many goroutines hammering Alloc with the same request_id must produce
// exactly one underlying deduction. All callers see the same node_id; only
// the first observes Duplicate=false.
func TestAlloc_SameIDConcurrent(t *testing.T) {
	a := newTestAllocator(2, 100)
	const workers = 200
	const tokens = int64(40)

	var (
		wg     sync.WaitGroup
		fresh  atomic.Int64
		dupCnt atomic.Int64
		nodeID atomic.Int64
	)
	nodeID.Store(-1)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := a.Alloc("same-id", tokens)
			if err != nil {
				t.Errorf("alloc failed: %v", err)
				return
			}
			if r.Duplicate {
				dupCnt.Add(1)
			} else {
				fresh.Add(1)
			}
			// All callers must agree on the node.
			if old := nodeID.Load(); old == -1 {
				nodeID.CompareAndSwap(-1, int64(r.NodeID))
			} else if old != int64(r.NodeID) {
				t.Errorf("inconsistent node_id: old=%d new=%d", old, r.NodeID)
			}
		}()
	}
	wg.Wait()

	if fresh.Load() != 1 {
		t.Fatalf("expected exactly 1 fresh alloc, got fresh=%d dup=%d", fresh.Load(), dupCnt.Load())
	}
	if dupCnt.Load() != int64(workers-1) {
		t.Fatalf("expected %d duplicates, got %d", workers-1, dupCnt.Load())
	}
	// Total deduction must equal exactly `tokens` across all nodes.
	st := a.Status()
	total := int64(0)
	for _, n := range st.Nodes {
		total += n.Remaining
	}
	wantTotal := int64(2*100) - tokens
	if total != wantTotal {
		t.Fatalf("expected total remaining %d, got %d (st=%+v)", wantTotal, total, st)
	}
	if st.InFlight != 1 {
		t.Fatalf("in-flight should be 1, got %d", st.InFlight)
	}
}

func TestAlloc_MixedConcurrent(t *testing.T) {
	a := newTestAllocator(4, 1000)
	const workers = 50
	const opsPerWorker = 200
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(int64(w)))
			held := []string{}
			for i := 0; i < opsPerWorker; i++ {
				if len(held) > 0 && r.Intn(2) == 0 {
					idx := r.Intn(len(held))
					a.Free(held[idx])
					held = append(held[:idx], held[idx+1:]...)
				} else {
					id := fmt.Sprintf("w%d-i%d", w, i)
					tokens := int64(r.Intn(50) + 1)
					if _, err := a.Alloc(id, tokens); err == nil {
						held = append(held, id)
					}
				}
			}
			for _, id := range held {
				a.Free(id)
			}
		}(w)
	}
	wg.Wait()
	st := a.Status()
	for _, n := range st.Nodes {
		if n.Remaining != 1000 {
			t.Errorf("node %d remaining %d, expected 1000", n.ID, n.Remaining)
		}
	}
	if st.InFlight != 0 {
		t.Errorf("in-flight not zero: %d", st.InFlight)
	}
}
