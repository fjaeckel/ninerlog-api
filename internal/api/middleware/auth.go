package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// SessionState reports whether the account behind an access token is disabled
// and whether the token's session still holds a live refresh token. A deleted
// account reports live=false; a non-nil error means the state could not be
// read. Nil disables the check.
type SessionState func(ctx context.Context, userID, sessionID uuid.UUID) (disabled bool, live bool, err error)

// AuthMiddleware enforces JWT authentication without checking session state.
// See AuthMiddlewareWithState.
func AuthMiddleware(jwtManager *jwt.Manager, publicPaths []string) gin.HandlerFunc {
	return AuthMiddlewareWithState(jwtManager, publicPaths, nil)
}

// AuthMiddlewareWithState enforces JWT authentication on all routes except
// explicitly allowed public paths. It extracts the user ID from the token and
// sets it in the Gin context as "userID".
//
// When state is non-nil it additionally rejects tokens whose session has been
// revoked and tokens belonging to a disabled or deleted account, with a 401,
// and answers 503 when the state cannot be read. Tokens carrying no session ID
// are checked for the disabled flag only.
func AuthMiddlewareWithState(jwtManager *jwt.Manager, publicPaths []string, state SessionState) gin.HandlerFunc {
	public := make(map[string]bool, len(publicPaths))
	for _, p := range publicPaths {
		public[p] = true
	}

	return func(c *gin.Context) {
		// Strip the router group prefix: /api/v1/auth/login -> /auth/login.
		path := c.Request.URL.Path
		if idx := strings.Index(path, "/api/v1"); idx >= 0 {
			path = path[idx+len("/api/v1"):]
		}

		// publicPaths may contain literal paths ("/auth/login") or gin route
		// patterns ("/sign/:token"); patterns are matched via c.FullPath().
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

		if state != nil && !sessionUsable(c, state, claims.UserID, claims.SessionID) {
			return
		}

		c.Set("userID", claims.UserID)
		if claims.SessionID != uuid.Nil {
			c.Set("sessionID", claims.SessionID)
		}
		c.Next()
	}
}

// sessionUsable reports whether the request may proceed, writing the response
// and aborting when it may not.
func sessionUsable(c *gin.Context, state SessionState, userID, sessionID uuid.UUID) bool {
	disabled, live, err := state(c.Request.Context(), userID, sessionID)
	switch {
	case err != nil:
		AccessTokensRejectedTotal.WithLabelValues("lookup_failed").Inc()
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Session state unavailable"})
		c.Abort()
		return false
	case disabled:
		AccessTokensRejectedTotal.WithLabelValues("account_disabled").Inc()
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Session ended, please sign in again"})
		c.Abort()
		return false
	case !live && sessionID != uuid.Nil:
		AccessTokensRejectedTotal.WithLabelValues("session_revoked").Inc()
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Session ended, please sign in again"})
		c.Abort()
		return false
	}
	return true
}
