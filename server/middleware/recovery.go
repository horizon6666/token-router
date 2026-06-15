package middleware

import (
	"log"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"token-router/global/berror"
)

// Recovery converts a panic into a unified 500 response.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("panic on %s %s: %v\n%s", c.Request.Method, c.Request.URL.Path, r, debug.Stack())
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": berror.ErrPanic.Msg(),
				})
			}
		}()
		c.Next()
	}
}
