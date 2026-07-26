package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/gin-gonic/gin"
)

// AuthMiddleware enforces JWT authentication on all routes except explicitly
// allowed public paths. It extracts the user ID from the token and sets it
// in the Gin context as "userID".
// UserSessionState reports whether a user still exists, is enabled, and the
// instant before which their access tokens are no longer valid. Implemented by
// the auth service; nil disables the check (used by tests).
type UserSessionState func(userID uuid.UUID) (disabled bool, tokensValidAfter *time.Time, err error)

// AuthMiddleware enforces JWT authentication. See AuthMiddlewareWithState for
// the session-revocation variant.
func AuthMiddleware(jwtManager *jwt.Manager, publicPaths []string) gin.HandlerFunc {
	return AuthMiddlewareWithState(jwtManager, publicPaths, nil)
}

// AuthMiddlewareWithState additionally rejects tokens belonging to a disabled
// or deleted user, and tokens issued before the user's session epoch.
//
// Access tokens are stateless 15-minute JWTs, so without this a token stayed
// usable for its full lifetime no matter what happened to the account:
// disabling a user or changing a password only deleted REFRESH tokens, which
// merely stops the session being extended. Both were confirmed against a
// running instance -- a disabled user's token still read and created flights,
// and an old token still worked after a password change.
func AuthMiddlewareWithState(jwtManager *jwt.Manager, publicPaths []string, state UserSessionState) gin.HandlerFunc {
	// Build a set for O(1) lookup
	public := make(map[string]bool, len(publicPaths))
	for _, p := range publicPaths {
		public[p] = true
	}

	return func(c *gin.Context) {
		// Strip the router group prefix to get the relative path
		// e.g., /api/v1/auth/login -> /auth/login
		path := c.Request.URL.Path
		// Remove /api/v1 prefix if present
		if idx := strings.Index(path, "/api/v1"); idx >= 0 {
			path = path[idx+len("/api/v1"):]
		}

		// Check if the path is public. publicPaths may contain either literal
		// paths (e.g. "/auth/login") or gin route patterns for parameterized
		// routes (e.g. "/sign/:token"); c.FullPath() returns the resolved
		// pattern for the matched route, so pattern-shaped entries need to be
		// checked against it rather than against the literal request path.
		fullPath := c.FullPath()
		if idx := strings.Index(fullPath, "/api/v1"); idx >= 0 {
			fullPath = fullPath[idx+len("/api/v1"):]
		}
		if public[path] || (fullPath != "" && public[fullPath]) {
			c.Next()
			return
		}

		// Extract and validate Bearer token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || len(authHeader) < 8 || !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			c.Abort()
			return
		}

		tokenString := authHeader[7:]
		claims, err := jwtManager.ValidateAccessToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		if state != nil {
			disabled, validAfter, err := state(claims.UserID)
			if err != nil {
				// Unknown or deleted user.
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
				c.Abort()
				return
			}
			if disabled {
				c.JSON(http.StatusForbidden, gin.H{"error": "Account disabled"})
				c.Abort()
				return
			}
			// Reject tokens minted before the last invalidation event. IssuedAt
			// has second resolution, so a token issued in the same second as the
			// event is also rejected (Before would let it through).
			if validAfter != nil && claims.IssuedAt != nil &&
				!claims.IssuedAt.Time.After(*validAfter) {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Session expired, please sign in again"})
				c.Abort()
				return
			}
		}

		// Set user ID in context for handlers to use
		c.Set("userID", claims.UserID)
		c.Next()
	}
}
