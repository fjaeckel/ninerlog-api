package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// MaxBodyBytesMiddleware caps the request body size for non-multipart
// requests. ShouldBindJSON (and json.NewDecoder) read the body in full
// before any application-level validation runs, so without a cap a large
// JSON payload is buffered entirely in memory regardless of what the
// handler goes on to check.
//
// Multipart (file upload) requests get their own, larger cap rather than being
// exempted. router.MaxMultipartMemory is NOT a total-size limit — it only sets
// how much of a part is buffered in RAM before Go spills the remainder to temp
// files on disk, with no ceiling on the remainder. The CSV upload handler's own
// "max 10 MB" check runs only AFTER c.Request.FormFile has consumed the whole
// body, so without a cap here the server pays the full cost of receiving and
// storing whatever was sent before rejecting it (measured: a 500 MB upload
// consumed 486 MB of disk and still returned "File too large").
//
// overrides maps a path suffix (matched the same way as
// RateLimitByPath/RateLimitByPathPrefix) to a larger limit, for the rare
// JSON endpoint that legitimately carries a bigger payload (e.g. restoring a
// full logbook backup via POST /imports/json). The first matching suffix
// wins; unmatched paths get defaultLimit.
func MaxBodyBytesMiddleware(defaultLimit, multipartLimit int64, overrides map[string]int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
			// Reject early when the client announces an oversized body, so we
			// do not read it at all; MaxBytesReader then enforces the cap for
			// chunked or under-declared requests.
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
