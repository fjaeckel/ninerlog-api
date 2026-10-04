package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ActivityRecorder stamps an authenticated request on the account.
type ActivityRecorder func(ctx context.Context, userID uuid.UUID)

// RecordActivity calls record with the authenticated user ID of every request
// that AuthMiddleware let through. Public paths are skipped.
func RecordActivity(record ActivityRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		if v, ok := c.Get("userID"); ok {
			if userID, ok := v.(uuid.UUID); ok {
				record(c.Request.Context(), userID)
			}
		}
		c.Next()
	}
}
