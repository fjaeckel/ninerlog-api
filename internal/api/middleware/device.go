package middleware

import (
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
)

// DeviceContext attaches the calling client's User-Agent and address to the
// request context, where the auth service reads them when creating or
// renewing a session.
func DeviceContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := service.ContextWithDevice(c.Request.Context(), service.DeviceInfo{
			UserAgent: c.Request.UserAgent(),
			IPAddress: c.ClientIP(),
		})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
