//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

type flightNeighbors struct {
	PreviousID *string `json:"previousId"`
	NextID     *string `json:"nextId"`
	Position   *int    `json:"position"`
	Total      int     `json:"total"`
}

func TestFlightNeighbors(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-nbr"), "SecurePass123!", "Neighbors Pilot")

	create := func(date, reg string) string {
		t.Helper()
		resp := c.POST("/flights", map[string]interface{}{
			"date": date, "aircraftReg": reg, "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		})
		requireStatus(t, resp, http.StatusCreated)
		var f map[string]interface{}
		resp.JSON(&f)
		return f["id"].(string)
	}
	oldest := create("2026-07-01", "D-ENBA")
	middle := create("2026-07-15", "D-ENBB")
	newest := create("2026-08-01", "D-ENBA")

	locate := func(t *testing.T, id, query string) flightNeighbors {
		t.Helper()
		resp := c.GET(fmt.Sprintf("/flights/%s/neighbors%s", id, query))
		requireStatus(t, resp, http.StatusOK)
		var n flightNeighbors
		resp.JSON(&n)
		return n
	}
	is := func(got *string, want string) bool {
		if want == "" {
			return got == nil
		}
		return got != nil && *got == want
	}

	t.Run("defaults to the list's order, newest first", func(t *testing.T) {
		n := locate(t, middle, "")
		if !is(n.PreviousID, newest) || !is(n.NextID, oldest) || n.Position == nil || *n.Position != 2 || n.Total != 3 {
			t.Errorf("middle = %+v, want previous newest, next oldest, 2 of 3", n)
		}
	})

	t.Run("ends have no neighbour beyond them", func(t *testing.T) {
		if n := locate(t, newest, ""); n.PreviousID != nil || !is(n.NextID, middle) {
			t.Errorf("newest = %+v, want no previous", n)
		}
		if n := locate(t, oldest, ""); n.NextID != nil || !is(n.PreviousID, middle) {
			t.Errorf("oldest = %+v, want no next", n)
		}
	})

	t.Run("follows the sort order", func(t *testing.T) {
		n := locate(t, middle, "?sortBy=date&sortOrder=asc")
		if !is(n.PreviousID, oldest) || !is(n.NextID, newest) {
			t.Errorf("middle ascending = %+v, want previous oldest, next newest", n)
		}
	})

	t.Run("follows a search, skipping flights it excludes", func(t *testing.T) {
		n := locate(t, newest, "?q="+url.QueryEscape("reg:D-ENBA"))
		if !is(n.NextID, oldest) || n.PreviousID != nil || n.Total != 2 {
			t.Errorf("newest in search = %+v, want next oldest, 2 in total", n)
		}
	})

	t.Run("a flight outside the filters has no position", func(t *testing.T) {
		n := locate(t, middle, "?aircraftReg=D-ENBA")
		if n.Position != nil || n.PreviousID != nil || n.NextID != nil || n.Total != 2 {
			t.Errorf("middle filtered out = %+v, want nulls and total 2", n)
		}
	})

	t.Run("an invalid query is a 400", func(t *testing.T) {
		requireStatus(t, c.GET(fmt.Sprintf("/flights/%s/neighbors?q=%s", middle, url.QueryEscape("night>"))), http.StatusBadRequest)
	})

	t.Run("another user's flight is a 404", func(t *testing.T) {
		other := NewE2EClient(t)
		registerAndLogin(t, other, uniqueEmail("flt-nbr-other"), "SecurePass123!", "Other Pilot")
		requireStatus(t, other.GET(fmt.Sprintf("/flights/%s/neighbors", middle)), http.StatusNotFound)
	})

	t.Run("an unauthenticated request is a 401", func(t *testing.T) {
		anon := NewE2EClient(t)
		requireStatus(t, anon.GET(fmt.Sprintf("/flights/%s/neighbors", middle)), http.StatusUnauthorized)
	})
}
