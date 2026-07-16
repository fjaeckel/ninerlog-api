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
// Multipart (file upload) requests are exempt — they already have their own
// size guards (router.MaxMultipartMemory plus an explicit per-file check in
// the CSV upload handler) sized for actual file uploads, which defaultLimit
// is not.
//
// overrides maps a path suffix (matched the same way as
// RateLimitByPath/RateLimitByPathPrefix) to a larger limit, for the rare
// JSON endpoint that legitimately carries a bigger payload (e.g. restoring a
// full logbook backup via POST /imports/json). The first matching suffix
// wins; unmatched paths get defaultLimit.
func MaxBodyBytesMiddleware(defaultLimit int64, overrides map[string]int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
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
