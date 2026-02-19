package middleware

import (
	"net/http"
	"strings"

	"github.com/fjaeckel/pilotlog-api/pkg/jwt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminMiddleware creates a middleware that restricts access to admin users only.
// It requires the auth middleware to have already set "userID" in the context.
// The admin user is determined by matching the user's email against the ADMIN_EMAIL env var.
func AdminMiddleware(jwtManager *jwt.Manager, adminEmail string, getUserEmail func(userID uuid.UUID) (string, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if adminEmail == "" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin access not configured"})
			c.Abort()
			return
		}

		userID, exists := c.Get("userID")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}

		id, ok := userID.(uuid.UUID)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}

		email, err := getUserEmail(id)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
			c.Abort()
			return
		}

		if !strings.EqualFold(email, adminEmail) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
			c.Abort()
			return
		}

		c.Set("isAdmin", true)
		c.Next()
	}
}
