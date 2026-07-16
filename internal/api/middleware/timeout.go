package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestTimeoutMiddleware bounds every request's context to the given
// duration. Handlers pass c.Request.Context() down to database calls
// (QueryContext/ExecContext), so without this a slow or unbounded query has
// no deadline and holds its pool connection until it finishes on its own.
// This is defense in depth alongside the database's own statement_timeout:
// whichever fires first cancels the query and frees the connection.
func RequestTimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
