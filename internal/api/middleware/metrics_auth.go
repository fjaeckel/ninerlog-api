package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// MetricsAuthMiddleware guards the Prometheus endpoint with a shared bearer
// token.
//
// /metrics exposes Go runtime stats, DB pool stats, per-path request counters
// and auth attempt/failure counters. That is useful reconnaissance for anyone
// who can reach the port, and the user/registration counters make it a slow
// enumeration oracle, so it should not be world-readable.
//
// The comparison is constant-time: a naive == leaks the token prefix-wise to an
// attacker who can measure response timing across many requests.
func MetricsAuthMiddleware(token string) gin.HandlerFunc {
	expected := []byte(token)
	return func(c *gin.Context) {
		presented := c.GetHeader("Authorization")
		if after, ok := strings.CutPrefix(presented, "Bearer "); ok {
			presented = after
		}
		if subtle.ConstantTimeCompare([]byte(presented), expected) != 1 {
			// Deliberately terse: no hint about whether a token was presented
			// or merely wrong.
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}
