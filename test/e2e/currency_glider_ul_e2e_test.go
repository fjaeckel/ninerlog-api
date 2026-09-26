//go:build e2e

package e2e_test

import "testing"

// ─── GLIDER / ULTRALIGHT class ratings ──────────────────────────────────────

// createGliderFlightsCur logs 13 PIC + 2 instructor winch flights of 30 min each.
func createGliderFlightsCur(t *testing.T, c *E2EClient, reg string) {
	t.Helper()
	for i := 0; i < 13; i++ {
		createFlightCur(t, c, map[string]interface{}{
			"date": pastDate(30 + i*15), "aircraftReg": reg, "aircraftType": "ASK21",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "10:00", "onBlockTime": "10:30",
			"landings": 1, "launchMethod": "winch",
		})
	}
	for i := 0; i < 2; i++ {
		createFlightCur(t, c, map[string]interface{}{
			"date": pastDate(10 + i*10), "aircraftReg": reg, "aircraftType": "ASK21",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "11:00", "onBlockTime": "11:30",
			"landings": 1, "launchMethod": "winch",
			"crewMembers": []map[string]interface{}{{"name": "FI", "role": "Instructor"}},
		})
	}
}

// createULFlightsCur logs 11 PIC + 1 instructor flights of 60 min each.
func createULFlightsCur(t *testing.T, c *E2EClient, reg string) {
	t.Helper()
	for i := 0; i < 11; i++ {
		createFlightCur(t, c, map[string]interface{}{
			"date": pastDate(30 + i*20), "aircraftReg": reg, "aircraftType": "C42",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00",
			"landings": 1,
		})
	}
	createFlightCur(t, c, map[string]interface{}{
		"date": pastDate(5), "aircraftReg": reg, "aircraftType": "C42",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "10:00", "onBlockTime": "11:00",
		"landings":    1,
		"crewMembers": []map[string]interface{}{{"name": "FI", "role": "Instructor"}},
	})
}

// TestEASA_GliderClass_Current — GLIDER rating counts flights on GLIDER aircraft under SFCL.160(a).
func TestEASA_GliderClass_Current(t *testing.T) {
	c := setupCurrencyUser(t, "easa-glider-cls")
	createAircraftCur(t, c, "D-5812", "ASK21", "GLIDER")
	createAircraftCur(t, c, "D-EGLD", "C172", "SEP_LAND")

	licID := createLicenseCur(t, c, "EASA", "SPL")
	createRatingCur(t, c, licID, "GLIDER", nil)

	createGliderFlightsCur(t, c, "D-5812")
	createFlightCur(t, c, map[string]interface{}{
		"date": pastDate(3), "aircraftReg": "D-EGLD", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:00",
		"landings": 1,
	})

	result := getCurrencyStatus(t, c)
	rc := findRatingCur(result, "GLIDER")
	if rc == nil {
		t.Fatal("GLIDER rating currency not found")
	}
	assertStr(t, "status", rc["status"], "current")
	assertStr(t, "ruleDescriptionKey", rc["ruleDescriptionKey"], "easa_spl")
	progress, _ := rc["progress"].(map[string]interface{})
	assertInt(t, "progress.landings", gi(progress, "landings"), 15)
	assertInt(t, "progress.launches", gi(progress, "launches"), 15)
	assertInt(t, "progress.trainingFlights", gi(progress, "trainingFlights"), 2)

	pc := findPaxCur(result, "GLIDER")
	if pc == nil {
		t.Fatal("GLIDER passenger currency not found")
	}
	assertBool(t, "nightPrivilege", gb(pc, "nightPrivilege"), false)
}

// TestEASA_GliderClass_NonSPLLicense — GLIDER rating on a non-SPL license still uses SFCL.160(a).
func TestEASA_GliderClass_NonSPLLicense(t *testing.T) {
	c := setupCurrencyUser(t, "easa-glider-ppl")
	createAircraftCur(t, c, "D-5813", "ASK21", "GLIDER")

	licID := createLicenseCur(t, c, "EASA", "Segelflug")
	createRatingCur(t, c, licID, "GLIDER", nil)

	createGliderFlightsCur(t, c, "D-5813")

	result := getCurrencyStatus(t, c)
	rc := findRatingCur(result, "GLIDER")
	if rc == nil {
		t.Fatal("GLIDER rating currency not found")
	}
	assertStr(t, "status", rc["status"], "current")
	assertStr(t, "ruleDescriptionKey", rc["ruleDescriptionKey"], "easa_spl")
}

// TestGliderClass_CaseInsensitiveAircraftClass — aircraft class "glider " matches a GLIDER rating.
func TestGliderClass_CaseInsensitiveAircraftClass(t *testing.T) {
	c := setupCurrencyUser(t, "glider-lowercase")
	createAircraftCur(t, c, "D-5814", "ASK21", "glider ")

	licID := createLicenseCur(t, c, "EASA", "SPL")
	createRatingCur(t, c, licID, "GLIDER", nil)

	createGliderFlightsCur(t, c, "D-5814")

	result := getCurrencyStatus(t, c)
	rc := findRatingCur(result, "GLIDER")
	if rc == nil {
		t.Fatal("GLIDER rating currency not found")
	}
	assertStr(t, "status", rc["status"], "current")
}

// TestUltralightClass_GermanAuthorityOnly — LuftPersV §45 applies to ULTRALIGHT only under LBA/DULV/DAeC.
func TestUltralightClass_GermanAuthorityOnly(t *testing.T) {
	c := setupCurrencyUser(t, "ul-cls-auth")
	createAircraftCur(t, c, "D-MULC", "C42", "ULTRALIGHT")

	easaID := createLicenseCur(t, c, "EASA", "UL")
	createRatingCur(t, c, easaID, "ULTRALIGHT", strPtr(plusDays(pastDate(0), 365)))
	createULFlightsCur(t, c, "D-MULC")

	result := getCurrencyStatus(t, c)
	rc := findRatingCurByAuth(result, "ULTRALIGHT", "EASA")
	if rc == nil {
		t.Fatal("EASA ULTRALIGHT rating currency not found")
	}
	if key, _ := rc["ruleDescriptionKey"].(string); key != "" {
		t.Errorf("EASA ULTRALIGHT ruleDescriptionKey = %q, want expiry-only", key)
	}
	if pc := findPaxCurByAuth(result, "ULTRALIGHT", "EASA"); pc != nil {
		t.Error("EASA ULTRALIGHT passenger currency present, want none")
	}

	dulvID := createLicenseCur(t, c, "DULV", "UL")
	createRatingCur(t, c, dulvID, "ULTRALIGHT", nil)

	result = getCurrencyStatus(t, c)
	rc = findRatingCurByAuth(result, "ULTRALIGHT", "DULV")
	if rc == nil {
		t.Fatal("DULV ULTRALIGHT rating currency not found")
	}
	assertStr(t, "status", rc["status"], "current")
	assertStr(t, "ruleDescriptionKey", rc["ruleDescriptionKey"], "ul_luftpersv")
	pc := findPaxCurByAuth(result, "ULTRALIGHT", "DULV")
	if pc == nil {
		t.Fatal("DULV ULTRALIGHT passenger currency not found")
	}
	assertStr(t, "ruleDescriptionKey", pc["ruleDescriptionKey"], "ul_pax")
}

// TestTowedFlights_ExcludedFromPoweredClass — winch/aerotow flights on a SEP_LAND aircraft don't count toward a PPL SEP rating.
func TestTowedFlights_ExcludedFromPoweredClass(t *testing.T) {
	c := setupCurrencyUser(t, "towed-sep")
	createAircraftCur(t, c, "D-0TOW", "ASK21", "SEP_LAND")
	createAircraftCur(t, c, "D-ETOW", "C172", "SEP_LAND")

	licID := createLicenseCur(t, c, "EASA", "PPL")
	createRatingCur(t, c, licID, "SEP_LAND", strPtr(plusDays(pastDate(0), 180)))

	createGliderFlightsCur(t, c, "D-0TOW")
	createFlightCur(t, c, map[string]interface{}{
		"date": pastDate(4), "aircraftReg": "D-ETOW", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:00",
		"landings": 1,
	})

	result := getCurrencyStatus(t, c)
	rc := findRatingCur(result, "SEP_LAND")
	if rc == nil {
		t.Fatal("SEP_LAND rating currency not found")
	}
	progress, _ := rc["progress"].(map[string]interface{})
	assertInt(t, "progress.flights", gi(progress, "flights"), 1)
	assertInt(t, "progress.landings", gi(progress, "landings"), 1)

	pc := findPaxCur(result, "SEP_LAND")
	if pc == nil {
		t.Fatal("SEP_LAND passenger currency not found")
	}
	assertInt(t, "dayLandings", gi(pc, "dayLandings"), 1)
}

// TestLaunchCounts_FilteredByClass — launches on other classes don't appear in a GLIDER rating's launch-method currency.
func TestLaunchCounts_FilteredByClass(t *testing.T) {
	c := setupCurrencyUser(t, "launch-cls")
	createAircraftCur(t, c, "D-5816", "ASK21", "GLIDER")
	createAircraftCur(t, c, "D-MLCH", "C42", "ULTRALIGHT")

	licID := createLicenseCur(t, c, "EASA", "SPL")
	createRatingCur(t, c, licID, "GLIDER", nil)

	createGliderFlightsCur(t, c, "D-5816")
	for i := 0; i < 3; i++ {
		createFlightCur(t, c, map[string]interface{}{
			"date": pastDate(20 + i), "aircraftReg": "D-MLCH", "aircraftType": "C42",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "08:00", "onBlockTime": "08:30",
			"landings": 1, "launchMethod": "aerotow",
		})
	}

	result := getCurrencyStatus(t, c)
	rc := findRatingCur(result, "GLIDER")
	if rc == nil {
		t.Fatal("GLIDER rating currency not found")
	}
	methods, _ := rc["launchMethodCurrency"].([]interface{})
	if len(methods) != 1 {
		t.Fatalf("launchMethodCurrency = %v, want winch only", methods)
	}
	m, _ := methods[0].(map[string]interface{})
	assertStr(t, "method", m["method"], "winch")
	assertInt(t, "launches", gi(m, "launches"), 15)
}

// TestGliderAndUltralight_SeparateRatings — glider and UL flights each count only toward their own rating.
func TestGliderAndUltralight_SeparateRatings(t *testing.T) {
	c := setupCurrencyUser(t, "glider-ul-split")
	createAircraftCur(t, c, "D-5815", "ASK21", "GLIDER")
	createAircraftCur(t, c, "D-MSPL", "C42", "ULTRALIGHT")

	splID := createLicenseCur(t, c, "EASA", "SPL")
	createRatingCur(t, c, splID, "GLIDER", nil)
	ulID := createLicenseCur(t, c, "DULV", "UL")
	createRatingCur(t, c, ulID, "ULTRALIGHT", nil)

	createGliderFlightsCur(t, c, "D-5815")
	createULFlightsCur(t, c, "D-MSPL")

	result := getCurrencyStatus(t, c)
	glider := findRatingCur(result, "GLIDER")
	ul := findRatingCur(result, "ULTRALIGHT")
	if glider == nil || ul == nil {
		t.Fatalf("ratings not found: glider=%v ul=%v", glider != nil, ul != nil)
	}
	gp, _ := glider["progress"].(map[string]interface{})
	up, _ := ul["progress"].(map[string]interface{})
	assertInt(t, "glider flights", gi(gp, "flights"), 15)
	assertInt(t, "ul flights", gi(up, "flights"), 12)
	assertStr(t, "glider status", glider["status"], "current")
	assertStr(t, "ul status", ul["status"], "current")
}

// createGliderFlightCur logs one 10-minute GLIDER flight with the given launch method.
func createGliderFlightCur(t *testing.T, c *E2EClient, reg, method string, daysAgo int, dual bool) {
	t.Helper()
	f := map[string]interface{}{
		"date": pastDate(daysAgo), "aircraftReg": reg, "aircraftType": "ASK21",
		"departureIcao": "EDNY", "arrivalIcao": "EDNY",
		"offBlockTime": "14:00", "onBlockTime": "14:10",
		"landings": 1, "launchMethod": method,
	}
	if dual {
		f["crewMembers"] = []map[string]interface{}{{"name": "FI", "role": "Instructor"}}
	}
	createFlightCur(t, c, f)
}

// TestEASA_Glider_TrainingFlightsCountedAsFlights — SFCL.160(a)(1)(ii): two short dual circuits are two training flights.
func TestEASA_Glider_TrainingFlightsCountedAsFlights(t *testing.T) {
	c := setupCurrencyUser(t, "glider-train-count")
	createAircraftCur(t, c, "D-5818", "ASK21", "GLIDER")
	licID := createLicenseCur(t, c, "EASA", "SPL")
	createRatingCur(t, c, licID, "GLIDER", nil)

	for i := 0; i < 2; i++ {
		createGliderFlightCur(t, c, "D-5818", "winch", 5+i, true)
	}

	result := getCurrencyStatus(t, c)
	rc := findRatingCur(result, "GLIDER")
	if rc == nil {
		t.Fatal("GLIDER rating currency not found")
	}
	r := getReq(rc, "requirement.training_flights")
	if r == nil || !gb(r, "met") || gi(r, "current") != 2 {
		t.Errorf("training_flights = %v, want 2 met", r)
	}
	if r := getReq(rc, "requirement.flight_time"); r == nil || gi(r, "current") != 20 {
		t.Errorf("flight_time = %v, want 20 minutes of dual counted", r)
	}
}

// TestEASA_Glider_LaunchMethodRecency — SFCL.155(c): lapsed methods stay listed, bungee needs 2, TMG take-offs count toward self-launch.
func TestEASA_Glider_LaunchMethodRecency(t *testing.T) {
	c := setupCurrencyUser(t, "glider-launch-recency")
	createAircraftCur(t, c, "D-5819", "ASK21", "GLIDER")
	createAircraftCur(t, c, "D-KSLG", "SF25", "TMG")
	licID := createLicenseCur(t, c, "EASA", "SPL")
	createRatingCur(t, c, licID, "GLIDER", nil)

	for i := 0; i < 5; i++ {
		createGliderFlightCur(t, c, "D-5819", "winch", 10+i, false)
	}
	createGliderFlightCur(t, c, "D-5819", "aerotow", 800, false)
	for i := 0; i < 2; i++ {
		createGliderFlightCur(t, c, "D-5819", "bungee", 20+i, false)
	}
	createGliderFlightCur(t, c, "D-5819", "car", 30, false)
	for i := 0; i < 2; i++ {
		createGliderFlightCur(t, c, "D-5819", "self-launch", 40+i, false)
	}
	for i := 0; i < 3; i++ {
		createFlightCur(t, c, map[string]interface{}{
			"date": pastDate(50 + i), "aircraftReg": "D-KSLG", "aircraftType": "SF25",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00",
			"landings": 1,
		})
	}

	result := getCurrencyStatus(t, c)
	rc := findRatingCur(result, "GLIDER")
	if rc == nil {
		t.Fatal("GLIDER rating currency not found")
	}
	methods, _ := rc["launchMethodCurrency"].([]interface{})
	want := []struct {
		method             string
		launches, required int
		met                bool
	}{
		{"winch", 5, 5, true},
		{"car", 1, 5, false},
		{"aerotow", 0, 5, false},
		{"self-launch", 5, 5, true},
		{"bungee", 2, 2, true},
	}
	if len(methods) != len(want) {
		t.Fatalf("launchMethodCurrency = %v, want %d methods", methods, len(want))
	}
	for i, w := range want {
		m, _ := methods[i].(map[string]interface{})
		assertStr(t, "method", m["method"], w.method)
		assertInt(t, w.method+" launches", gi(m, "launches"), w.launches)
		assertInt(t, w.method+" required", gi(m, "required"), w.required)
		assertBool(t, w.method+" met", gb(m, "met"), w.met)
	}
}

// TestEASA_Glider_PassengerCurrencyPICOnly — SFCL.160(e)(1): dual launches do not count toward passenger recency.
func TestEASA_Glider_PassengerCurrencyPICOnly(t *testing.T) {
	c := setupCurrencyUser(t, "glider-pax-pic")
	createAircraftCur(t, c, "D-5820", "ASK21", "GLIDER")
	licID := createLicenseCur(t, c, "EASA", "SPL")
	createRatingCur(t, c, licID, "GLIDER", nil)

	for i := 0; i < 3; i++ {
		createGliderFlightCur(t, c, "D-5820", "winch", 5+i, true)
	}
	createGliderFlightCur(t, c, "D-5820", "winch", 9, false)

	result := getCurrencyStatus(t, c)
	pc := findPaxCur(result, "GLIDER")
	if pc == nil {
		t.Fatal("GLIDER passenger currency not found")
	}
	assertStr(t, "ruleDescriptionKey", pc["ruleDescriptionKey"], "easa_spl_pax")
	assertInt(t, "dayLandings", gi(pc, "dayLandings"), 1)
	assertStr(t, "dayStatus", pc["dayStatus"], "expired")
}

// TestTowedFlights_CarAndBungeeExcludedFromPoweredClass — car and bungee launches are towed like winch and aerotow.
func TestTowedFlights_CarAndBungeeExcludedFromPoweredClass(t *testing.T) {
	c := setupCurrencyUser(t, "towed-car-bungee")
	createAircraftCur(t, c, "D-0CAR", "ASK21", "SEP_LAND")
	licID := createLicenseCur(t, c, "EASA", "PPL")
	createRatingCur(t, c, licID, "SEP_LAND", strPtr(plusDays(pastDate(0), 180)))

	createGliderFlightCur(t, c, "D-0CAR", "car", 5, false)
	createGliderFlightCur(t, c, "D-0CAR", "bungee", 6, false)

	result := getCurrencyStatus(t, c)
	rc := findRatingCur(result, "SEP_LAND")
	if rc == nil {
		t.Fatal("SEP_LAND rating currency not found")
	}
	progress, _ := rc["progress"].(map[string]interface{})
	assertInt(t, "progress.flights", gi(progress, "flights"), 0)
}

// ─── FAA glider rating — §61.56 / §61.57(a) ─────────────────────────────────

// TestFAA_GliderRating_PaxRuleDoesNotExpireRating — a glider rating is current on its flight review with no landings in 90 days.
func TestFAA_GliderRating_PaxRuleDoesNotExpireRating(t *testing.T) {
	c := setupCurrencyUser(t, "faa-glider-fr")
	createAircraftCur(t, c, "N5821G", "ASK21", "GLIDER")
	licID := createLicenseCur(t, c, "FAA", "Private")
	createRatingCur(t, c, licID, "GLIDER", nil)

	createFlightCur(t, c, map[string]interface{}{
		"date": pastDate(200), "aircraftReg": "N5821G", "aircraftType": "ASK21",
		"departureIcao": "KFFZ", "arrivalIcao": "KFFZ",
		"offBlockTime": "10:00", "onBlockTime": "10:30",
		"landings": 1, "launchMethod": "aerotow", "isFlightReview": true,
	})

	result := getCurrencyStatus(t, c)
	rc := findRatingCurByAuth(result, "GLIDER", "FAA")
	if rc == nil {
		t.Fatal("FAA GLIDER rating currency not found")
	}
	assertStr(t, "status", rc["status"], "current")
	assertStr(t, "messageKey", rc["messageKey"], "flight_review.current")
	assertStr(t, "ruleDescriptionKey", rc["ruleDescriptionKey"], "faa_flight_review")
	if r := getReq(rc, "requirement.flight_review"); r == nil || !gb(r, "met") {
		t.Errorf("flight_review requirement = %v, want met", r)
	}
	if r := getReq(rc, "requirement.launches_and_landings"); r != nil {
		t.Errorf("rating carries the passenger requirement %v", r)
	}

	pc := findPaxCurByAuth(result, "GLIDER", "FAA")
	if pc == nil {
		t.Fatal("FAA GLIDER passenger currency not found")
	}
	assertStr(t, "pax dayStatus", pc["dayStatus"], "expired")
	assertStr(t, "pax ruleDescriptionKey", pc["ruleDescriptionKey"], "faa_glider")
}

// TestFAA_Glider_FlightReviewGliderAlternative — §61.56(b): three instructional glider flights stand in for the review; dual launches are not passenger landings.
func TestFAA_Glider_FlightReviewGliderAlternative(t *testing.T) {
	c := setupCurrencyUser(t, "faa-glider-alt")
	createAircraftCur(t, c, "N5822G", "ASK21", "GLIDER")
	createAircraftCur(t, c, "N5822C", "C172", "SEP_LAND")
	licID := createLicenseCur(t, c, "FAA", "Private")
	createRatingCur(t, c, licID, "GLIDER", nil)

	for i := 0; i < 2; i++ {
		createGliderFlightCur(t, c, "N5822G", "aerotow", 10+i, true)
	}
	createFlightCur(t, c, map[string]interface{}{
		"date": pastDate(15), "aircraftReg": "N5822C", "aircraftType": "C172",
		"departureIcao": "KFFZ", "arrivalIcao": "KFFZ",
		"offBlockTime": "08:00", "onBlockTime": "09:00",
		"landings":    1,
		"crewMembers": []map[string]interface{}{{"name": "CFI", "role": "Instructor"}},
	})

	result := getCurrencyStatus(t, c)
	rc := findRatingCurByAuth(result, "GLIDER", "FAA")
	if rc == nil {
		t.Fatal("FAA GLIDER rating currency not found")
	}
	assertStr(t, "status with two glider + one aeroplane lesson", rc["status"], "expired")

	createGliderFlightCur(t, c, "N5822G", "winch", 20, true)

	result = getCurrencyStatus(t, c)
	rc = findRatingCurByAuth(result, "GLIDER", "FAA")
	assertStr(t, "status", rc["status"], "current")
	assertStr(t, "messageKey", rc["messageKey"], "rating.flight_review_glider_alternative")
	if r := getReq(rc, "requirement.training_flights"); r == nil || !gb(r, "met") || gi(r, "current") != 3 {
		t.Errorf("training_flights = %v, want 3 met", r)
	}
	if r := getReq(rc, "requirement.flight_review"); r == nil || gb(r, "met") {
		t.Errorf("flight_review = %v, want not met", r)
	}

	pc := findPaxCurByAuth(result, "GLIDER", "FAA")
	if pc == nil {
		t.Fatal("FAA GLIDER passenger currency not found")
	}
	assertInt(t, "pax dayLandings", gi(pc, "dayLandings"), 0)
	assertStr(t, "pax dayStatus", pc["dayStatus"], "expired")
}

// TestFAA_PrivateGliderRating_NoNightRequirement — a glider rating on an FAA Private licence has no night passenger requirement; its SEP rating keeps one.
func TestFAA_PrivateGliderRating_NoNightRequirement(t *testing.T) {
	c := setupCurrencyUser(t, "faa-glider-night")
	createAircraftCur(t, c, "N5823G", "ASK21", "GLIDER")
	licID := createLicenseCur(t, c, "FAA", "Private")
	createRatingCur(t, c, licID, "SEP_LAND", nil)
	createRatingCur(t, c, licID, "GLIDER", nil)

	for i := 0; i < 3; i++ {
		createGliderFlightCur(t, c, "N5823G", "aerotow", 5+i, false)
	}

	result := getCurrencyStatus(t, c)
	pc := findPaxCurByAuth(result, "GLIDER", "FAA")
	if pc == nil {
		t.Fatal("FAA GLIDER passenger currency not found")
	}
	assertBool(t, "glider nightPrivilege", gb(pc, "nightPrivilege"), false)
	assertInt(t, "glider nightRequired", gi(pc, "nightRequired"), 0)
	assertStr(t, "glider nightStatus", pc["nightStatus"], "unknown")
	assertStr(t, "glider dayStatus", pc["dayStatus"], "current")
	assertStr(t, "glider messageKey", pc["messageKey"], "pax.current_day_no_night_privilege")

	sep := findPaxCurByAuth(result, "SEP_LAND", "FAA")
	if sep == nil {
		t.Fatal("FAA SEP_LAND passenger currency not found")
	}
	assertBool(t, "SEP nightPrivilege", gb(sep, "nightPrivilege"), true)
	assertInt(t, "SEP nightRequired", gi(sep, "nightRequired"), 3)
}
