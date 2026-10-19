//go:build e2e

package e2e_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"
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
		resp.MustJSON(t, &page)

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
		resp.MustJSON(t, &page)

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
		resp.MustJSON(t, &page)

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
		resp.MustJSON(t, &page)

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
		resp.MustJSON(t, &page)

		if page.Totals.Flights != 1 || page.Totals.TotalTime != 90 {
			t.Errorf("query totals = %+v, want 1 flight / 90 min", page.Totals)
		}
		if page.Pagination.Total != page.Totals.Flights {
			t.Errorf("pagination.total = %d, totals.flights = %d; want equal", page.Pagination.Total, page.Totals.Flights)
		}
	})

	t.Run("month totals are skipped when the list is not in date order", func(t *testing.T) {
		resp := c.GET("/flights?sortBy=totalTime")
		requireStatus(t, resp, http.StatusOK)
		var page flightListTotals
		resp.MustJSON(t, &page)

		if page.MonthTotals == nil || len(page.MonthTotals) != 0 {
			t.Errorf("monthTotals = %v, want an empty array", page.MonthTotals)
		}
		if page.Totals.Flights != 3 {
			t.Errorf("totals.flights = %d, want 3", page.Totals.Flights)
		}
	})

	t.Run("a delta request skips month totals and sums only changed flights", func(t *testing.T) {
		since := url.QueryEscape(time.Now().Add(-time.Hour).UTC().Format(time.RFC3339))
		resp := c.GET("/flights?updatedSince=" + since)
		requireStatus(t, resp, http.StatusOK)
		var page flightListTotals
		resp.MustJSON(t, &page)

		if len(page.MonthTotals) != 0 {
			t.Errorf("monthTotals = %v, want none on a delta request", page.MonthTotals)
		}
		if page.Totals.Flights != len(page.Data) {
			t.Errorf("totals.flights = %d, want the %d changed flights", page.Totals.Flights, len(page.Data))
		}
	})

	t.Run("an empty logbook has zero totals and no months", func(t *testing.T) {
		empty := NewE2EClient(t)
		registerAndLogin(t, empty, uniqueEmail("flt-totals-empty"), "SecurePass123!", "Empty Pilot")
		resp := empty.GET("/flights")
		requireStatus(t, resp, http.StatusOK)
		var page flightListTotals
		resp.MustJSON(t, &page)

		if page.Totals.Flights != 0 || page.Totals.TotalTime != 0 {
			t.Errorf("totals = %+v, want zeros", page.Totals)
		}
		if page.MonthTotals == nil || len(page.MonthTotals) != 0 {
			t.Errorf("monthTotals = %v, want an empty array", page.MonthTotals)
		}
	})
}

type flightListLogbookTotals struct {
	Pagination struct {
		Total int `json:"total"`
	} `json:"pagination"`
	Totals struct {
		Flights   int `json:"flights"`
		TotalTime int `json:"totalTime"`
	} `json:"totals"`
	LogbookTotals *struct {
		Flights       int `json:"flights"`
		TotalTime     int `json:"totalTime"`
		PicTime       int `json:"picTime"`
		LandingsDay   int `json:"landingsDay"`
		LandingsNight int `json:"landingsNight"`
	} `json:"logbookTotals"`
	Baseline *struct {
		BaselineDate string `json:"baselineDate"`
		TotalFlights int    `json:"totalFlights"`
		TotalMinutes int    `json:"totalMinutes"`
	} `json:"baseline"`
	MonthTotals []struct {
		Totals struct {
			TotalTime int `json:"totalTime"`
		} `json:"totals"`
	} `json:"monthTotals"`
}

func TestFlightListLogbookTotals(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-logbook-totals"), "SecurePass123!", "Baseline Pilot")

	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": "2026-08-02", "aircraftReg": "D-ETOT", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
	}), http.StatusCreated)

	get := func(path string) flightListLogbookTotals {
		t.Helper()
		resp := c.GET(path)
		requireStatus(t, resp, http.StatusOK)
		var page flightListLogbookTotals
		resp.MustJSON(t, &page)
		return page
	}

	t.Run("without a snapshot neither field is present", func(t *testing.T) {
		page := get("/flights")
		if page.LogbookTotals != nil || page.Baseline != nil {
			t.Errorf("logbookTotals = %+v, baseline = %+v; want both absent", page.LogbookTotals, page.Baseline)
		}
	})

	requireStatus(t, c.PUT("/users/me/baseline", map[string]interface{}{
		"baselineDate":  "2020-12-31",
		"totalFlights":  12,
		"totalMinutes":  2000,
		"picMinutes":    1800,
		"landingsDay":   36,
		"landingsNight": 2,
	}), http.StatusOK)

	t.Run("an unfiltered list carries the whole logbook", func(t *testing.T) {
		page := get("/flights?pageSize=1")
		if page.LogbookTotals == nil || page.Baseline == nil {
			t.Fatalf("logbookTotals = %+v, baseline = %+v; want both present", page.LogbookTotals, page.Baseline)
		}
		if page.LogbookTotals.Flights != 13 || page.LogbookTotals.TotalTime != 2090 || page.LogbookTotals.LandingsDay != 37 {
			t.Errorf("logbookTotals = %+v, want 13 flights / 2090 min / 37 day landings", *page.LogbookTotals)
		}
		if page.Baseline.TotalFlights != 12 || page.Baseline.TotalMinutes != 2000 || page.Baseline.BaselineDate != "2020-12-31" {
			t.Errorf("baseline = %+v", *page.Baseline)
		}
		if page.Totals.Flights != 1 || page.Totals.TotalTime != 90 || page.Pagination.Total != 1 {
			t.Errorf("totals = %+v, pagination.total = %d; want the logged flight only", page.Totals, page.Pagination.Total)
		}
		for _, m := range page.MonthTotals {
			if m.Totals.TotalTime != 90 {
				t.Errorf("month totals include the snapshot: %+v", m)
			}
		}
	})

	t.Run("a date range reaching back to the snapshot keeps it", func(t *testing.T) {
		page := get("/flights?startDate=2020-01-01")
		if page.LogbookTotals == nil || page.LogbookTotals.Flights != 13 {
			t.Errorf("logbookTotals = %+v, want 13 flights", page.LogbookTotals)
		}
	})

	for name, path := range map[string]string{
		"a filter":                    "/flights?aircraftReg=D-ETOT",
		"a search":                    "/flights?search=EDNY",
		"an advanced query":           "/flights?q=" + url.QueryEscape("reg:D-ETOT"),
		"a window after the snapshot": "/flights?startDate=2026-01-01",
		"a delta request":             "/flights?updatedSince=" + url.QueryEscape(time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)),
	} {
		t.Run(name+" leaves both fields out", func(t *testing.T) {
			page := get(path)
			if page.LogbookTotals != nil || page.Baseline != nil {
				t.Errorf("logbookTotals = %+v, baseline = %+v; want both absent", page.LogbookTotals, page.Baseline)
			}
		})
	}
}
