package token

import (
	"github.com/gin-gonic/gin"

	"token-router/global/berror"
)

// HeaderDuplicate marks an alloc that hit the idempotent path so observability
// can spot client retries without polluting the JSON contract specified by the
// problem statement.
const HeaderDuplicate = "X-Allocation-Duplicate"

// errorBody matches the problem statement: {"error": "<reason>"}.
type errorBody struct {
	Error string `json:"error"`
}

// OK writes data as the response body verbatim. The problem statement
// requires flat JSON like {"node_id": 0, "remaining_quota": 220}.
func OK(c *gin.Context, data any) {
	c.JSON(200, data)
}

func Fail(c *gin.Context, err berror.Error) {
	c.JSON(err.HTTPStatus(), errorBody{Error: err.Msg()})
}
