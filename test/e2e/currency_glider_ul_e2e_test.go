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

// TestEASA_UltralightClass_Current — ULTRALIGHT rating under EASA uses LuftPersV §45.
func TestEASA_UltralightClass_Current(t *testing.T) {
	c := setupCurrencyUser(t, "easa-ul-cls")
	createAircraftCur(t, c, "D-MULC", "C42", "ULTRALIGHT")

	licID := createLicenseCur(t, c, "EASA", "UL")
	createRatingCur(t, c, licID, "ULTRALIGHT", nil)

	createULFlightsCur(t, c, "D-MULC")

	result := getCurrencyStatus(t, c)
	rc := findRatingCur(result, "ULTRALIGHT")
	if rc == nil {
		t.Fatal("ULTRALIGHT rating currency not found")
	}
	assertStr(t, "status", rc["status"], "current")
	assertStr(t, "ruleDescriptionKey", rc["ruleDescriptionKey"], "ul_luftpersv")

	pc := findPaxCur(result, "ULTRALIGHT")
	if pc == nil {
		t.Fatal("ULTRALIGHT passenger currency not found")
	}
	assertStr(t, "ruleDescriptionKey", pc["ruleDescriptionKey"], "ul_pax")
	assertBool(t, "nightPrivilege", gb(pc, "nightPrivilege"), false)
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
