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
// request safe to retry. Spelled as in the IETF "Idempotency-Key Header Field"
// draft, which is what HTTP clients and API tooling already expect.
const HeaderIdempotencyKey = "Idempotency-Key"

// HeaderIdempotencyReplayed is set on a response served from a stored record
// rather than by executing the request. Clients use it to tell "my write was
// applied just now" from "my write had already been applied".
const HeaderIdempotencyReplayed = "Idempotency-Replayed"

// maxIdempotencyKeyLength bounds what a client may send. Long enough for a
// UUID, a ULID, or a namespaced client-side queue ID.
const maxIdempotencyKeyLength = 255

// maxFingerprintBodyBytes is the largest request body hashed into the
// fingerprint. Matches the default JSON body cap, so every ordinary write is
// covered and only the deliberately-large endpoints opt out.
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
// needs. Declared here so the middleware can be unit-tested without a database.
type IdempotencyStore interface {
	Begin(ctx context.Context, userID uuid.UUID, key string, requestHash []byte) (service.IdempotencyClaim, error)
	Finish(ctx context.Context, userID uuid.UUID, key string, claimedAt time.Time, resp service.IdempotentResponse) error
	Abandon(ctx context.Context, userID uuid.UUID, key string, claimedAt time.Time) error
	MaxResponseBytes() int
}

// IdempotencyMiddleware makes mutating requests safe to retry when the client
// opts in with an `Idempotency-Key` header.
//
// The header is the entire opt-in. A request without it takes exactly the path
// it took before this middleware existed — which is what keeps the current
// frontend, and every other existing client, unaffected — while a client that
// queues writes offline can replay its queue without risking a duplicate
// logbook entry.
//
// Shape of the guarantee:
//
//   - Scoped per authenticated user. Unauthenticated endpoints (login,
//     registration, password reset) are not covered: there is no user to key
//     the record by, and re-running them is not a logbook-integrity problem.
//   - The first request with a key executes; concurrent duplicates get 409;
//     later duplicates get the stored response verbatim, plus
//     `Idempotency-Replayed: true`.
//   - Reusing one key for a different request body gets 422 rather than the
//     first request's answer.
//   - Server errors (5xx) and panics release the key, because a 5xx says
//     nothing about whether the write landed and the client must stay free to
//     retry.
//
// It must be registered after AuthMiddleware (it reads "userID" from the
// context) and after the rate limiters, so a rejected request never consumes
// a key.
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

		// No authenticated user means no record to key by. Pass the request
		// through untouched rather than failing it: an offline client that
		// sets the header on every queued request should not have its login
		// rejected for it.
		userID, ok := c.Get("userID")
		uid, isUUID := userID.(uuid.UUID)
		if !ok || !isUUID {
			c.Next()
			return
		}

		hash, err := fingerprintRequest(c)
		if err != nil {
			// The only way to fail here is an unreadable or over-sized body,
			// which MaxBodyBytesMiddleware already rejects downstream with the
			// same status.
			IdempotencyRequestsTotal.WithLabelValues("body_error").Inc()
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Request body too large"})
			return
		}

		claim, err := store.Begin(c.Request.Context(), uid, key, hash)
		if err != nil {
			// Fail closed. The client asked for exactly-once; quietly
			// downgrading it to at-least-once is how duplicate entries are
			// created, and a 503 is something an offline queue already knows
			// how to handle.
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

		// Deferred so a panic unwinding to the recovery middleware settles the
		// claim too — a panicking request has no response worth replaying, and
		// leaving the claim behind would answer the retry with a 409 instead.
		defer settleClaim(c, store, uid, key, claim.ClaimedAt, writer)

		c.Next()
	}
}

// settleClaim either stores the captured response or releases the claim.
//
// It deliberately uses a background context: the request context may already
// be cancelled (client hung up, or the request timeout fired) at exactly the
// moment a stored response matters most — that is the "server committed, client
// never heard" case this whole mechanism exists for.
func settleClaim(c *gin.Context, store IdempotencyStore, userID uuid.UUID, key string, claimedAt time.Time, w *capturingWriter) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	status := w.Status()
	if status >= http.StatusInternalServerError || !w.wroteHeader {
		// A 5xx (or a panic, which unwinds before any status is set) says
		// nothing about whether the write landed, so the client must stay
		// free to retry.
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
		// Over the cap: mark the key consumed with no response. Finish records
		// that as "completed, not replayable", so the retry is refused rather
		// than re-executed — re-executing is the duplicate we are here to
		// prevent.
		resp.Status = 0
		resp.ContentType = ""
	}
	if err := store.Finish(ctx, userID, key, claimedAt, resp); err != nil {
		// The response has already gone out; all that is lost is the ability
		// to replay it. The stale claim expires on its own lease.
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

// validIdempotencyKey accepts bounded, printable-ASCII keys. Rejecting
// anything else keeps control characters and non-ASCII bytes out of logs and
// out of the primary key, and costs clients nothing: UUIDs and ULIDs qualify.
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

// fingerprintRequest hashes what makes this request unique: method, path and
// query, plus the body. It leaves the body readable by the handler.
//
// Bulk payloads — multipart uploads, the 50 MB logbook restore, anything sent
// chunked with no declared length — are fingerprinted by method and path
// alone. Buffering them a second time to hash them would cost more than the
// check is worth, and a bulk upload is not what a client varies between
// retries of the same queued write. Only the mismatch *diagnostic* weakens;
// the replay guarantee itself is unaffected.
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

// capturingWriter tees the response body into a buffer so it can be stored
// for replay, stopping at limit bytes rather than growing without bound.
//
// It also records whether a status was ever set. gin's own Written() only
// flips once bytes reach the socket, which would misread an empty 204 (the
// shape of every successful DELETE here) as "no response produced".
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
		// Release what was buffered: a partial body is worse than none, since
		// the caller must not replay it.
		w.truncated = true
		w.body.Reset()
		return
	}
	w.body.Write(b)
}
