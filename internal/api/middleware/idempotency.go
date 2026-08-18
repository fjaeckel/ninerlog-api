package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

// HeaderIdempotencyKey is the request header a client sets to make a mutating
// request safe to retry (spelled as in the IETF "Idempotency-Key Header
// Field" draft).
const HeaderIdempotencyKey = "Idempotency-Key"

// HeaderIdempotencyReplayed is set on a response served from a stored record
// rather than by executing the request.
const HeaderIdempotencyReplayed = "Idempotency-Replayed"

// maxIdempotencyKeyLength bounds the client-supplied key.
const maxIdempotencyKeyLength = 255

// maxFingerprintBodyBytes is the largest request body hashed into the
// fingerprint.
const maxFingerprintBodyBytes = 1 << 20

// IdempotencyRequestsTotal counts requests that carried an Idempotency-Key,
// by outcome: executed (claimed and run), replayed, in_progress, mismatch,
// not_replayable, invalid_key, unavailable.
var IdempotencyRequestsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "idempotency_requests_total",
		Help: "Requests carrying an Idempotency-Key header, by outcome.",
	},
	[]string{"outcome"},
)

func init() {
	prometheus.MustRegister(IdempotencyRequestsTotal)
}

// IdempotencyStore is the slice of the idempotency service this middleware
// needs.
type IdempotencyStore interface {
	Begin(ctx context.Context, userID uuid.UUID, key string, requestHash []byte) (service.IdempotencyClaim, error)
	Finish(ctx context.Context, userID uuid.UUID, key string, claimedAt time.Time, resp service.IdempotentResponse) error
	Abandon(ctx context.Context, userID uuid.UUID, key string, claimedAt time.Time) error
	MaxResponseBytes() int
}

// IdempotencyMiddleware makes mutating requests safe to retry when the client
// opts in with an `Idempotency-Key` header; a request without the header
// passes through untouched.
//
// Shape of the guarantee:
//
//   - Scoped per authenticated user; unauthenticated endpoints are not
//     covered.
//   - The first request with a key executes; concurrent duplicates get 409;
//     later duplicates get the stored response verbatim, plus
//     `Idempotency-Replayed: true`.
//   - Reusing one key for a different request body gets 422.
//   - Server errors (5xx) and panics release the key.
//
// It must be registered after AuthMiddleware (it reads "userID" from the
// context) and after the rate limiters.
func IdempotencyMiddleware(store IdempotencyStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isMutatingMethod(c.Request.Method) {
			c.Next()
			return
		}
		key := strings.TrimSpace(c.GetHeader(HeaderIdempotencyKey))
		if key == "" {
			c.Next()
			return
		}
		if !validIdempotencyKey(key) {
			IdempotencyRequestsTotal.WithLabelValues("invalid_key").Inc()
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "Idempotency-Key must be 1-255 printable ASCII characters",
			})
			return
		}

		// Without an authenticated user the request passes through untouched.
		userID, ok := c.Get("userID")
		uid, isUUID := userID.(uuid.UUID)
		if !ok || !isUUID {
			c.Next()
			return
		}

		hash, err := fingerprintRequest(c)
		if err != nil {
			IdempotencyRequestsTotal.WithLabelValues("body_error").Inc()
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Request body too large"})
			return
		}

		claim, err := store.Begin(c.Request.Context(), uid, key, hash)
		if err != nil {
			// Fail closed with 503.
			slog.Warn("Idempotency claim failed", "error", err, "path", c.Request.URL.Path)
			IdempotencyRequestsTotal.WithLabelValues("unavailable").Inc()
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": "Idempotent request handling is temporarily unavailable",
			})
			return
		}

		switch claim.Outcome {
		case service.IdempotencyReplay:
			IdempotencyRequestsTotal.WithLabelValues("replayed").Inc()
			replayResponse(c, claim.Response)
			return
		case service.IdempotencyInProgress:
			IdempotencyRequestsTotal.WithLabelValues("in_progress").Inc()
			c.Header("Retry-After", "1")
			c.AbortWithStatusJSON(service.IdempotencyStatus(claim.Outcome), gin.H{
				"error": "A request with this Idempotency-Key is still in progress",
			})
			return
		case service.IdempotencyMismatch:
			IdempotencyRequestsTotal.WithLabelValues("mismatch").Inc()
			c.AbortWithStatusJSON(service.IdempotencyStatus(claim.Outcome), gin.H{
				"error": "This Idempotency-Key was already used for a different request",
			})
			return
		case service.IdempotencyNotReplayable:
			IdempotencyRequestsTotal.WithLabelValues("not_replayable").Inc()
			c.AbortWithStatusJSON(service.IdempotencyStatus(claim.Outcome), gin.H{
				"error": "The original request completed but its response is no longer available for replay",
			})
			return
		}

		writer := &capturingWriter{ResponseWriter: c.Writer, limit: store.MaxResponseBytes()}
		c.Writer = writer

		// Deferred: the claim is settled even when a panic unwinds to the
		// recovery middleware.
		defer settleClaim(c, store, uid, key, claim.ClaimedAt, writer)

		c.Next()
	}
}

// settleClaim either stores the captured response or releases the claim, on a
// fresh background context.
func settleClaim(c *gin.Context, store IdempotencyStore, userID uuid.UUID, key string, claimedAt time.Time, w *capturingWriter) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	status := w.Status()
	if status >= http.StatusInternalServerError || !w.wroteHeader {
		// A 5xx, or a panic that unwound before any status was set, releases
		// the claim.
		if err := store.Abandon(ctx, userID, key, claimedAt); err != nil {
			slog.Warn("Releasing idempotency claim failed", "error", err, "path", c.Request.URL.Path)
		}
		IdempotencyRequestsTotal.WithLabelValues("released").Inc()
		return
	}

	resp := service.IdempotentResponse{
		Status:      status,
		ContentType: w.Header().Get("Content-Type"),
	}
	if !w.truncated {
		resp.Body = w.body.Bytes()
	} else {
		// Over the cap: the key is marked consumed with no replayable response.
		resp.Status = 0
		resp.ContentType = ""
	}
	if err := store.Finish(ctx, userID, key, claimedAt, resp); err != nil {
		// The stale claim expires on its own lease.
		slog.Warn("Storing idempotent response failed", "error", err, "path", c.Request.URL.Path)
	}
	IdempotencyRequestsTotal.WithLabelValues("executed").Inc()
}

func replayResponse(c *gin.Context, resp *service.IdempotentResponse) {
	c.Header(HeaderIdempotencyReplayed, "true")
	if len(resp.Body) == 0 {
		c.Status(resp.Status)
		c.Abort()
		return
	}
	contentType := resp.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Data(resp.Status, contentType, resp.Body)
	c.Abort()
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// validIdempotencyKey accepts bounded, printable-ASCII keys.
func validIdempotencyKey(key string) bool {
	if len(key) == 0 || len(key) > maxIdempotencyKeyLength {
		return false
	}
	for i := 0; i < len(key); i++ {
		if key[i] < 0x21 || key[i] > 0x7E {
			return false
		}
	}
	return true
}

// fingerprintRequest hashes method, path and query, plus the body, leaving
// the body readable by the handler. Multipart, chunked, and over-cap bodies
// are fingerprinted by method and path alone.
func fingerprintRequest(c *gin.Context) ([]byte, error) {
	h := sha256.New()
	h.Write([]byte(c.Request.Method))
	h.Write([]byte{0})
	h.Write([]byte(c.Request.URL.RequestURI()))
	h.Write([]byte{0})

	if c.Request.Body == nil ||
		strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") ||
		c.Request.ContentLength < 0 ||
		c.Request.ContentLength > maxFingerprintBodyBytes {
		return h.Sum(nil), nil
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, err
	}
	_ = c.Request.Body.Close()
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	h.Write(body)
	return h.Sum(nil), nil
}

// capturingWriter tees the response body into a buffer, stopping at limit
// bytes, and records whether a status was ever set.
type capturingWriter struct {
	gin.ResponseWriter
	body        bytes.Buffer
	limit       int
	truncated   bool
	wroteHeader bool
}

func (w *capturingWriter) WriteHeader(code int) {
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *capturingWriter) Write(b []byte) (int, error) {
	w.wroteHeader = true
	w.capture(b)
	return w.ResponseWriter.Write(b)
}

func (w *capturingWriter) WriteString(s string) (int, error) {
	w.wroteHeader = true
	w.capture([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

func (w *capturingWriter) capture(b []byte) {
	if w.truncated {
		return
	}
	if w.body.Len()+len(b) > w.limit {
		// Drop the partial buffer.
		w.truncated = true
		w.body.Reset()
		return
	}
	w.body.Write(b)
}
