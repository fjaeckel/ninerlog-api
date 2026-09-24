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

// TestEASA_GliderClass_Current — GLIDER rating counts flights on GLIDER aircraft under FCL.140.S.
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

	pc := findPaxCur(result, "GLIDER")
	if pc == nil {
		t.Fatal("GLIDER passenger currency not found")
	}
	assertBool(t, "nightPrivilege", gb(pc, "nightPrivilege"), false)
}

// TestEASA_GliderClass_NonSPLLicense — GLIDER rating on a non-SPL license still uses FCL.140.S.
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
