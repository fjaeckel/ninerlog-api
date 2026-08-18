package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// MaxBodyBytesMiddleware caps request body size: defaultLimit for
// non-multipart requests, multipartLimit for multipart ones. overrides maps a
// path suffix (matched the same way as RateLimitByPath) to a larger limit;
// the first matching suffix wins.
func MaxBodyBytesMiddleware(defaultLimit, multipartLimit int64, overrides map[string]int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
			// Reject an announced oversized body before reading it;
			// MaxBytesReader covers chunked and under-declared requests.
			if multipartLimit > 0 {
				if c.Request.ContentLength > multipartLimit {
					c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge,
						gin.H{"error": "Request body too large"})
					return
				}
				c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, multipartLimit)
			}
			c.Next()
			return
		}

		limit := defaultLimit
		for suffix, l := range overrides {
			if strings.HasSuffix(c.Request.URL.Path, suffix) {
				limit = l
				break
			}
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}
