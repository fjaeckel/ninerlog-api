//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

// fleetSize exceeds the old 100-row page cap so truncation would show up.
const paginationFleetSize = 150

type aircraftPage struct {
	Data []struct {
		Registration string `json:"registration"`
	} `json:"data"`
	Pagination struct {
		Page       int `json:"page"`
		PageSize   int `json:"pageSize"`
		Total      int `json:"total"`
		TotalPages int `json:"totalPages"`
	} `json:"pagination"`
}

// TestAircraftPaginationLargeFleet covers listing a fleet larger than a single
// default page: page walking, the widened 500 pageSize cap, and the counts a
// client needs to know more pages exist.
func TestAircraftPaginationLargeFleet(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("fleet"), "SecurePass123!", "Fleet User")

	for i := 0; i < paginationFleetSize; i++ {
		resp := c.POST("/aircraft", map[string]interface{}{
			"registration": fmt.Sprintf("D-E%03d", i),
			"type":         "C172",
			"make":         "Cessna",
			"model":        "172",
		})
		requireStatus(t, resp, http.StatusCreated)
	}

	t.Run("default page reports the full total", func(t *testing.T) {
		resp := c.GET("/aircraft")
		requireStatus(t, resp, http.StatusOK)
		var r aircraftPage
		resp.JSON(&r)
		if r.Pagination.Total != paginationFleetSize {
			t.Errorf("total = %d, want %d", r.Pagination.Total, paginationFleetSize)
		}
		if r.Pagination.TotalPages < 2 {
			t.Errorf("totalPages = %d, want >=2 so a client knows to keep paging", r.Pagination.TotalPages)
		}
		if len(r.Data) != r.Pagination.PageSize {
			t.Errorf("returned %d aircraft, want the page size %d", len(r.Data), r.Pagination.PageSize)
		}
	})

	t.Run("pageSize 500 returns the whole fleet in one request", func(t *testing.T) {
		resp := c.GET("/aircraft?pageSize=500")
		requireStatus(t, resp, http.StatusOK)
		var r aircraftPage
		resp.JSON(&r)
		if len(r.Data) != paginationFleetSize {
			t.Errorf("returned %d aircraft, want all %d", len(r.Data), paginationFleetSize)
		}
		if r.Pagination.TotalPages != 1 {
			t.Errorf("totalPages = %d, want 1", r.Pagination.TotalPages)
		}
	})

	t.Run("walking pages covers every aircraft exactly once", func(t *testing.T) {
		seen := map[string]bool{}
		for page := 1; ; page++ {
			resp := c.GET(fmt.Sprintf("/aircraft?page=%d&pageSize=40", page))
			requireStatus(t, resp, http.StatusOK)
			var r aircraftPage
			resp.JSON(&r)
			for _, a := range r.Data {
				if seen[a.Registration] {
					t.Errorf("registration %s returned on more than one page", a.Registration)
				}
				seen[a.Registration] = true
			}
			if page >= r.Pagination.TotalPages {
				break
			}
		}
		if len(seen) != paginationFleetSize {
			t.Errorf("paging covered %d aircraft, want %d", len(seen), paginationFleetSize)
		}
	})

	t.Run("pageSize above the maximum is clamped, not rejected", func(t *testing.T) {
		resp := c.GET("/aircraft?pageSize=5000")
		requireStatus(t, resp, http.StatusOK)
		var r aircraftPage
		resp.JSON(&r)
		if r.Pagination.PageSize > 500 {
			t.Errorf("pageSize = %d, want it clamped to at most 500", r.Pagination.PageSize)
		}
		if len(r.Data) != paginationFleetSize {
			t.Errorf("returned %d aircraft, want all %d", len(r.Data), paginationFleetSize)
		}
	})

	t.Run("page past the end is empty with the total intact", func(t *testing.T) {
		resp := c.GET("/aircraft?page=99&pageSize=100")
		requireStatus(t, resp, http.StatusOK)
		var r aircraftPage
		resp.JSON(&r)
		if len(r.Data) != 0 {
			t.Errorf("returned %d aircraft past the end, want 0", len(r.Data))
		}
		if r.Pagination.Total != paginationFleetSize {
			t.Errorf("total = %d, want %d", r.Pagination.Total, paginationFleetSize)
		}
	})
}
