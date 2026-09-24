//go:build e2e

package e2e_test

import "testing"

// ─── Cross-class crediting — FCL.140.A / FCL.740.A(b)(1) ────────────────────

// createRecencyFlightsCur logs 11 PIC + 1 instructor flights of 60 min each, one landing per flight.
func createRecencyFlightsCur(t *testing.T, c *E2EClient, reg, acType string) {
	t.Helper()
	for i := 0; i < 11; i++ {
		createFlightCur(t, c, map[string]interface{}{
			"date": pastDate(30 + i*20), "aircraftReg": reg, "aircraftType": acType,
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00",
			"landings": 1,
		})
	}
	createFlightCur(t, c, map[string]interface{}{
		"date": pastDate(10), "aircraftReg": reg, "aircraftType": acType,
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "10:00", "onBlockTime": "11:00",
		"landings":    1,
		"crewMembers": []map[string]interface{}{{"name": "FI", "role": "Instructor"}},
	})
}

func countedClassesCur(rc map[string]interface{}) []string {
	raw, _ := rc["countedClasses"].([]interface{})
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, _ := v.(string)
		out = append(out, s)
	}
	return out
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// TestEASA_LAPL_TMGCountsTowardSEP — LAPL(A) SEP_LAND rating is current from TMG-only flights.
func TestEASA_LAPL_TMGCountsTowardSEP(t *testing.T) {
	c := setupCurrencyUser(t, "easa-lapl-tmgsep")
	createAircraftCur(t, c, "D-KTMG", "SF25", "TMG")

	licID := createLicenseCur(t, c, "EASA", "LAPL(A)")
	createRatingCur(t, c, licID, "SEP_LAND", nil)
	createRatingCur(t, c, licID, "TMG", nil)

	createRecencyFlightsCur(t, c, "D-KTMG", "SF25")

	result := getCurrencyStatus(t, c)
	for _, ct := range []string{"SEP_LAND", "TMG"} {
		rc := findRatingCur(result, ct)
		if rc == nil {
			t.Fatalf("LAPL %s not found", ct)
		}
		assertStr(t, ct+" status", rc["status"], "current")
		progress, _ := rc["progress"].(map[string]interface{})
		assertInt(t, ct+" progress.totalMinutes", gi(progress, "totalMinutes"), 720)
		counted := countedClassesCur(rc)
		if !containsStr(counted, "SEP_LAND") || !containsStr(counted, "TMG") || containsStr(counted, "GLIDER") {
			t.Errorf("%s countedClasses = %v, want aeroplane classes + TMG", ct, counted)
		}
	}

	pc := findPaxCur(result, "SEP_LAND")
	if pc == nil {
		t.Fatal("SEP_LAND passenger currency not found")
	}
	assertInt(t, "SEP_LAND pax dayLandings", gi(pc, "dayLandings"), 0)
}

// TestEASA_LAPL_GliderNotCounted — glider flights do not count toward LAPL(A) recency.
func TestEASA_LAPL_GliderNotCounted(t *testing.T) {
	c := setupCurrencyUser(t, "easa-lapl-glider")
	createAircraftCur(t, c, "D-5815", "ASK21", "GLIDER")

	licID := createLicenseCur(t, c, "EASA", "LAPL")
	createRatingCur(t, c, licID, "SEP_LAND", nil)

	createRecencyFlightsCur(t, c, "D-5815", "ASK21")

	rc := findRatingCur(getCurrencyStatus(t, c), "SEP_LAND")
	if rc == nil {
		t.Fatal("LAPL SEP_LAND not found")
	}
	assertStr(t, "status", rc["status"], "expiring")
}

// TestEASA_LAPL_ProficiencyCheck — a LAPL(A) proficiency check alone satisfies FCL.140.A(a)(2).
func TestEASA_LAPL_ProficiencyCheck(t *testing.T) {
	c := setupCurrencyUser(t, "easa-lapl-profchk")
	createAircraftCur(t, c, "D-ELPC", "C172", "SEP_LAND")

	licID := createLicenseCur(t, c, "EASA", "LAPL")
	createRatingCur(t, c, licID, "SEP_LAND", nil)

	createFlightCur(t, c, map[string]interface{}{
		"date": pastDate(20), "aircraftReg": "D-ELPC", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDNY",
		"offBlockTime": "09:00", "onBlockTime": "10:00",
		"landings": 3, "isProficiencyCheck": true,
	})

	rc := findRatingCur(getCurrencyStatus(t, c), "SEP_LAND")
	if rc == nil {
		t.Fatal("LAPL SEP_LAND not found")
	}
	assertStr(t, "status", rc["status"], "current")
	req := getReq(rc, "requirement.proficiency_check")
	if req == nil {
		t.Fatal("proficiency check requirement missing")
	}
	assertBool(t, "profCheck.met", gb(req, "met"), true)
	assertStr(t, "profCheck.messageKey", req["messageKey"], "requirement.prof_check_completed")
}

// TestEASA_LAPL_LandSeaSplit — SEP(land)+SEP(sea) holders need 1h and 6 landings in each class.
func TestEASA_LAPL_LandSeaSplit(t *testing.T) {
	c := setupCurrencyUser(t, "easa-lapl-landsea")
	createAircraftCur(t, c, "D-ELLS", "C172", "SEP_LAND")

	licID := createLicenseCur(t, c, "EASA", "LAPL")
	createRatingCur(t, c, licID, "SEP_LAND", nil)
	createRatingCur(t, c, licID, "SEP_SEA", nil)

	createRecencyFlightsCur(t, c, "D-ELLS", "C172")

	rc := findRatingCur(getCurrencyStatus(t, c), "SEP_SEA")
	if rc == nil {
		t.Fatal("LAPL SEP_SEA not found")
	}
	assertStr(t, "status", rc["status"], "expiring")
	for _, key := range []string{"requirement.sep_land_time", "requirement.sep_land_landings"} {
		req := getReq(rc, key)
		if req == nil {
			t.Fatalf("%s missing", key)
		}
		assertBool(t, key+".met", gb(req, "met"), true)
	}
	for _, key := range []string{"requirement.sep_sea_time", "requirement.sep_sea_landings"} {
		req := getReq(rc, key)
		if req == nil {
			t.Fatalf("%s missing", key)
		}
		assertBool(t, key+".met", gb(req, "met"), false)
	}
}

// TestEASA_PPL_SEPTMG_Combined — PPL with SEP_LAND and TMG ratings pools both classes.
func TestEASA_PPL_SEPTMG_Combined(t *testing.T) {
	c := setupCurrencyUser(t, "easa-ppl-septmg")
	createAircraftCur(t, c, "D-ESTA", "C172", "SEP_LAND")
	createAircraftCur(t, c, "D-KSTB", "SF25", "TMG")

	licID := createLicenseCur(t, c, "EASA", "PPL")
	expiry := futureDate(200)
	createRatingCur(t, c, licID, "SEP_LAND", &expiry)
	createRatingCur(t, c, licID, "TMG", &expiry)

	for i := 0; i < 6; i++ {
		for _, ac := range []struct{ reg, typ string }{{"D-ESTA", "C172"}, {"D-KSTB", "SF25"}} {
			createFlightCur(t, c, map[string]interface{}{
				"date": pastDate(30 + i*20), "aircraftReg": ac.reg, "aircraftType": ac.typ,
				"departureIcao": "EDNY", "arrivalIcao": "EDDS",
				"offBlockTime": "08:00", "onBlockTime": "09:00",
				"landings": 1,
			})
		}
	}
	createFlightCur(t, c, map[string]interface{}{
		"date": pastDate(10), "aircraftReg": "D-KSTB", "aircraftType": "SF25",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "10:00", "onBlockTime": "11:00",
		"landings":    1,
		"crewMembers": []map[string]interface{}{{"name": "FI", "role": "Instructor"}},
	})

	result := getCurrencyStatus(t, c)
	for _, ct := range []string{"SEP_LAND", "TMG"} {
		rc := findRatingCur(result, ct)
		if rc == nil {
			t.Fatalf("%s not found", ct)
		}
		assertStr(t, ct+" status", rc["status"], "current")
		progress, _ := rc["progress"].(map[string]interface{})
		assertInt(t, ct+" progress.landings", gi(progress, "landings"), 13)
		counted := countedClassesCur(rc)
		if len(counted) != 2 || !containsStr(counted, "SEP_LAND") || !containsStr(counted, "TMG") {
			t.Errorf("%s countedClasses = %v, want [SEP_LAND TMG]", ct, counted)
		}
	}
}

// TestEASA_PPL_SEPOnly_TMGNotCounted — without a TMG rating, TMG flights do not count for SEP_LAND.
func TestEASA_PPL_SEPOnly_TMGNotCounted(t *testing.T) {
	c := setupCurrencyUser(t, "easa-ppl-seponly")
	createAircraftCur(t, c, "D-KSOC", "SF25", "TMG")

	licID := createLicenseCur(t, c, "EASA", "PPL")
	expiry := futureDate(200)
	createRatingCur(t, c, licID, "SEP_LAND", &expiry)

	createRecencyFlightsCur(t, c, "D-KSOC", "SF25")

	rc := findRatingCur(getCurrencyStatus(t, c), "SEP_LAND")
	if rc == nil {
		t.Fatal("SEP_LAND not found")
	}
	assertStr(t, "status", rc["status"], "expiring")
	if _, ok := rc["countedClasses"]; ok {
		t.Errorf("countedClasses = %v, want absent", rc["countedClasses"])
	}
}
