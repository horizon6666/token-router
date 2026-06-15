package token

type AllocRequest struct {
	RequestID  string `json:"request_id" binding:"required"`
	TokenCount int64  `json:"token_count" binding:"required"`
}

// AllocResponse mirrors the prompt success body exactly:
// {"node_id": <int>, "remaining_quota": <int>}.
type AllocResponse struct {
	NodeID         int   `json:"node_id"`
	RemainingQuota int64 `json:"remaining_quota"`
}

type FreeRequest struct {
	RequestID string `json:"request_id" binding:"required"`
}

// FreeResponse mirrors the prompt success body: {"node_id": <int>}.
type FreeResponse struct {
	NodeID int `json:"node_id"`
}

type NodeStatus struct {
	ID        int   `json:"id"`
	Remaining int64 `json:"remaining"`
}

type StatusResponse struct {
	Nodes    []NodeStatus `json:"nodes"`
	InFlight int          `json:"in_flight"`
	Budget   int64        `json:"budget"`
}
