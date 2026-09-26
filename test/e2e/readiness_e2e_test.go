//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// nextSaturday returns the next Saturday after today (UTC) as YYYY-MM-DD.
func nextSaturday() string {
	d := time.Now().UTC()
	days := (int(time.Saturday) - int(d.Weekday()) + 7) % 7
	if days == 0 {
		days = 7
	}
	return d.AddDate(0, 0, days).Format("2006-01-02")
}

// getReadiness calls GET /currency/readiness with query and returns the report.
func getReadiness(t *testing.T, c *E2EClient, query url.Values) map[string]interface{} {
	t.Helper()
	resp := c.GET("/currency/readiness?" + query.Encode())
	requireStatus(t, resp, http.StatusOK)
	var report map[string]interface{}
	if err := resp.JSON(&report); err != nil {
		t.Fatal(err)
	}
	return report
}

// readinessItems returns the report's items of kind, keyed by launch method,
// ulKind or classType.
func readinessItems(report map[string]interface{}, kind string) map[string]map[string]interface{} {
	out := map[string]map[string]interface{}{}
	items, _ := report["items"].([]interface{})
	for _, raw := range items {
		it := raw.(map[string]interface{})
		if it["kind"] != kind {
			continue
		}
		key, _ := it["classType"].(string)
		if m, ok := it["launchMethod"].(string); ok {
			key = m
		} else if k, ok := it["ulKind"].(string); ok {
			key = k
		} else if id, ok := it["credentialId"].(string); ok {
			key = id
		}
		out[key] = it
	}
	return out
}

func param(it map[string]interface{}, k string) interface{} {
	p, _ := it["params"].(map[string]interface{})
	return p[k]
}

func TestReadiness(t *testing.T) {
	t.Run("L4 Lena: Saturday readiness for solo, winch, aerotow and passengers", func(t *testing.T) {
		c := setupCurrencyUser(t, "ready-lena")
		createAircraftCur(t, c, "D-1234", "ASK21", "GLIDER")
		licID := createLicenseCur(t, c, "EASA", "SPL")
		ratingID := createRatingCur(t, c, licID, "GLIDER", nil)

		createGliderFlightsCur(t, c, "D-1234")
		for i := 0; i < 3; i++ {
			createFlightCur(t, c, map[string]interface{}{
				"date": pastDate(40 + i*10), "aircraftReg": "D-1234", "aircraftType": "ASK21",
				"departureIcao": "EDNY", "arrivalIcao": "EDNY",
				"offBlockTime": "13:00", "onBlockTime": "13:40",
				"landings": 1, "launchMethod": "aerotow",
			})
		}
		resp := c.POST("/credentials", map[string]interface{}{
			"credentialType": "EASA_LAPL_MEDICAL", "issueDate": pastDate(100),
			"expiryDate": futureDate(400), "issuingAuthority": "AME",
		})
		requireStatus(t, resp, http.StatusCreated)
		var medical map[string]interface{}
		resp.JSON(&medical)

		saturday := nextSaturday()
		report := getReadiness(t, c, url.Values{"date": {saturday}, "aircraftReg": {"d-1234"}, "passengers": {"true"}})
		assertStr(t, "date", report["date"], saturday)
		assertStr(t, "aircraftReg", report["aircraftReg"], "D-1234")

		ratings := readinessItems(report, "rating")
		if len(ratings) != 1 {
			t.Fatalf("rating items = %v, want the GLIDER rating only", ratings)
		}
		solo := ratings["GLIDER"]
		assertBool(t, "solo ready", gb(solo, "ready"), true)
		assertStr(t, "solo status", solo["status"], "current")
		assertStr(t, "solo classRatingId", solo["classRatingId"], ratingID)
		assertStr(t, "solo reasonKey", solo["reasonKey"], "rating.recency_current")

		methods := readinessItems(report, "launch_method")
		winch := methods["winch"]
		if winch == nil {
			t.Fatalf("no winch item in %v", methods)
		}
		assertBool(t, "winch ready", gb(winch, "ready"), true)
		assertStr(t, "winch reasonKey", winch["reasonKey"], "readiness.launch_method_current")
		if _, ok := param(winch, "date").(string); !ok {
			t.Errorf("winch params.date missing: %v", winch)
		}

		aerotow := methods["aerotow"]
		if aerotow == nil {
			t.Fatalf("no aerotow item in %v", methods)
		}
		assertBool(t, "aerotow ready", gb(aerotow, "ready"), false)
		assertStr(t, "aerotow status", aerotow["status"], "lapsed")
		assertStr(t, "aerotow reasonKey", aerotow["reasonKey"], "remedy.launch_method_dual")
		assertStr(t, "aerotow params.method", param(aerotow, "method"), "aerotow")
		assertInt(t, "aerotow params.missing", int(param(aerotow, "missing").(float64)), 2)

		pax := readinessItems(report, "passengers")
		if pax["GLIDER"] == nil {
			t.Fatalf("no GLIDER passenger item in %v", report["items"])
		}
		assertBool(t, "passengers ready", gb(pax["GLIDER"], "ready"), true)

		creds := readinessItems(report, "credential")
		med := creds[medical["id"].(string)]
		if med == nil {
			t.Fatalf("no medical item in %v", report["items"])
		}
		assertBool(t, "medical ready", gb(med, "ready"), true)
		assertStr(t, "medical status", med["status"], "valid")
		assertStr(t, "medical reasonKey", med["reasonKey"], "readiness.credential_valid")
	})

	t.Run("L4 Lena: without passengers no passenger item", func(t *testing.T) {
		c := setupCurrencyUser(t, "ready-lena-nopax")
		createAircraftCur(t, c, "D-1234", "ASK21", "GLIDER")
		licID := createLicenseCur(t, c, "EASA", "SPL")
		createRatingCur(t, c, licID, "GLIDER", nil)
		createGliderFlightsCur(t, c, "D-1234")

		report := getReadiness(t, c, url.Values{"aircraftReg": {"D-1234"}})
		if n := len(readinessItems(report, "passengers")); n != 0 {
			t.Errorf("passenger items = %d, want 0", n)
		}
		assertStr(t, "date defaults to today", report["date"], time.Now().UTC().Format("2006-01-02"))
	})

	t.Run("L4 Lena: recency lapses on a date after validUntil", func(t *testing.T) {
		c := setupCurrencyUser(t, "ready-lena-lapse")
		createAircraftCur(t, c, "D-1234", "ASK21", "GLIDER")
		licID := createLicenseCur(t, c, "EASA", "SPL")
		createRatingCur(t, c, licID, "GLIDER", nil)
		// 13 winch launches 600 to 660 days ago and 2 recent training flights:
		// 15 launches, the oldest leaving the 24-month window in about 70 days.
		for i := 0; i < 13; i++ {
			createFlightCur(t, c, map[string]interface{}{
				"date": pastDate(600 + i*5), "aircraftReg": "D-1234", "aircraftType": "ASK21",
				"departureIcao": "EDNY", "arrivalIcao": "EDNY",
				"offBlockTime": "10:00", "onBlockTime": "10:30",
				"landings": 1, "launchMethod": "winch",
			})
		}
		for i := 0; i < 2; i++ {
			createFlightCur(t, c, map[string]interface{}{
				"date": pastDate(10 + i*10), "aircraftReg": "D-1234", "aircraftType": "ASK21",
				"departureIcao": "EDNY", "arrivalIcao": "EDNY",
				"offBlockTime": "11:00", "onBlockTime": "11:30",
				"landings": 1, "launchMethod": "winch",
				"crewMembers": []map[string]interface{}{{"name": "FI", "role": "Instructor"}},
			})
		}

		rc := findRatingCur(getCurrencyStatus(t, c), "GLIDER")
		until, ok := rc["validUntil"].(string)
		if !ok {
			t.Fatalf("GLIDER rating has no validUntil: %v", rc)
		}
		launches := getReq(rc, "requirement.launches")
		assertStr(t, "rating validUntil is the launches row's", launches["validUntil"], until)
		assertStr(t, "launches validUntil", until, time.Now().AddDate(0, 0, -660).AddDate(2, 0, -1).Format("2006-01-02"))
		on := getReadiness(t, c, url.Values{"date": {until}})
		assertBool(t, "ready on validUntil", gb(readinessItems(on, "rating")["GLIDER"], "ready"), true)

		after := getReadiness(t, c, url.Values{"date": {plusDays(until, 1)}})
		item := readinessItems(after, "rating")["GLIDER"]
		assertBool(t, "ready after validUntil", gb(item, "ready"), false)
		assertStr(t, "status after validUntil", item["status"], "lapsed")
		assertStr(t, "reasonKey after validUntil", item["reasonKey"], "remedy.fly_more")
	})

	t.Run("M1 Mehmet: three-axis UL readiness with passengers", func(t *testing.T) {
		c := setupCurrencyUser(t, "ready-mehmet")
		createULAircraftCur(t, c, "D-MXYZ", "THREE_AXIS")
		createULAircraftCur(t, c, "D-MTRK", "WEIGHT_SHIFT")
		licID := createLicenseCur(t, c, "DULV", "UL")
		createULRatingCur(t, c, licID, "THREE_AXIS")
		createULRatingCur(t, c, licID, "WEIGHT_SHIFT")
		createULFlightsCur(t, c, "D-MXYZ")

		report := getReadiness(t, c, url.Values{"aircraftReg": {"D-MXYZ"}, "passengers": {"true"}})
		ratings := readinessItems(report, "rating")
		if len(ratings) != 1 || ratings["THREE_AXIS"] == nil {
			t.Fatalf("rating items = %v, want the THREE_AXIS rating only", ratings)
		}
		assertBool(t, "UL ready", gb(ratings["THREE_AXIS"], "ready"), true)
		assertStr(t, "UL classType", ratings["THREE_AXIS"]["classType"], "ULTRALIGHT")
		pax := readinessItems(report, "passengers")
		if len(pax) != 1 || pax["THREE_AXIS"] == nil {
			t.Fatalf("passenger items = %v, want THREE_AXIS only", pax)
		}
		assertBool(t, "UL passengers ready", gb(pax["THREE_AXIS"], "ready"), true)
		if n := len(readinessItems(report, "launch_method")); n != 0 {
			t.Errorf("launch method items = %d, want 0", n)
		}

		trike := getReadiness(t, c, url.Values{"aircraftReg": {"D-MTRK"}})
		item := readinessItems(trike, "rating")["WEIGHT_SHIFT"]
		if item == nil {
			t.Fatalf("no WEIGHT_SHIFT rating item in %v", trike["items"])
		}
		assertBool(t, "trike ready", gb(item, "ready"), false)
		assertStr(t, "trike status", item["status"], "lapsed")
		assertStr(t, "trike reasonKey", item["reasonKey"], "remedy.fly_more")
	})

	t.Run("A1 Mark: SEP readiness unaffected by gliders", func(t *testing.T) {
		c := setupCurrencyUser(t, "ready-mark")
		createAircraftCur(t, c, "D-EABC", "C172", "SEP_LAND")
		licID := createLicenseCur(t, c, "EASA", "PPL")
		createRatingCur(t, c, licID, "SEP_LAND", strPtr(futureDate(200)))
		createFlightCur(t, c, map[string]interface{}{
			"date": pastDate(5), "aircraftReg": "D-EABC", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		})

		report := getReadiness(t, c, url.Values{"aircraftReg": {"D-EABC"}, "date": {nextSaturday()}})
		items, _ := report["items"].([]interface{})
		if len(items) != 1 {
			t.Fatalf("items = %v, want the SEP rating only", items)
		}
		item := readinessItems(report, "rating")["SEP_LAND"]
		assertBool(t, "SEP ready", gb(item, "ready"), true)
		assertStr(t, "SEP status", item["status"], "expiring")

		rc := findRatingCur(getCurrencyStatus(t, c), "SEP_LAND")
		if _, ok := rc["validUntil"]; ok {
			t.Errorf("FCL.740.A rating carries validUntil: %v", rc["validUntil"])
		}
		for _, r := range rc["requirements"].([]interface{}) {
			if _, ok := r.(map[string]interface{})["validUntil"]; ok {
				t.Errorf("FCL.740.A requirement carries validUntil: %v", r)
			}
		}

		after := getReadiness(t, c, url.Values{"aircraftReg": {"D-EABC"}, "date": {futureDate(250)}})
		expired := readinessItems(after, "rating")["SEP_LAND"]
		assertBool(t, "SEP ready after expiry", gb(expired, "ready"), false)
		assertStr(t, "SEP status after expiry", expired["status"], "expired")
		assertStr(t, "SEP reasonKey after expiry", expired["reasonKey"], "rating.expired")
	})

	t.Run("expired medical is not ready on the date", func(t *testing.T) {
		c := setupCurrencyUser(t, "ready-medical")
		resp := c.POST("/credentials", map[string]interface{}{
			"credentialType": "EASA_CLASS2_MEDICAL", "issueDate": pastDate(300),
			"expiryDate": futureDate(3), "issuingAuthority": "AME",
		})
		requireStatus(t, resp, http.StatusCreated)
		report := getReadiness(t, c, url.Values{"date": {futureDate(10)}})
		creds := readinessItems(report, "credential")
		if len(creds) != 1 {
			t.Fatalf("credential items = %v", creds)
		}
		for _, it := range creds {
			assertBool(t, "medical ready", gb(it, "ready"), false)
			assertStr(t, "medical status", it["status"], "expired")
			assertStr(t, "medical reasonKey", it["reasonKey"], "readiness.credential_expired")
			assertStr(t, "medical params.date", param(it, "date"), futureDate(3))
		}
	})

	t.Run("rejects bad input", func(t *testing.T) {
		c := setupCurrencyUser(t, "ready-bad")
		tests := []struct {
			name  string
			query string
			want  int
		}{
			{"yesterday", "date=" + pastDate(1), http.StatusBadRequest},
			{"367 days ahead", "date=" + time.Now().UTC().AddDate(0, 0, 367).Format("2006-01-02"), http.StatusBadRequest},
			{"not a date", "date=next-saturday", http.StatusBadRequest},
			{"unknown aircraft", "aircraftReg=D-NONE", http.StatusNotFound},
			{"366 days ahead", "date=" + time.Now().UTC().AddDate(0, 0, 366).Format("2006-01-02"), http.StatusOK},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				assertStatus(t, c.GET("/currency/readiness?"+tt.query), tt.want)
			})
		}
	})

	t.Run("another user's aircraft is not found", func(t *testing.T) {
		owner := setupCurrencyUser(t, "ready-owner")
		createAircraftCur(t, owner, "D-OWNR", "ASK21", "GLIDER")
		other := setupCurrencyUser(t, "ready-other")
		assertStatus(t, other.GET("/currency/readiness?aircraftReg=D-OWNR"), http.StatusNotFound)
	})

	t.Run("requires authentication", func(t *testing.T) {
		c := NewE2EClient(t)
		assertStatus(t, c.GET(fmt.Sprintf("/currency/readiness?date=%s", nextSaturday())), http.StatusUnauthorized)
	})
}
