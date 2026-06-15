package token

import (
	"github.com/gin-gonic/gin"

	"token-router/app/logic/allocator"
	"token-router/app/models/token"
	"token-router/global/berror"
)

// Handler bundles the dependencies the token endpoints need.
type Handler struct {
	Alloc allocator.Allocator
}

func New(a allocator.Allocator) *Handler {
	return &Handler{Alloc: a}
}

// AllocHandler handles POST /alloc.
//
// Response shape follows the problem statement exactly:
//
//	200 {"node_id": <int>, "remaining_quota": <int>}
//	429 {"error": "overloaded"}
//	400 {"error": "invalid_request"}
//
// When the request_id is a duplicate (idempotent retry), we still answer 200
// with the original placement and signal it via the X-Allocation-Duplicate
// header. The JSON body stays clean for strict contract checkers.
func (h *Handler) AllocHandler(c *gin.Context) {
	var req token.AllocRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, berror.ErrInvalid)
		return
	}

	res, bErr := h.Alloc.Alloc(req.RequestID, req.TokenCount)
	if bErr != nil {
		Fail(c, bErr)
		return
	}

	if res.Duplicate {
		c.Header(HeaderDuplicate, "true")
	}
	OK(c, token.AllocResponse{
		NodeID:         res.NodeID,
		RemainingQuota: res.Remaining,
	})
}
