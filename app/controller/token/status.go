package token

import (
	"github.com/gin-gonic/gin"

	"token-router/app/models/token"
)

// StatusHandler handles GET /debug/status.
func (h *Handler) StatusHandler(c *gin.Context) {
	st := h.Alloc.Status()
	nodes := make([]token.NodeStatus, len(st.Nodes))
	for i, n := range st.Nodes {
		nodes[i] = token.NodeStatus{ID: n.ID, Remaining: n.Remaining}
	}
	OK(c, token.StatusResponse{
		Nodes:    nodes,
		InFlight: st.InFlight,
		Budget:   st.Budget,
	})
}
