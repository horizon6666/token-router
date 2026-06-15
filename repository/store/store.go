package store

// Allocation represents a held reservation owned by a request.
type Allocation struct {
	NodeID int
	Tokens int64
}

// Store abstracts the per-node remaining quotas and the request_id ledger.
//
// Implementations must guarantee that ReserveOn does not produce a negative
// remaining and that LoadOrStoreAllocation is atomic w.r.t. concurrent callers
// using the same key.
type Store interface {
	// NodeCount returns the number of nodes managed by the store.
	NodeCount() int

	// Budget returns the per-node maximum quota.
	Budget() int64

	// SnapshotRemaining returns a snapshot of every node's current remaining
	// quota. Index i corresponds to node id i.
	SnapshotRemaining() []int64

	// ReserveOn attempts to atomically deduct `tokens` from node `id` if and
	// only if its current remaining is exactly `expected`. Returns the new
	// remaining and true on success.
	ReserveOn(id int, expected, tokens int64) (newRemaining int64, ok bool)

	// ReleaseTo adds `tokens` back to node `id`.
	ReleaseTo(id int, tokens int64)

	// RemainingOf returns the current remaining quota of node `id`.
	RemainingOf(id int) int64

	// LoadAllocation returns the allocation for `reqID` and whether it exists.
	LoadAllocation(reqID string) (Allocation, bool)

	// LoadOrStoreAllocation atomically inserts `a` if `reqID` is unknown.
	// Returns the existing allocation and loaded=true on collision; otherwise
	// loaded=false and the inserted allocation.
	LoadOrStoreAllocation(reqID string, a Allocation) (existing Allocation, loaded bool)

	// LoadAndDeleteAllocation removes the allocation for `reqID` and returns it.
	LoadAndDeleteAllocation(reqID string) (Allocation, bool)

	// InFlight returns the number of currently-held allocations.
	InFlight() int
}
