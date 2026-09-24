//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

type customReportRow struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Value     int    `json:"value"`
	Flights   int    `json:"flights"`
	TotalTime int    `json:"totalTime"`
	FstdTime  int    `json:"fstdTime"`
	Landings  int    `json:"landings"`
}

type customReportResult struct {
	GroupBy   string            `json:"groupBy"`
	Metric    string            `json:"metric"`
	StartDate string            `json:"startDate"`
	EndDate   string            `json:"endDate"`
	Rows      []customReportRow `json:"rows"`
	Totals    struct {
		Flights   int `json:"flights"`
		TotalTime int `json:"totalTime"`
		FstdTime  int `json:"fstdTime"`
		Landings  int `json:"landings"`
	} `json:"totals"`
	OtherGroups int `json:"otherGroups"`
}

type customReportBody struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Position   int                    `json:"position"`
	Definition map[string]interface{} `json:"definition"`
}

// seedCustomReportFlights logs three flights and an FSTD session inside
// Jan–Jun 2024 and one flight before it.
func seedCustomReportFlights(t *testing.T, c *E2EClient) {
	t.Helper()
	for _, f := range []map[string]interface{}{
		{"date": "2024-03-10", "aircraftReg": "D-ECRA", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDDS", "offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1},
		{"date": "2024-03-20", "aircraftReg": "D-ECRA", "aircraftType": "C172", "departureIcao": "EDDS", "arrivalIcao": "EDNY", "offBlockTime": "10:00", "onBlockTime": "11:00", "landings": 1},
		{"date": "2024-05-05", "aircraftReg": "D-ECRB", "aircraftType": "PA28", "departureIcao": "EDNY", "arrivalIcao": "EDTL", "offBlockTime": "08:00", "onBlockTime": "10:00", "landings": 1},
		{"date": "2023-12-01", "aircraftReg": "D-ECRA", "aircraftType": "C172", "departureIcao": "EDNY", "arrivalIcao": "EDNY", "offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1},
		{"date": "2024-03-15", "aircraftType": "PA34", "isSimulator": true, "fstdType": "FNPT II", "simulatedFlightTime": 60},
	} {
		requireStatus(t, c.POST("/flights", f), http.StatusCreated)
	}
}

func monthlyTimeReport() map[string]interface{} {
	return map[string]interface{}{
		"filter":  map[string]interface{}{},
		"window":  map[string]interface{}{"kind": "range", "startDate": "2024-01-01", "endDate": "2024-06-30"},
		"groupBy": "month",
		"metric":  "totalTime",
	}
}

func createCustomReport(t *testing.T, c *E2EClient, name string, def map[string]interface{}) customReportBody {
	t.Helper()
	resp := c.POST("/reports/custom", map[string]interface{}{"name": name, "definition": def})
	requireStatus(t, resp, http.StatusCreated)
	var rep customReportBody
	if err := resp.JSON(&rep); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if rep.ID == "" {
		t.Fatalf("created report has no id: %s", resp.Body)
	}
	return rep
}

func customReportResultOf(t *testing.T, c *E2EClient, id string) customReportResult {
	t.Helper()
	resp := c.GET("/reports/custom/" + id + "/result")
	requireStatus(t, resp, http.StatusOK)
	var res customReportResult
	if err := resp.JSON(&res); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return res
}

func listCustomReportNames(t *testing.T, c *E2EClient) []string {
	t.Helper()
	resp := c.GET("/reports/custom")
	requireStatus(t, resp, http.StatusOK)
	var list []customReportBody
	if err := resp.JSON(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	names := []string{}
	for _, r := range list {
		names = append(names, r.Name)
	}
	return names
}

func TestCustomReports(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("creport"), "SecurePass123!", "Custom Report")
	seedCustomReportFlights(t, c)

	monthly := createCustomReport(t, c, "Monthly hours", monthlyTimeReport())
	if monthly.Position != 0 {
		t.Errorf("first report position = %d, want 0", monthly.Position)
	}

	t.Run("monthly result is gap-filled and excludes FSTD time", func(t *testing.T) {
		res := customReportResultOf(t, c, monthly.ID)
		var keys []string
		for _, r := range res.Rows {
			keys = append(keys, r.Key)
		}
		want := []string{"2024-01", "2024-02", "2024-03", "2024-04", "2024-05", "2024-06"}
		if !reflect.DeepEqual(keys, want) {
			t.Fatalf("keys = %v, want %v", keys, want)
		}
		march := res.Rows[2]
		assertInt(t, "march value", march.Value, 150)
		assertInt(t, "march flights", march.Flights, 2)
		assertInt(t, "march fstdTime", march.FstdTime, 60)
		assertInt(t, "may value", res.Rows[4].Value, 120)
		assertInt(t, "totals flights", res.Totals.Flights, 3)
		assertInt(t, "totals totalTime", res.Totals.TotalTime, 270)
		assertInt(t, "totals fstdTime", res.Totals.FstdTime, 60)
		assertStr(t, "startDate", res.StartDate, "2024-01-01")
		assertStr(t, "endDate", res.EndDate, "2024-06-30")
		landings := 0
		for _, r := range res.Rows {
			landings += r.Landings
		}
		assertInt(t, "landings sum", landings, res.Totals.Landings)
	})

	ranked := createCustomReport(t, c, "Types", map[string]interface{}{
		"filter":  map[string]interface{}{"q": "reg:D-ECR*"},
		"window":  map[string]interface{}{"kind": "all"},
		"groupBy": "aircraftType",
		"metric":  "flights",
		"limit":   1,
	})

	t.Run("ranked result applies q and limit", func(t *testing.T) {
		res := customReportResultOf(t, c, ranked.ID)
		if len(res.Rows) != 1 || res.Rows[0].Key != "C172" || res.Rows[0].Value != 3 {
			t.Fatalf("rows = %+v", res.Rows)
		}
		assertInt(t, "otherGroups", res.OtherGroups, 1)
		assertInt(t, "totals flights", res.Totals.Flights, 4)
		if res.StartDate != "" || res.EndDate != "" {
			t.Errorf("all-time window reported bounds %q..%q", res.StartDate, res.EndDate)
		}
	})

	t.Run("structured filters and role", func(t *testing.T) {
		resp := c.POST("/reports/custom/preview", map[string]interface{}{"definition": map[string]interface{}{
			"filter":  map[string]interface{}{"aircraftReg": "d-ecrb", "departureIcao": "edny", "role": "pic"},
			"window":  map[string]interface{}{"kind": "all"},
			"groupBy": "route",
			"metric":  "totalTime",
		}})
		requireStatus(t, resp, http.StatusOK)
		var res customReportResult
		resp.JSON(&res)
		if len(res.Rows) != 1 || res.Rows[0].Key != "EDNY-EDTL" || res.Rows[0].Value != 120 {
			t.Fatalf("rows = %+v", res.Rows)
		}
	})

	t.Run("list, reorder and get", func(t *testing.T) {
		if got := listCustomReportNames(t, c); !reflect.DeepEqual(got, []string{"Monthly hours", "Types"}) {
			t.Fatalf("list = %v", got)
		}
		resp := c.PUT("/reports/custom/order", map[string]interface{}{"reportIds": []string{ranked.ID, monthly.ID}})
		requireStatus(t, resp, http.StatusOK)
		if got := listCustomReportNames(t, c); !reflect.DeepEqual(got, []string{"Types", "Monthly hours"}) {
			t.Fatalf("list after reorder = %v", got)
		}
		assertStatus(t, c.PUT("/reports/custom/order", map[string]interface{}{"reportIds": []string{ranked.ID}}), http.StatusBadRequest)
		requireStatus(t, c.GET("/reports/custom/"+monthly.ID), http.StatusOK)
	})

	t.Run("update", func(t *testing.T) {
		def := monthlyTimeReport()
		def["metric"] = "flights"
		resp := c.PUT("/reports/custom/"+monthly.ID, map[string]interface{}{"name": "Monthly flights", "definition": def})
		requireStatus(t, resp, http.StatusOK)
		var rep customReportBody
		resp.JSON(&rep)
		assertStr(t, "name", rep.Name, "Monthly flights")
		assertInt(t, "position kept", rep.Position, 1)
		res := customReportResultOf(t, c, monthly.ID)
		assertStr(t, "metric", res.Metric, "flights")
		assertInt(t, "march value", res.Rows[2].Value, 2)
	})

	t.Run("validation", func(t *testing.T) {
		bad := []map[string]interface{}{
			{"filter": map[string]interface{}{"q": "reg:("}, "window": map[string]interface{}{"kind": "all"}, "groupBy": "month", "metric": "flights"},
			{"filter": map[string]interface{}{}, "window": map[string]interface{}{"kind": "all"}, "groupBy": "pilot", "metric": "flights"},
			{"filter": map[string]interface{}{}, "window": map[string]interface{}{"kind": "all"}, "groupBy": "month", "metric": "fuel"},
			{"filter": map[string]interface{}{}, "window": map[string]interface{}{"kind": "lastMonths"}, "groupBy": "month", "metric": "flights"},
			{"filter": map[string]interface{}{}, "window": map[string]interface{}{"kind": "range", "startDate": "2024-05-01", "endDate": "2024-01-01"}, "groupBy": "month", "metric": "flights"},
		}
		for i, def := range bad {
			if r := c.POST("/reports/custom/preview", map[string]interface{}{"definition": def}); r.StatusCode != http.StatusBadRequest {
				t.Errorf("bad definition %d: status %d, body %s", i, r.StatusCode, r.Body)
			}
			if r := c.POST("/reports/custom", map[string]interface{}{"name": "x", "definition": def}); r.StatusCode != http.StatusBadRequest {
				t.Errorf("bad definition %d on create: status %d", i, r.StatusCode)
			}
		}
		assertStatus(t, c.POST("/reports/custom", map[string]interface{}{"name": " ", "definition": monthlyTimeReport()}), http.StatusBadRequest)
		assertStatus(t, c.POST("/reports/custom", map[string]interface{}{"name": strings.Repeat("n", 121), "definition": monthlyTimeReport()}), http.StatusBadRequest)
	})

	t.Run("export csv", func(t *testing.T) {
		resp := c.GET("/reports/custom/" + ranked.ID + "/export?format=csv")
		requireStatus(t, resp, http.StatusOK)
		if ct := resp.Headers.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
			t.Errorf("Content-Type = %q", ct)
		}
		if cd := resp.Headers.Get("Content-Disposition"); !strings.Contains(cd, "attachment; filename=ninerlog_report_types_") || !strings.HasSuffix(cd, ".csv") {
			t.Errorf("Content-Disposition = %q", cd)
		}
		body := string(resp.Body)
		if !strings.HasPrefix(body, "Aircraft type,Flights,Total time,") || !strings.Contains(body, "\nC172,3,") || !strings.Contains(body, "\nTotal,4,") {
			t.Errorf("csv =\n%s", body)
		}
	})

	t.Run("export pdf", func(t *testing.T) {
		resp := c.GET("/reports/custom/" + monthly.ID + "/export?format=pdf")
		requireStatus(t, resp, http.StatusOK)
		if ct := resp.Headers.Get("Content-Type"); ct != "application/pdf" {
			t.Errorf("Content-Type = %q", ct)
		}
		if !bytes.HasPrefix(resp.Body, []byte("%PDF-")) {
			t.Error("body is not a PDF")
		}
		assertStatus(t, c.GET("/reports/custom/"+monthly.ID+"/export?format=xlsx"), http.StatusBadRequest)
		assertStatus(t, c.GET("/reports/custom/"+monthly.ID+"/export"), http.StatusBadRequest)
	})

	t.Run("another user cannot reach the reports", func(t *testing.T) {
		o := NewE2EClient(t)
		registerAndLogin(t, o, uniqueEmail("creport-other"), "SecurePass123!", "Other")
		assertStatus(t, o.GET("/reports/custom/"+monthly.ID), http.StatusNotFound)
		assertStatus(t, o.GET("/reports/custom/"+monthly.ID+"/result"), http.StatusNotFound)
		assertStatus(t, o.GET("/reports/custom/"+monthly.ID+"/export?format=csv"), http.StatusNotFound)
		assertStatus(t, o.PUT("/reports/custom/"+monthly.ID, map[string]interface{}{"name": "x", "definition": monthlyTimeReport()}), http.StatusNotFound)
		assertStatus(t, o.DELETE("/reports/custom/"+monthly.ID), http.StatusNotFound)
		assertStatus(t, o.PUT("/reports/custom/order", map[string]interface{}{"reportIds": []string{monthly.ID}}), http.StatusBadRequest)
		if got := listCustomReportNames(t, o); len(got) != 0 {
			t.Errorf("other user lists %v", got)
		}
	})

	t.Run("delete", func(t *testing.T) {
		assertStatus(t, c.DELETE("/reports/custom/"+ranked.ID), http.StatusNoContent)
		assertStatus(t, c.GET("/reports/custom/"+ranked.ID), http.StatusNotFound)
		assertStatus(t, c.DELETE("/reports/custom/"+ranked.ID), http.StatusNotFound)
	})

	t.Run("no auth returns 401", func(t *testing.T) {
		anon := NewE2EClient(t)
		assertStatus(t, anon.GET("/reports/custom"), http.StatusUnauthorized)
		assertStatus(t, anon.GET("/reports/custom/"+monthly.ID+"/result"), http.StatusUnauthorized)
	})
}

func TestCustomReportLicenceScopeAndPortability(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("creport-lic"), "SecurePass123!", "Custom Report Licence")
	createAircraftCur(t, c, "D-ECRA", "C172", "SEP_LAND")
	createAircraftCur(t, c, "D-ECRB", "PA28", "MEP_LAND")
	licID := createLicenseCur(t, c, "EASA", "PPL")
	createRatingCur(t, c, licID, "SEP_LAND", nil)
	seedCustomReportFlights(t, c)

	def := monthlyTimeReport()
	def["filter"] = map[string]interface{}{"logbookLicenseId": licID}
	scoped := createCustomReport(t, c, "SEP logbook", def)
	createCustomReport(t, c, "Monthly hours", monthlyTimeReport())

	t.Run("licence scope restricts to rated classes", func(t *testing.T) {
		res := customReportResultOf(t, c, scoped.ID)
		assertInt(t, "flights", res.Totals.Flights, 2)
		assertInt(t, "totalTime", res.Totals.TotalTime, 150)
	})

	t.Run("foreign licence is rejected", func(t *testing.T) {
		o := NewE2EClient(t)
		registerAndLogin(t, o, uniqueEmail("creport-lic-other"), "SecurePass123!", "Other")
		assertStatus(t, o.POST("/reports/custom", map[string]interface{}{"name": "x", "definition": def}), http.StatusBadRequest)
	})

	t.Run("backup round trip keeps order and re-points the licence", func(t *testing.T) {
		resp := c.GET("/exports/json")
		requireStatus(t, resp, http.StatusOK)
		var backup map[string]interface{}
		if err := json.Unmarshal(resp.Body, &backup); err != nil {
			t.Fatal(err)
		}
		reports, ok := backup["customReports"].([]interface{})
		if !ok || len(reports) != 2 {
			t.Fatalf("customReports in backup = %v", backup["customReports"])
		}
		if name := reports[0].(map[string]interface{})["name"]; name != "SEP logbook" {
			t.Errorf("first backed-up report = %v", name)
		}

		dest := NewE2EClient(t)
		registerAndLogin(t, dest, uniqueEmail("creport-dest"), "SecurePass123!", "Dest")
		restore := dest.Do("POST", "/imports/json", backup)
		requireStatus(t, restore, http.StatusOK)
		var summary struct {
			CustomReportsImported int `json:"customReportsImported"`
		}
		restore.JSON(&summary)
		assertInt(t, "customReportsImported", summary.CustomReportsImported, 2)

		list := dest.GET("/reports/custom")
		requireStatus(t, list, http.StatusOK)
		var restored []customReportBody
		list.JSON(&restored)
		if len(restored) != 2 || restored[0].Name != "SEP logbook" || restored[1].Name != "Monthly hours" {
			t.Fatalf("restored = %+v", restored)
		}
		filter := restored[0].Definition["filter"].(map[string]interface{})
		newLic, _ := filter["logbookLicenseId"].(string)
		if newLic == "" || newLic == licID {
			t.Fatalf("logbookLicenseId = %q, want the restored licence", newLic)
		}
		res := customReportResultOf(t, dest, restored[0].ID)
		assertInt(t, "restored scoped flights", res.Totals.Flights, 2)
	})
}
