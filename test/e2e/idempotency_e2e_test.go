//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// countFlights returns how many flights the authenticated user has.
func countFlights(t *testing.T, c *E2EClient) int {
	t.Helper()
	r := c.GET("/flights?limit=100")
	requireStatus(t, r, http.StatusOK)
	var page map[string]interface{}
	if err := r.JSON(&page); err != nil {
		t.Fatalf("decode flights page: %v", err)
	}
	data, _ := page["data"].([]interface{})
	return len(data)
}

func idempotencyKey(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func flightPayload(remarks string) map[string]interface{} {
	return map[string]interface{}{
		"date":          today(),
		"aircraftReg":   "D-EIDM",
		"aircraftType":  "C172",
		"departureIcao": "EDNY",
		"arrivalIcao":   "EDDS",
		"offBlockTime":  "08:00",
		"onBlockTime":   "09:30",
		"landings":      1,
		"remarks":       remarks,
	}
}

// TestIdempotentFlightCreate covers a replayed keyed POST returning the first
// response without creating a second flight.
func TestIdempotentFlightCreate(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("idem-create"), "SecurePass123!", "Idem Create")
	key := idempotencyKey("create")
	headers := map[string]string{"Idempotency-Key": key}

	first := c.DoWithHeaders("POST", "/flights", flightPayload("queued offline"), headers)
	requireStatus(t, first, http.StatusCreated)
	var created map[string]interface{}
	if err := first.JSON(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if first.Headers.Get("Idempotency-Replayed") != "" {
		t.Error("the first request must not be flagged as a replay")
	}

	second := c.DoWithHeaders("POST", "/flights", flightPayload("queued offline"), headers)
	assertStatus(t, second, http.StatusCreated)
	if second.Headers.Get("Idempotency-Replayed") != "true" {
		t.Errorf("retry should carry Idempotency-Replayed: true, headers %v", second.Headers)
	}
	if string(second.Body) != string(first.Body) {
		t.Errorf("retry body differs\nfirst:  %s\nsecond: %s", first.Body, second.Body)
	}

	if n := countFlights(t, c); n != 1 {
		t.Errorf("retry created a duplicate: %d flights logged, want 1", n)
	}
}

// Without the header the API behaviour is unchanged.
func TestIdempotencyOptInOnly(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("idem-optout"), "SecurePass123!", "Idem OptOut")

	requireStatus(t, c.POST("/flights", flightPayload("no key 1")), http.StatusCreated)
	requireStatus(t, c.POST("/flights", flightPayload("no key 1")), http.StatusCreated)

	if n := countFlights(t, c); n != 2 {
		t.Errorf("keyless requests should each create a flight: got %d, want 2", n)
	}
}

// Reusing one key for a different payload is rejected with 422.
func TestIdempotencyKeyReuseWithDifferentBody(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("idem-mismatch"), "SecurePass123!", "Idem Mismatch")
	headers := map[string]string{"Idempotency-Key": idempotencyKey("mismatch")}

	requireStatus(t, c.DoWithHeaders("POST", "/flights", flightPayload("original"), headers), http.StatusCreated)

	changed := flightPayload("different")
	changed["arrivalIcao"] = "EDDM"
	resp := c.DoWithHeaders("POST", "/flights", changed, headers)
	assertStatus(t, resp, http.StatusUnprocessableEntity)

	if n := countFlights(t, c); n != 1 {
		t.Errorf("the rejected request must not have been executed: %d flights, want 1", n)
	}
}

// Keys are scoped per user.
func TestIdempotencyKeysAreScopedPerUser(t *testing.T) {
	key := idempotencyKey("shared")
	headers := map[string]string{"Idempotency-Key": key}

	first := NewE2EClient(t)
	registerAndLogin(t, first, uniqueEmail("idem-user-a"), "SecurePass123!", "Idem A")
	requireStatus(t, first.DoWithHeaders("POST", "/flights", flightPayload("pilot a"), headers), http.StatusCreated)

	second := NewE2EClient(t)
	registerAndLogin(t, second, uniqueEmail("idem-user-b"), "SecurePass123!", "Idem B")
	resp := second.DoWithHeaders("POST", "/flights", flightPayload("pilot b"), headers)
	requireStatus(t, resp, http.StatusCreated)
	if resp.Headers.Get("Idempotency-Replayed") == "true" {
		t.Error("another user's key was replayed")
	}
	if n := countFlights(t, second); n != 1 {
		t.Errorf("second user should have their own flight: got %d", n)
	}
}

// A repeated keyed DELETE replays the original 204 instead of 404.
func TestIdempotentFlightDelete(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("idem-delete"), "SecurePass123!", "Idem Delete")

	create := c.POST("/flights", flightPayload("to be deleted"))
	requireStatus(t, create, http.StatusCreated)
	var flight map[string]interface{}
	if err := create.JSON(&flight); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	id, _ := flight["id"].(string)
	if id == "" {
		t.Fatalf("no flight id in %s", create.Body)
	}

	headers := map[string]string{"Idempotency-Key": idempotencyKey("delete")}
	first := c.DoWithHeaders("DELETE", "/flights/"+id, nil, headers)
	requireStatus(t, first, http.StatusNoContent)

	second := c.DoWithHeaders("DELETE", "/flights/"+id, nil, headers)
	assertStatus(t, second, http.StatusNoContent)
	if second.Headers.Get("Idempotency-Replayed") != "true" {
		t.Error("repeated DELETE should be flagged as a replay")
	}

	// Without a key the same repeat is a 404.
	assertStatus(t, c.DELETE("/flights/"+id), http.StatusNotFound)
}

// Keyed updates replay the same way as creates.
func TestIdempotentFlightUpdate(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("idem-update"), "SecurePass123!", "Idem Update")

	create := c.POST("/flights", flightPayload("before"))
	requireStatus(t, create, http.StatusCreated)
	var flight map[string]interface{}
	if err := create.JSON(&flight); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	id, _ := flight["id"].(string)

	headers := map[string]string{"Idempotency-Key": idempotencyKey("update")}
	payload := flightPayload("after")
	first := c.DoWithHeaders("PUT", "/flights/"+id, payload, headers)
	requireStatus(t, first, http.StatusOK)

	second := c.DoWithHeaders("PUT", "/flights/"+id, payload, headers)
	assertStatus(t, second, http.StatusOK)
	if second.Headers.Get("Idempotency-Replayed") != "true" {
		t.Error("repeated PUT should be flagged as a replay")
	}
	if string(second.Body) != string(first.Body) {
		t.Errorf("replayed PUT body differs\nfirst:  %s\nsecond: %s", first.Body, second.Body)
	}
}

// A rejected request's response replays too, and no flight is created on
// retry.
func TestIdempotentValidationFailureReplays(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("idem-invalid"), "SecurePass123!", "Idem Invalid")
	headers := map[string]string{"Idempotency-Key": idempotencyKey("invalid")}

	bad := map[string]interface{}{"aircraftReg": "D-EIDM"} // no date, no route
	first := c.DoWithHeaders("POST", "/flights", bad, headers)
	if first.StatusCode < 400 || first.StatusCode >= 500 {
		t.Fatalf("expected a 4xx for an invalid flight, got %d: %s", first.StatusCode, first.Body)
	}

	second := c.DoWithHeaders("POST", "/flights", bad, headers)
	assertStatus(t, second, first.StatusCode)
	if second.Headers.Get("Idempotency-Replayed") != "true" {
		t.Error("repeated invalid request should replay the original rejection")
	}
	if n := countFlights(t, c); n != 0 {
		t.Errorf("no flight should exist, got %d", n)
	}
}

func TestIdempotencyKeyValidation(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("idem-badkey"), "SecurePass123!", "Idem BadKey")

	cases := []struct {
		name string
		key  string
	}{
		{"too long", repeatChar('k', 256)},
		{"contains a space", "key with space"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := c.DoWithHeaders("POST", "/flights", flightPayload("bad key"),
				map[string]string{"Idempotency-Key": tc.key})
			assertStatus(t, resp, http.StatusBadRequest)
		})
	}

	if n := countFlights(t, c); n != 0 {
		t.Errorf("rejected keys must not create flights, got %d", n)
	}
}

// The header is inert on GET.
func TestIdempotencyKeyIgnoredOnReads(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("idem-read"), "SecurePass123!", "Idem Read")
	headers := map[string]string{"Idempotency-Key": idempotencyKey("read")}

	for i := 0; i < 2; i++ {
		resp := c.DoWithHeaders("GET", "/flights", nil, headers)
		assertStatus(t, resp, http.StatusOK)
		if resp.Headers.Get("Idempotency-Replayed") != "" {
			t.Error("GET responses must never be served from the replay store")
		}
	}
}

func repeatChar(ch byte, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = ch
	}
	return string(b)
}
