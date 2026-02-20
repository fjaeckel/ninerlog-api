package middleware

import (
	"net/http"
	"strings"

	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/gin-gonic/gin"
)

// AuthMiddleware enforces JWT authentication on all routes except explicitly
// allowed public paths. It extracts the user ID from the token and sets it
// in the Gin context as "userID".
func AuthMiddleware(jwtManager *jwt.Manager, publicPaths []string) gin.HandlerFunc {
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

		// Check if the path is public
		if public[path] {
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

		// Set user ID in context for handlers to use
		c.Set("userID", claims.UserID)
		c.Next()
	}
}
