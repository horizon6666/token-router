package server

import (
	"github.com/gin-gonic/gin"

	tokenctl "token-router/app/controller/token"
	"token-router/app/logic/allocator"
	"token-router/server/middleware"
)

// NewEngine wires middleware + routes onto a fresh gin.Engine.
//
// The route table is the single source of truth for HTTP entry points,
// mirroring black-card's `server/router.go` style.
func NewEngine(a allocator.Allocator) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(middleware.Recovery())

	h := tokenctl.New(a)

	r.POST("/alloc", h.AllocHandler)
	r.POST("/free", h.FreeHandler)
	r.GET("/debug/status", h.StatusHandler)

	return r
}
