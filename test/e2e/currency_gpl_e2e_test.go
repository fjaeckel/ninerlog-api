//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// ─── GPL gyroplane recency, aircraft MTOM and the SFCL.160(c) exemption ─────

func minuteFlight(t *testing.T, c *E2EClient, reg, acType string, daysAgo int, off, on string, extra map[string]interface{}) {
	t.Helper()
	f := map[string]interface{}{
		"date": pastDate(daysAgo), "aircraftReg": reg, "aircraftType": acType,
		"departureIcao": "EDNY", "arrivalIcao": "EDNY",
		"offBlockTime": off, "onBlockTime": on,
		"landings": 1,
	}
	for k, v := range extra {
		f[k] = v
	}
	createFlightCur(t, c, f)
}

var withInstructor = []map[string]interface{}{{"name": "FI", "role": "Instructor"}}

// TestSFCL160c_PartFCLTMGExemptsSPLTMG — an SPL TMG rating needs no SFCL.160(b)
// experience when the pilot holds Part-FCL TMG privileges.
func TestSFCL160c_PartFCLTMGExemptsSPLTMG(t *testing.T) {
	c := setupCurrencyUser(t, "sfcl-c")
	splID := createLicenseCur(t, c, "EASA", "SPL")
	splTMG := createRatingCur(t, c, splID, "TMG", nil)

	rc := ratingByID(getCurrencyStatus(t, c), splTMG)
	assertStr(t, "status without Part-FCL TMG", rc["status"], "expiring")

	pplID := createLicenseCur(t, c, "EASA", "PPL")
	createRatingCur(t, c, pplID, "TMG", strPtr(plusDays(pastDate(0), 200)))

	rc = ratingByID(getCurrencyStatus(t, c), splTMG)
	assertStr(t, "status", rc["status"], "current")
	assertStr(t, "messageKey", rc["messageKey"], "rating.sfcl_tmg_exempt")
}

// TestGPL_GyroplaneRecencyWithAnnexICredit — FCL.240.G, with UL gyroplanes of
// at least 450 kg credited toward time and landings (FCL.035(a)(5)).
func TestGPL_GyroplaneRecencyWithAnnexICredit(t *testing.T) {
	c := setupCurrencyUser(t, "gpl")
	createAircraftCur(t, c, "D-EGPL", "CAVA", "GYROPLANE")
	resp := c.POST("/aircraft", map[string]interface{}{
		"registration": "D-MGPL", "type": "MTO", "make": "AutoGyro", "model": "MTOsport",
		"aircraftClass": "ULTRALIGHT", "ulKind": "GYROPLANE", "maxTakeoffMassKg": 472,
	})
	requireStatus(t, resp, http.StatusCreated)
	var ul map[string]interface{}
	resp.JSON(&ul)
	if gf(ul, "maxTakeoffMassKg") != 472 {
		t.Fatalf("maxTakeoffMassKg = %v, want 472", ul["maxTakeoffMassKg"])
	}

	licID := createLicenseCur(t, c, "EASA", "GPL")
	ratingID := createRatingCur(t, c, licID, "GYROPLANE", nil)

	minuteFlight(t, c, "D-EGPL", "CAVA", 5, "09:00", "10:00", map[string]interface{}{"crewMembers": withInstructor})
	minuteFlight(t, c, "D-EGPL", "CAVA", 6, "09:00", "10:00", nil)
	for i := 0; i < 10; i++ {
		minuteFlight(t, c, "D-MGPL", "MTO", 10+i, "09:00", "10:00", map[string]interface{}{"crewMembers": withInstructor})
	}

	result := getCurrencyStatus(t, c)
	rc := ratingByID(result, ratingID)
	if rc == nil {
		t.Fatal("GYROPLANE rating currency not found")
	}
	assertStr(t, "ruleDescriptionKey", rc["ruleDescriptionKey"], "easa_gpl")
	if got := stringList(rc["creditedUltralightKinds"]); len(got) != 1 || got[0] != "GYROPLANE" {
		t.Errorf("creditedUltralightKinds = %v, want [GYROPLANE]", got)
	}
	if req := getReq(rc, "requirement.refresher_training"); req == nil || gf(req, "current") != 60 {
		t.Errorf("refresher = %v, want 60 (UL dual excluded)", req)
	}
	assertStr(t, "status", rc["status"], "current")

	pc := findPaxCurByAuth(result, "GYROPLANE", "EASA")
	if pc == nil {
		t.Fatal("GYROPLANE passenger currency not found")
	}
	assertStr(t, "messageKey", pc["messageKey"], "pax.gpl_experience_not_met")
	assertBool(t, "nightPrivilege", gb(pc, "nightPrivilege"), false)

	requireStatus(t, c.PATCH("/aircraft/"+ul["id"].(string), map[string]interface{}{"maxTakeoffMassKg": 400}), http.StatusOK)
	rc = ratingByID(getCurrencyStatus(t, c), ratingID)
	assertStr(t, "status under 450 kg", rc["status"], "expiring")
}

func TestAircraft_MaxTakeoffMass(t *testing.T) {
	c := setupCurrencyUser(t, "mtom")
	resp := c.POST("/aircraft", map[string]interface{}{
		"registration": "D-MBAD", "type": "MTO", "make": "AutoGyro", "model": "MTOsport", "maxTakeoffMassKg": 0,
	})
	requireStatus(t, resp, http.StatusBadRequest)
	if !strings.Contains(string(resp.Body), "Invalid maximum take-off mass") {
		t.Errorf("body = %s", resp.Body)
	}

	resp = c.POST("/aircraft", map[string]interface{}{
		"registration": "D-MOKK", "type": "MTO", "make": "AutoGyro", "model": "MTOsport", "maxTakeoffMassKg": 560,
	})
	requireStatus(t, resp, http.StatusCreated)
	var ac map[string]interface{}
	resp.JSON(&ac)
	resp = c.PATCH(fmt.Sprintf("/aircraft/%s", ac["id"]), map[string]interface{}{"maxTakeoffMassKg": nil})
	requireStatus(t, resp, http.StatusOK)
	var got map[string]interface{}
	resp.JSON(&got)
	if _, ok := got["maxTakeoffMassKg"]; ok {
		t.Errorf("maxTakeoffMassKg = %v, want cleared", got["maxTakeoffMassKg"])
	}
}
