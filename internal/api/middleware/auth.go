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

		c.Set("userID", claims.UserID)
		c.Next()
	}
}
