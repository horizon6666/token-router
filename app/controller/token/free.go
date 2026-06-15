package token

import (
	"github.com/gin-gonic/gin"

	"token-router/app/models/token"
	"token-router/global/berror"
)

// FreeHandler handles POST /free.
func (h *Handler) FreeHandler(c *gin.Context) {
	var req token.FreeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, berror.ErrInvalid)
		return
	}

	nodeID, bErr := h.Alloc.Free(req.RequestID)
	if bErr != nil {
		Fail(c, bErr)
		return
	}
	OK(c, token.FreeResponse{NodeID: nodeID})
}
