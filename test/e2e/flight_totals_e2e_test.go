//go:build e2e

package e2e_test

import (
	"net/http"
	"net/url"
	"testing"
)

type flightListTotals struct {
	Data       []map[string]interface{} `json:"data"`
	Pagination struct {
		Total int `json:"total"`
	} `json:"pagination"`
	Totals struct {
		Flights     int     `json:"flights"`
		TotalTime   int     `json:"totalTime"`
		LandingsDay int     `json:"landingsDay"`
		Distance    float64 `json:"distance"`
	} `json:"totals"`
	MonthTotals []struct {
		Month  string `json:"month"`
		Totals struct {
			Flights   int `json:"flights"`
			TotalTime int `json:"totalTime"`
		} `json:"totals"`
	} `json:"monthTotals"`
}

func TestFlightListTotals(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-totals"), "SecurePass123!", "Totals Pilot")

	create := func(date, reg, off, on string) {
		t.Helper()
		requireStatus(t, c.POST("/flights", map[string]interface{}{
			"date": date, "aircraftReg": reg, "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": off, "onBlockTime": on, "landings": 1,
		}), http.StatusCreated)
	}
	create("2026-07-10", "D-ETOT", "08:00", "09:00")
	create("2026-08-02", "D-ETOT", "08:00", "09:30")
	create("2026-08-15", "D-EOTH", "10:00", "10:45")

	t.Run("totals cover every matching flight, not just the page", func(t *testing.T) {
		resp := c.GET("/flights?pageSize=1&sortBy=date&sortOrder=desc")
		requireStatus(t, resp, http.StatusOK)
		var page flightListTotals
		resp.JSON(&page)

		if len(page.Data) != 1 {
			t.Fatalf("page has %d flights, want 1", len(page.Data))
		}
		if page.Totals.Flights != 3 || page.Totals.TotalTime != 195 || page.Totals.LandingsDay != 3 {
			t.Errorf("totals = %+v, want 3 flights, 195 min, 3 day landings", page.Totals)
		}
		if page.Totals.Distance <= 0 {
			t.Errorf("distance total = %v, want > 0", page.Totals.Distance)
		}
	})

	t.Run("month totals cover the whole month of the flights on the page", func(t *testing.T) {
		resp := c.GET("/flights?pageSize=1&sortBy=date&sortOrder=desc")
		requireStatus(t, resp, http.StatusOK)
		var page flightListTotals
		resp.JSON(&page)

		if len(page.MonthTotals) != 1 {
			t.Fatalf("monthTotals = %+v, want only August", page.MonthTotals)
		}
		aug := page.MonthTotals[0]
		if aug.Month != "2026-08" || aug.Totals.Flights != 2 || aug.Totals.TotalTime != 135 {
			t.Errorf("August = %+v, want 2026-08 with 2 flights / 135 min", aug)
		}
	})

	t.Run("totals follow the filters", func(t *testing.T) {
		resp := c.GET("/flights?aircraftReg=D-ETOT")
		requireStatus(t, resp, http.StatusOK)
		var page flightListTotals
		resp.JSON(&page)

		if page.Totals.Flights != 2 || page.Totals.TotalTime != 150 {
			t.Errorf("filtered totals = %+v, want 2 flights / 150 min", page.Totals)
		}
		if len(page.MonthTotals) != 2 {
			t.Errorf("filtered monthTotals = %+v, want July and August", page.MonthTotals)
		}
	})

	t.Run("totals follow a free-text search", func(t *testing.T) {
		resp := c.GET("/flights?search=D-EOTH")
		requireStatus(t, resp, http.StatusOK)
		var page flightListTotals
		resp.JSON(&page)

		if page.Totals.Flights != 1 || page.Totals.TotalTime != 45 {
			t.Errorf("search totals = %+v, want 1 flight / 45 min", page.Totals)
		}
		if len(page.MonthTotals) != 1 || page.MonthTotals[0].Totals.Flights != 1 {
			t.Errorf("search monthTotals = %+v, want August with only the matching flight", page.MonthTotals)
		}
	})

	t.Run("totals follow an advanced query", func(t *testing.T) {
		resp := c.GET("/flights?q=" + url.QueryEscape("reg:D-ETOT AND total>=90"))
		requireStatus(t, resp, http.StatusOK)
		var page flightListTotals
		resp.JSON(&page)

		if page.Totals.Flights != 1 || page.Totals.TotalTime != 90 {
			t.Errorf("query totals = %+v, want 1 flight / 90 min", page.Totals)
		}
		if page.Pagination.Total != page.Totals.Flights {
			t.Errorf("pagination.total = %d, totals.flights = %d; want equal", page.Pagination.Total, page.Totals.Flights)
		}
	})

	t.Run("an empty logbook has zero totals and no months", func(t *testing.T) {
		empty := NewE2EClient(t)
		registerAndLogin(t, empty, uniqueEmail("flt-totals-empty"), "SecurePass123!", "Empty Pilot")
		resp := empty.GET("/flights")
		requireStatus(t, resp, http.StatusOK)
		var page flightListTotals
		resp.JSON(&page)

		if page.Totals.Flights != 0 || page.Totals.TotalTime != 0 {
			t.Errorf("totals = %+v, want zeros", page.Totals)
		}
		if page.MonthTotals == nil || len(page.MonthTotals) != 0 {
			t.Errorf("monthTotals = %v, want an empty array", page.MonthTotals)
		}
	})
}
