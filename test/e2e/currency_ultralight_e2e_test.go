//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// ─── Ultralight kind, FCL.035(a)(4) crediting and LuftPersV §45 per kind ────

func createULAircraftCur(t *testing.T, c *E2EClient, reg, kind string) {
	t.Helper()
	resp := c.POST("/aircraft", map[string]interface{}{
		"registration": reg, "type": "UL", "make": "Test", "model": "Test",
		"aircraftClass": "ULTRALIGHT", "ulKind": kind,
	})
	requireStatus(t, resp, http.StatusCreated)
}

func createULRatingCur(t *testing.T, c *E2EClient, licID, kind string) string {
	t.Helper()
	resp := c.POST(fmt.Sprintf("/licenses/%s/ratings", licID), map[string]interface{}{
		"classType": "ULTRALIGHT", "issueDate": "2020-01-01", "ulKind": kind,
	})
	requireStatus(t, resp, http.StatusCreated)
	var r map[string]interface{}
	resp.JSON(&r)
	return r["id"].(string)
}

// hourFlights logs n one-hour flights with one landing each on reg.
func hourFlights(t *testing.T, c *E2EClient, reg string, n, firstDaysAgo int, dual bool) {
	t.Helper()
	for i := 0; i < n; i++ {
		f := map[string]interface{}{
			"date": pastDate(firstDaysAgo + i*3), "aircraftReg": reg, "aircraftType": "UL",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00",
			"landings": 1,
		}
		if dual {
			f["crewMembers"] = []map[string]interface{}{{"name": "FI", "role": "Instructor"}}
		}
		createFlightCur(t, c, f)
	}
}

func ratingByID(result map[string]interface{}, id string) map[string]interface{} {
	for _, r := range result["ratings"].([]interface{}) {
		rc := r.(map[string]interface{})
		if rc["classRatingId"] == id {
			return rc
		}
	}
	return nil
}

func stringList(v interface{}) []string {
	var out []string
	if arr, ok := v.([]interface{}); ok {
		for _, x := range arr {
			out = append(out, x.(string))
		}
	}
	return out
}

func TestAircraft_ULKind(t *testing.T) {
	c := setupCurrencyUser(t, "ul-kind-ac")

	resp := c.POST("/aircraft", map[string]interface{}{
		"registration": "D-MKND", "type": "C42", "make": "Ikarus", "model": "C42",
		"aircraftClass": "ULTRALIGHT", "ulKind": "THREE_AXIS",
	})
	requireStatus(t, resp, http.StatusCreated)
	var ac map[string]interface{}
	resp.JSON(&ac)
	assertStr(t, "ulKind", ac["ulKind"], "THREE_AXIS")
	id := ac["id"].(string)

	t.Run("unknown kind rejected", func(t *testing.T) {
		resp := c.POST("/aircraft", map[string]interface{}{
			"registration": "D-MBAD", "type": "C42", "make": "Ikarus", "model": "C42",
			"aircraftClass": "ULTRALIGHT", "ulKind": "HOVERCRAFT",
		})
		requireStatus(t, resp, http.StatusBadRequest)
		if !strings.Contains(string(resp.Body), "Invalid ultralight kind") {
			t.Errorf("body = %s, want invalid ultralight kind", resp.Body)
		}
	})

	t.Run("kind updated", func(t *testing.T) {
		resp := c.PATCH("/aircraft/"+id, map[string]interface{}{"ulKind": "THREE_AXIS_MOTORGLIDER"})
		requireStatus(t, resp, http.StatusOK)
		var got map[string]interface{}
		resp.JSON(&got)
		assertStr(t, "ulKind", got["ulKind"], "THREE_AXIS_MOTORGLIDER")
	})

	t.Run("non-UL class clears kind", func(t *testing.T) {
		resp := c.PATCH("/aircraft/"+id, map[string]interface{}{"aircraftClass": "SEP_LAND"})
		requireStatus(t, resp, http.StatusOK)
		var got map[string]interface{}
		resp.JSON(&got)
		if _, ok := got["ulKind"]; ok {
			t.Errorf("ulKind = %v, want cleared on SEP_LAND", got["ulKind"])
		}
	})

	t.Run("null clears kind", func(t *testing.T) {
		requireStatus(t, c.PATCH("/aircraft/"+id, map[string]interface{}{"aircraftClass": "ULTRALIGHT", "ulKind": "GYROPLANE"}), http.StatusOK)
		resp := c.PATCH("/aircraft/"+id, map[string]interface{}{"ulKind": nil})
		requireStatus(t, resp, http.StatusOK)
		var got map[string]interface{}
		resp.JSON(&got)
		if _, ok := got["ulKind"]; ok {
			t.Errorf("ulKind = %v, want cleared by null", got["ulKind"])
		}
	})
}

func TestClassRating_ULKind(t *testing.T) {
	c := setupCurrencyUser(t, "ul-kind-cr")
	licID := createLicenseCur(t, c, "DULV", "UL")

	resp := c.POST(fmt.Sprintf("/licenses/%s/ratings", licID), map[string]interface{}{
		"classType": "ULTRALIGHT", "issueDate": "2020-01-01", "ulKind": "GYROPLANE",
	})
	requireStatus(t, resp, http.StatusCreated)
	var cr map[string]interface{}
	resp.JSON(&cr)
	assertStr(t, "ulKind", cr["ulKind"], "GYROPLANE")

	t.Run("aircraft-only kind rejected on a rating", func(t *testing.T) {
		resp := c.POST(fmt.Sprintf("/licenses/%s/ratings", licID), map[string]interface{}{
			"classType": "ULTRALIGHT", "issueDate": "2020-01-01", "ulKind": "THREE_AXIS_MOTORGLIDER",
		})
		requireStatus(t, resp, http.StatusBadRequest)
		if !strings.Contains(string(resp.Body), "Invalid ultralight kind") {
			t.Errorf("body = %s, want invalid ultralight kind", resp.Body)
		}
	})

	t.Run("non-UL rating drops kind", func(t *testing.T) {
		resp := c.POST(fmt.Sprintf("/licenses/%s/ratings", licID), map[string]interface{}{
			"classType": "SEP_LAND", "issueDate": "2020-01-01", "ulKind": "GYROPLANE",
		})
		requireStatus(t, resp, http.StatusCreated)
		var got map[string]interface{}
		resp.JSON(&got)
		if _, ok := got["ulKind"]; ok {
			t.Errorf("ulKind = %v, want none on SEP_LAND", got["ulKind"])
		}
	})

	t.Run("kind updated", func(t *testing.T) {
		resp := c.PATCH(fmt.Sprintf("/licenses/%s/ratings/%s", licID, cr["id"]), map[string]interface{}{"ulKind": "THREE_AXIS"})
		requireStatus(t, resp, http.StatusOK)
		var got map[string]interface{}
		resp.JSON(&got)
		assertStr(t, "ulKind", got["ulKind"], "THREE_AXIS")
	})
}

// TestEASA_SEP_ThreeAxisULCredit — FCL.035(a)(4): three-axis UL time and landings
// count toward SEP revalidation; trike time does not; UL dual is not the refresher.
func TestEASA_SEP_ThreeAxisULCredit(t *testing.T) {
	c := setupCurrencyUser(t, "ul-credit-sep")
	createAircraftCur(t, c, "D-EULC", "C172", "SEP_LAND")
	createULAircraftCur(t, c, "D-MULC", "THREE_AXIS")
	createULAircraftCur(t, c, "D-MTRK", "WEIGHT_SHIFT")

	licID := createLicenseCur(t, c, "EASA", "PPL")
	ratingID := createRatingCur(t, c, licID, "SEP_LAND", strPtr(plusDays(pastDate(0), 180)))

	hourFlights(t, c, "D-EULC", 3, 5, false)
	hourFlights(t, c, "D-EULC", 1, 4, true)
	hourFlights(t, c, "D-MULC", 8, 20, false)
	hourFlights(t, c, "D-MTRK", 5, 50, false)

	rc := ratingByID(getCurrencyStatus(t, c), ratingID)
	if rc == nil {
		t.Fatal("SEP_LAND rating currency not found")
	}
	if got := stringList(rc["creditedUltralightKinds"]); len(got) != 1 || got[0] != "THREE_AXIS" {
		t.Errorf("creditedUltralightKinds = %v, want [THREE_AXIS]", got)
	}
	progress := rc["progress"].(map[string]interface{})
	if gf(progress, "totalMinutes") != 720 || gf(progress, "landings") != 12 {
		t.Errorf("progress = %v, want 720 min and 12 landings (SEP + three-axis UL, trike excluded)", progress)
	}
	if req := getReq(rc, "requirement.refresher_training"); req == nil || !gb(req, "met") {
		t.Errorf("refresher = %v, want met by the SEP dual flight", req)
	}
	assertStr(t, "status", rc["status"], "current")
}

func TestEASA_SEP_ULDualIsNotRefresher(t *testing.T) {
	c := setupCurrencyUser(t, "ul-credit-dual")
	createULAircraftCur(t, c, "D-MULD", "THREE_AXIS")

	licID := createLicenseCur(t, c, "EASA", "PPL")
	ratingID := createRatingCur(t, c, licID, "SEP_LAND", strPtr(plusDays(pastDate(0), 180)))

	hourFlights(t, c, "D-MULD", 11, 10, false)
	hourFlights(t, c, "D-MULD", 1, 5, true)

	rc := ratingByID(getCurrencyStatus(t, c), ratingID)
	if rc == nil {
		t.Fatal("SEP_LAND rating currency not found")
	}
	if req := getReq(rc, "requirement.total_time"); req == nil || !gb(req, "met") {
		t.Errorf("total time = %v, want met from UL flights", req)
	}
	if req := getReq(rc, "requirement.refresher_training"); req == nil || gb(req, "met") {
		t.Errorf("refresher = %v, want not met by UL dual", req)
	}
}

// TestGermanUL_GyroplaneCountsGyroplaneOnly — gyroplane recency ignores
// three-axis UL and SEP time.
func TestGermanUL_GyroplaneCountsGyroplaneOnly(t *testing.T) {
	c := setupCurrencyUser(t, "ul-gyro")
	createAircraftCur(t, c, "D-EGYS", "C172", "SEP_LAND")
	createULAircraftCur(t, c, "D-MGY3", "THREE_AXIS")
	createULAircraftCur(t, c, "D-MGYR", "GYROPLANE")

	licID := createLicenseCur(t, c, "DULV", "UL")
	ratingID := createULRatingCur(t, c, licID, "GYROPLANE")

	hourFlights(t, c, "D-EGYS", 12, 10, false)
	hourFlights(t, c, "D-MGY3", 12, 12, false)
	hourFlights(t, c, "D-MGYR", 2, 14, false)

	result := getCurrencyStatus(t, c)
	rc := ratingByID(result, ratingID)
	if rc == nil {
		t.Fatal("gyroplane rating currency not found")
	}
	assertStr(t, "ruleDescriptionKey", rc["ruleDescriptionKey"], "ul_gyroplane")
	if got := gf(rc["progress"].(map[string]interface{}), "totalMinutes"); got != 120 {
		t.Errorf("totalMinutes = %v, want 120 (gyroplane only)", got)
	}
	assertStr(t, "status", rc["status"], "lapsed")

	pc := findPaxCurByAuth(result, "ULTRALIGHT", "DULV")
	if pc == nil {
		t.Fatal("gyroplane passenger currency not found")
	}
	assertStr(t, "ulKind", pc["ulKind"], "GYROPLANE")
	assertStr(t, "dayStatus", pc["dayStatus"], "expired")
}

// TestGermanUL_ThreeAxisCountsSEP — §45(2) counts SEP(land) hours toward
// three-axis recency, the training flight only on a UL.
func TestGermanUL_ThreeAxisCountsSEP(t *testing.T) {
	c := setupCurrencyUser(t, "ul-3ax-sep")
	createAircraftCur(t, c, "D-E3AX", "C172", "SEP_LAND")
	createULAircraftCur(t, c, "D-M3AX", "THREE_AXIS")

	licID := createLicenseCur(t, c, "LBA", "UL")
	ratingID := createULRatingCur(t, c, licID, "THREE_AXIS")

	hourFlights(t, c, "D-E3AX", 10, 10, false)
	hourFlights(t, c, "D-E3AX", 1, 8, true)
	hourFlights(t, c, "D-M3AX", 1, 5, false)

	rc := ratingByID(getCurrencyStatus(t, c), ratingID)
	if rc == nil {
		t.Fatal("three-axis rating currency not found")
	}
	if req := getReq(rc, "requirement.total_time"); req == nil || !gb(req, "met") {
		t.Errorf("total time = %v, want met with SEP hours", req)
	}
	if req := getReq(rc, "requirement.training_flight"); req == nil || gb(req, "met") {
		t.Errorf("training flight = %v, want not met by SEP dual", req)
	}
	assertStr(t, "status", rc["status"], "lapsed")

	hourFlights(t, c, "D-M3AX", 1, 2, true)
	rc = ratingByID(getCurrencyStatus(t, c), ratingID)
	assertStr(t, "status", rc["status"], "current")
}

// TestGermanUL_SEPRatingOnLBALicenceUsesEASA — only ULTRALIGHT ratings take
// the German UL rules.
func TestGermanUL_SEPRatingOnLBALicenceUsesEASA(t *testing.T) {
	c := setupCurrencyUser(t, "ul-lba-sep")
	licID := createLicenseCur(t, c, "LBA", "PPL")
	ratingID := createRatingCur(t, c, licID, "SEP_LAND", strPtr(plusDays(pastDate(0), 180)))

	rc := ratingByID(getCurrencyStatus(t, c), ratingID)
	if rc == nil {
		t.Fatal("SEP_LAND rating currency not found")
	}
	assertStr(t, "ruleDescriptionKey", rc["ruleDescriptionKey"], "easa_sep_tmg")
}

func TestBackup_ULKindRoundTrips(t *testing.T) {
	source := setupCurrencyUser(t, "ul-backup-src")
	createULAircraftCur(t, source, "D-MBKP", "THREE_AXIS_MOTORGLIDER")
	licID := createLicenseCur(t, source, "DAeC", "UL")
	createULRatingCur(t, source, licID, "SAILPLANE")

	backupResp := source.GET("/exports/json")
	requireStatus(t, backupResp, http.StatusOK)
	var backup map[string]interface{}
	if err := json.Unmarshal(backupResp.Body, &backup); err != nil {
		t.Fatalf("invalid export: %v", err)
	}

	dest := setupCurrencyUser(t, "ul-backup-dst")
	requireStatus(t, dest.Do("POST", "/imports/json", backup), http.StatusOK)

	resp := dest.GET("/aircraft")
	requireStatus(t, resp, http.StatusOK)
	var page struct {
		Data []map[string]interface{} `json:"data"`
	}
	resp.JSON(&page)
	if len(page.Data) != 1 {
		t.Fatalf("aircraft = %d, want 1", len(page.Data))
	}
	assertStr(t, "aircraft ulKind", page.Data[0]["ulKind"], "THREE_AXIS_MOTORGLIDER")

	resp = dest.GET("/licenses")
	requireStatus(t, resp, http.StatusOK)
	var licenses []map[string]interface{}
	resp.JSON(&licenses)
	if len(licenses) != 1 {
		t.Fatalf("licenses = %d, want 1", len(licenses))
	}
	resp = dest.GET(fmt.Sprintf("/licenses/%s/ratings", licenses[0]["id"]))
	requireStatus(t, resp, http.StatusOK)
	var ratings []map[string]interface{}
	resp.JSON(&ratings)
	if len(ratings) != 1 {
		t.Fatalf("ratings = %d, want 1", len(ratings))
	}
	assertStr(t, "rating ulKind", ratings[0]["ulKind"], "SAILPLANE")
}

// dualFlights logs n dual flights of the given minutes with one landing each on reg.
func dualFlights(t *testing.T, c *E2EClient, reg string, n, minutes, firstDaysAgo int) {
	t.Helper()
	for i := 0; i < n; i++ {
		createFlightCur(t, c, map[string]interface{}{
			"date": pastDate(firstDaysAgo + i), "aircraftReg": reg, "aircraftType": "UL",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "08:00", "onBlockTime": fmt.Sprintf("%02d:%02d", 8+minutes/60, minutes%60),
			"landings":    1,
			"crewMembers": []map[string]interface{}{{"name": "FI", "role": "Instructor"}},
		})
	}
}

// TestGermanUL_TrainingFlightIsOneFlight — §45(2): the training flight is one
// flight of at least 1h with an instructor, not dual minutes summed.
func TestGermanUL_TrainingFlightIsOneFlight(t *testing.T) {
	c := setupCurrencyUser(t, "ul-m2-training")
	createULAircraftCur(t, c, "D-MXYZ", "THREE_AXIS")
	licID := createLicenseCur(t, c, "DULV", "UL")
	ratingID := createULRatingCur(t, c, licID, "THREE_AXIS")

	hourFlights(t, c, "D-MXYZ", 11, 20, false)

	t.Run("M2 three 20-minute dual flights do not satisfy the training flight", func(t *testing.T) {
		dualFlights(t, c, "D-MXYZ", 3, 20, 5)
		rc := ratingByID(getCurrencyStatus(t, c), ratingID)
		if rc == nil {
			t.Fatal("three-axis rating currency not found")
		}
		req := getReq(rc, "requirement.training_flight")
		if req == nil || gb(req, "met") || gi(req, "current") != 20 {
			t.Errorf("training flight = %v, want unmet with current 20", req)
		}
		if req := getReq(rc, "requirement.total_time"); req == nil || !gb(req, "met") {
			t.Errorf("total time = %v, want met", req)
		}
		assertStr(t, "status", rc["status"], "lapsed")
		assertStr(t, "messageKey", rc["messageKey"], "rating.recency_not_met")
	})

	t.Run("M2 one 60-minute dual flight satisfies", func(t *testing.T) {
		dualFlights(t, c, "D-MXYZ", 1, 60, 2)
		rc := ratingByID(getCurrencyStatus(t, c), ratingID)
		req := getReq(rc, "requirement.training_flight")
		if req == nil || !gb(req, "met") || gi(req, "current") != 60 {
			t.Errorf("training flight = %v, want met with current 60", req)
		}
		assertStr(t, "status", rc["status"], "current")
	})
}

// TestEASA_LAPL_TrainingFlightIsOneFlight — FCL.140.A(a)(1): one refresher
// training flight of at least 1h total time with an instructor.
func TestEASA_LAPL_TrainingFlightIsOneFlight(t *testing.T) {
	c := setupCurrencyUser(t, "lapl-training")
	createAircraftCur(t, c, "D-ELTF", "C172", "SEP_LAND")
	licID := createLicenseCur(t, c, "EASA", "LAPL")
	ratingID := createRatingCur(t, c, licID, "SEP_LAND", nil)

	hourFlights(t, c, "D-ELTF", 11, 20, false)

	t.Run("LAPL three 20-minute dual flights do not satisfy the training flight", func(t *testing.T) {
		dualFlights(t, c, "D-ELTF", 3, 20, 5)
		rc := ratingByID(getCurrencyStatus(t, c), ratingID)
		if rc == nil {
			t.Fatal("LAPL rating currency not found")
		}
		if req := getReq(rc, "requirement.training_flight"); req == nil || gb(req, "met") || gi(req, "current") != 20 {
			t.Errorf("training flight = %v, want unmet with current 20", req)
		}
		assertStr(t, "status", rc["status"], "lapsed")
	})

	t.Run("LAPL one 60-minute dual flight satisfies", func(t *testing.T) {
		dualFlights(t, c, "D-ELTF", 1, 60, 2)
		rc := ratingByID(getCurrencyStatus(t, c), ratingID)
		if req := getReq(rc, "requirement.training_flight"); req == nil || !gb(req, "met") {
			t.Errorf("training flight = %v, want met", req)
		}
		assertStr(t, "status", rc["status"], "current")
	})
}

// TestEASA_SEP_RefresherStaysCumulative — FCL.740.A(b)(1)(ii) "1 hour of flight
// training" may be flown over several flights.
func TestEASA_SEP_RefresherStaysCumulative(t *testing.T) {
	c := setupCurrencyUser(t, "sep-refresher-cum")
	createAircraftCur(t, c, "D-ESRC", "C172", "SEP_LAND")
	licID := createLicenseCur(t, c, "EASA", "PPL")
	ratingID := createRatingCur(t, c, licID, "SEP_LAND", strPtr(plusDays(pastDate(0), 180)))

	hourFlights(t, c, "D-ESRC", 11, 20, false)
	dualFlights(t, c, "D-ESRC", 3, 20, 5)

	rc := ratingByID(getCurrencyStatus(t, c), ratingID)
	if rc == nil {
		t.Fatal("SEP_LAND rating currency not found")
	}
	if req := getReq(rc, "requirement.refresher_training"); req == nil || !gb(req, "met") || gi(req, "current") != 60 {
		t.Errorf("refresher = %v, want met with 60 cumulative minutes", req)
	}
}

// paxByKind returns the passenger currency entries of German ultralight kinds.
func paxByKind(result map[string]interface{}) map[string]map[string]interface{} {
	out := map[string]map[string]interface{}{}
	for _, p := range result["passengerCurrency"].([]interface{}) {
		pc := p.(map[string]interface{})
		if k, ok := pc["ulKind"].(string); ok {
			out[k] = pc
		}
	}
	return out
}

// TestGermanUL_KindlessFlightCountsForNeither — S1: a flight on an ultralight
// with no kind counts for no kind when the pilot holds two, and is reported
// as unclassified until the aircraft's kind is set.
func TestGermanUL_KindlessFlightCountsForNeither(t *testing.T) {
	c := setupCurrencyUser(t, "ul-s1-kindless")
	resp := c.POST("/aircraft", map[string]interface{}{
		"registration": "D-MTRK", "type": "Trike", "make": "Test", "model": "Test", "aircraftClass": "ULTRALIGHT",
	})
	requireStatus(t, resp, http.StatusCreated)
	var ac map[string]interface{}
	resp.JSON(&ac)

	licID := createLicenseCur(t, c, "DULV", "UL")
	trikeID := createULRatingCur(t, c, licID, "WEIGHT_SHIFT")
	ppgID := createULRatingCur(t, c, licID, "POWERED_PARAGLIDER")

	hourFlights(t, c, "D-MTRK", 12, 2, false)

	t.Run("S1 kindless flight with trike and powered-paraglider ratings counts for neither", func(t *testing.T) {
		result := getCurrencyStatus(t, c)
		for name, id := range map[string]string{"trike": trikeID, "powered paraglider": ppgID} {
			rc := ratingByID(result, id)
			if rc == nil {
				t.Fatalf("%s rating currency not found", name)
			}
			assertInt(t, name+" flights", gi(rc["progress"].(map[string]interface{}), "flights"), 0)
			assertInt(t, name+" unclassifiedFlights", gi(rc, "unclassifiedFlights"), 12)
			assertStr(t, name+" status", rc["status"], "lapsed")
		}
		pax := paxByKind(result)
		for _, kind := range []string{"WEIGHT_SHIFT", "POWERED_PARAGLIDER"} {
			if pax[kind] == nil {
				t.Fatalf("%s passenger currency not found", kind)
			}
			assertInt(t, kind+" pax dayLandings", gi(pax[kind], "dayLandings"), 0)
		}
	})

	t.Run("setting the aircraft kind counts the flights for that kind only", func(t *testing.T) {
		requireStatus(t, c.PATCH("/aircraft/"+ac["id"].(string), map[string]interface{}{"ulKind": "WEIGHT_SHIFT"}), http.StatusOK)
		result := getCurrencyStatus(t, c)
		trike := ratingByID(result, trikeID)
		assertInt(t, "trike flights", gi(trike["progress"].(map[string]interface{}), "flights"), 12)
		if _, ok := trike["unclassifiedFlights"]; ok {
			t.Errorf("trike unclassifiedFlights = %v, want absent", trike["unclassifiedFlights"])
		}
		assertStr(t, "trike status", trike["status"], "current")
		ppg := ratingByID(result, ppgID)
		assertInt(t, "powered paraglider flights", gi(ppg["progress"].(map[string]interface{}), "flights"), 0)
		if _, ok := ppg["unclassifiedFlights"]; ok {
			t.Errorf("powered paraglider unclassifiedFlights = %v, want absent", ppg["unclassifiedFlights"])
		}
		pax := paxByKind(result)
		assertInt(t, "trike pax dayLandings", gi(pax["WEIGHT_SHIFT"], "dayLandings"), 12)
		assertInt(t, "powered paraglider pax dayLandings", gi(pax["POWERED_PARAGLIDER"], "dayLandings"), 0)
	})
}

// TestGermanUL_RatingWithoutKindIsUnknown — a UL rating with no kind is not
// evaluated as three-axis and carries no passenger currency.
func TestGermanUL_RatingWithoutKindIsUnknown(t *testing.T) {
	c := setupCurrencyUser(t, "ul-no-kind")
	createAircraftCur(t, c, "D-EUNK", "C172", "SEP_LAND")
	licID := createLicenseCur(t, c, "DULV", "UL")
	ratingID := createRatingCur(t, c, licID, "ULTRALIGHT", nil)
	hourFlights(t, c, "D-EUNK", 12, 10, true)

	result := getCurrencyStatus(t, c)
	rc := ratingByID(result, ratingID)
	if rc == nil {
		t.Fatal("UL rating currency not found")
	}
	assertStr(t, "status", rc["status"], "unknown")
	assertStr(t, "messageKey", rc["messageKey"], "rating.ul_kind_required")
	for _, field := range []string{"requirements", "countedClasses", "creditedUltralightKinds"} {
		if v, ok := rc[field]; ok {
			t.Errorf("%s = %v, want absent", field, v)
		}
	}
	if pc := findPaxCur(result, "ULTRALIGHT"); pc != nil {
		t.Errorf("passenger currency = %v, want none", pc)
	}
}

// TestGermanUL_PassengersNeedTakeoffsAndLandings — §45a: 3 take-offs and 3
// landings in 90 days, not landings alone.
func TestGermanUL_PassengersNeedTakeoffsAndLandings(t *testing.T) {
	c := setupCurrencyUser(t, "ul-pax-takeoffs")
	createULAircraftCur(t, c, "D-MPAX", "THREE_AXIS")
	licID := createLicenseCur(t, c, "DULV", "UL")
	createULRatingCur(t, c, licID, "THREE_AXIS")

	createFlightCur(t, c, map[string]interface{}{
		"date": pastDate(10), "aircraftReg": "D-MPAX", "aircraftType": "C42",
		"departureIcao": "EDNY", "arrivalIcao": "EDNY",
		"offBlockTime": "08:00", "onBlockTime": "09:00",
		"landings": 3, "takeoffsDay": 2,
	})

	t.Run("three landings and two take-offs is not current", func(t *testing.T) {
		pc := paxByKind(getCurrencyStatus(t, c))["THREE_AXIS"]
		if pc == nil {
			t.Fatal("three-axis passenger currency not found")
		}
		assertStr(t, "dayStatus", pc["dayStatus"], "expired")
		assertInt(t, "dayLandings", gi(pc, "dayLandings"), 2)
	})

	t.Run("a third take-off makes it current", func(t *testing.T) {
		createFlightCur(t, c, map[string]interface{}{
			"date": pastDate(5), "aircraftReg": "D-MPAX", "aircraftType": "C42",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "08:00", "onBlockTime": "08:30",
			"landings": 1,
		})
		pc := paxByKind(getCurrencyStatus(t, c))["THREE_AXIS"]
		assertStr(t, "dayStatus without authorisation", pc["dayStatus"], "unknown")
		assertStr(t, "messageKey without authorisation", pc["messageKey"], "pax.ul_authorisation_missing")
		assertInt(t, "dayLandings", gi(pc, "dayLandings"), 3)
		assertStr(t, "dayExpiresOn", pc["dayExpiresOn"], plusDays(pastDate(10), 90))

		requireStatus(t, c.POST("/licenses/"+licID+"/privileges", map[string]interface{}{"kind": "UL_PASSENGER_AUTH"}), http.StatusCreated)
		pc = paxByKind(getCurrencyStatus(t, c))["THREE_AXIS"]
		assertStr(t, "dayStatus", pc["dayStatus"], "current")
	})
}
