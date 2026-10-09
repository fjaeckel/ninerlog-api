//go:build e2e

package e2e_test

import (
	"net/http"
	"testing"
)

// A flight session records a field without an ICAO code by its local
// identifier.
func TestFlightSession_LocalIdentAirport(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("session-ident"), "SecurePass123!", "Session")

	resp := c.POST("/flight-sessions/current/events", map[string]interface{}{
		"type":        "offblock",
		"aircraftReg": "D-MABC",
		"icao":        "de-0249",
	})
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("offblock status = %d: %s", resp.StatusCode, string(resp.Body))
	}
	var session map[string]interface{}
	resp.JSON(&session)
	if session["departureIcao"] != "DE-0249" {
		t.Errorf("departureIcao = %v, want DE-0249", session["departureIcao"])
	}

	cur := c.GET("/flight-sessions/current")
	requireStatus(t, cur, http.StatusOK)
	cur.JSON(&session)
	if session["departureIcao"] != "DE-0249" {
		t.Errorf("stored departureIcao = %v, want DE-0249", session["departureIcao"])
	}
}
