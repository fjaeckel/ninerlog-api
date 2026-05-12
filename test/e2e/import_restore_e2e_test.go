//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
)

// TestJSONExportImportRoundTrip exercises the full backup-and-restore flow:
//
//  1. User A populates a rich profile (aircraft, licenses + class ratings,
//     credentials, multi-flight history with crew members).
//  2. They download a JSON backup via GET /exports/json.
//  3. A brand new User B (fresh account, no data) uploads the same JSON to
//     POST /imports/json.
//  4. User B's GET endpoints are then asserted to return semantically
//     equivalent data — i.e. the user can take their JSON download and
//     restore it on any NinerLog installation.
//
// This is the canonical "data portability" test for NinerLog: as long as the
// JSON export remains the source of truth, a user is never locked in.
func TestJSONExportImportRoundTrip(t *testing.T) {
	source := NewE2EClient(t)
	dest := NewE2EClient(t)

	registerAndLogin(t, source, uniqueEmail("backup-src"), "SecurePass123!", "Source")
	registerAndLogin(t, dest, uniqueEmail("backup-dst"), "SecurePass123!", "Destination")

	// ----------------------------------------------------------------------
	// 1. Build a complex configuration on the source account.
	// ----------------------------------------------------------------------

	// Aircraft fleet — three different classes including a complex/HP twin.
	aircraftFleet := []map[string]interface{}{
		{
			"registration": "D-EBKP", "type": "C172", "make": "Cessna", "model": "172S",
			"aircraftClass": "SEP_LAND",
		},
		{
			"registration": "D-MBKP", "type": "PA44", "make": "Piper", "model": "Seminole",
			"aircraftClass": "MEP_LAND", "isComplex": true, "isHighPerformance": true,
			"notes": "Club twin",
		},
		{
			"registration": "N747BK", "type": "B738", "make": "Boeing", "model": "737-800",
			"aircraftClass": "MEP_LAND",
		},
	}
	for _, ac := range aircraftFleet {
		r := source.POST("/aircraft", ac)
		requireStatus(t, r, http.StatusCreated)
	}

	// Licenses + class ratings — two licenses, several ratings.
	pplResp := source.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "PPL",
		"licenseNumber":       "DE-PPL-BKP-001",
		"issueDate":           "2022-04-01",
		"issuingAuthority":    "LBA",
	})
	requireStatus(t, pplResp, http.StatusCreated)
	var pplLic map[string]interface{}
	pplResp.JSON(&pplLic)
	pplID := pplLic["id"].(string)

	atplResp := source.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority":     "FAA",
		"licenseType":             "ATP",
		"licenseNumber":           "US-ATP-BKP-001",
		"issueDate":               "2024-09-15",
		"issuingAuthority":        "FAA",
		"requiresSeparateLogbook": true,
	})
	requireStatus(t, atplResp, http.StatusCreated)

	ratings := []map[string]interface{}{
		{"classType": "SEP_LAND", "issueDate": "2022-04-01"},
		{"classType": "MEP_LAND", "issueDate": "2023-06-01", "expiryDate": futureDate(365), "notes": "Initial MEP"},
		{"classType": "IR", "issueDate": "2024-01-10", "expiryDate": futureDate(365)},
	}
	for _, cr := range ratings {
		requireStatus(t, source.POST(fmt.Sprintf("/licenses/%s/ratings", pplID), cr), http.StatusCreated)
	}

	// Credentials — medical, language, security clearance.
	creds := []map[string]interface{}{
		{
			"credentialType": "EASA_CLASS1_MEDICAL", "credentialNumber": "MED-BKP-001",
			"issueDate": "2024-02-01", "expiryDate": futureDate(365),
			"issuingAuthority": "AME Schmidt", "notes": "Class 1 with audio waiver",
		},
		{
			"credentialType": "LANG_ICAO_LEVEL5", "issueDate": "2023-07-15",
			"expiryDate": futureDate(1825), "issuingAuthority": "LBA",
		},
		{
			"credentialType": "SEC_CLEARANCE_ZUP", "credentialNumber": "ZUP-BKP-77",
			"issueDate": "2024-05-10", "issuingAuthority": "LBA",
		},
	}
	for _, cred := range creds {
		requireStatus(t, source.POST("/credentials", cred), http.StatusCreated)
	}

	// Flights — a varied logbook: solo training, dual instruction (with crew),
	// IFR cross-country, and an airline leg.
	flights := []map[string]interface{}{
		{
			"date": "2024-06-01", "aircraftReg": "D-EBKP", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 3,
			"remarks": "Pattern work — solo",
		},
		{
			"date": "2024-06-15", "aircraftReg": "D-MBKP", "aircraftType": "PA44",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "10:00", "onBlockTime": "11:30", "landings": 1,
			"remarks":  "MEP dual XC",
			"isDual":   true,
			"crewMembers": []map[string]interface{}{
				{"name": "Hans Müller", "role": "Instructor"},
			},
		},
		{
			"date": "2024-07-02", "aircraftReg": "D-MBKP", "aircraftType": "PA44",
			"departureIcao": "EDDS", "arrivalIcao": "LSZH",
			"offBlockTime": "13:15", "onBlockTime": "15:00", "landings": 1,
			"ifrTime": 90,
			"remarks": "MEP IFR XC",
		},
		{
			"date": "2024-08-20", "aircraftReg": "N747BK", "aircraftType": "B738",
			"departureIcao": "EDDF", "arrivalIcao": "KJFK",
			"offBlockTime": "12:00", "onBlockTime": "20:30", "landings": 1,
			"remarks": "Airline leg",
			"crewMembers": []map[string]interface{}{
				{"name": "Captain Ada Lovelace", "role": "PIC"},
				{"name": "FO Grace Hopper", "role": "SIC"},
			},
		},
	}
	for _, fl := range flights {
		r := source.POST("/flights", fl)
		requireStatus(t, r, http.StatusCreated)
	}

	// ----------------------------------------------------------------------
	// 2. Download the JSON backup.
	// ----------------------------------------------------------------------
	backupResp := source.GET("/exports/json")
	requireStatus(t, backupResp, http.StatusOK)

	if !strings.Contains(string(backupResp.Body), "NinerLog JSON Backup") {
		t.Fatal("backup is missing the NinerLog JSON Backup format marker")
	}

	// Sanity-check: it deserialises into a backup envelope.
	var backupShape map[string]interface{}
	if err := json.Unmarshal(backupResp.Body, &backupShape); err != nil {
		t.Fatalf("backup is not valid JSON: %v", err)
	}
	if backupShape["format"] != "NinerLog JSON Backup" {
		t.Fatalf("unexpected backup format: %v", backupShape["format"])
	}

	// ----------------------------------------------------------------------
	// 3. Restore into a fresh account via POST /imports/json.
	// ----------------------------------------------------------------------
	restoreResp := dest.Do("POST", "/imports/json", backupShape)
	requireStatus(t, restoreResp, http.StatusOK)

	var summary struct {
		AircraftImported     int `json:"aircraftImported"`
		AircraftSkipped      int `json:"aircraftSkipped"`
		LicensesImported     int `json:"licensesImported"`
		ClassRatingsImported int `json:"classRatingsImported"`
		CredentialsImported  int `json:"credentialsImported"`
		FlightsImported      int `json:"flightsImported"`
		CrewMembersImported  int `json:"crewMembersImported"`
	}
	if err := restoreResp.JSON(&summary); err != nil {
		t.Fatalf("invalid summary response: %v — body=%s", err, string(restoreResp.Body))
	}

	assertInt(t, "aircraftImported", summary.AircraftImported, len(aircraftFleet))
	assertInt(t, "aircraftSkipped", summary.AircraftSkipped, 0)
	assertInt(t, "licensesImported", summary.LicensesImported, 2)
	assertInt(t, "classRatingsImported", summary.ClassRatingsImported, len(ratings))
	assertInt(t, "credentialsImported", summary.CredentialsImported, len(creds))
	assertInt(t, "flightsImported", summary.FlightsImported, len(flights))
	// Two flights have crew: 1 instructor + (1 PIC + 1 SIC) = 3 members.
	assertInt(t, "crewMembersImported", summary.CrewMembersImported, 3)

	// ----------------------------------------------------------------------
	// 4. Verify the destination account now mirrors the source.
	// ----------------------------------------------------------------------

	// Aircraft — compare the set of registrations.
	{
		r := dest.GET("/aircraft")
		requireStatus(t, r, http.StatusOK)
		var page struct {
			Data []map[string]interface{} `json:"data"`
		}
		r.JSON(&page)

		gotRegs := registrations(page.Data)
		wantRegs := []string{"D-EBKP", "D-MBKP", "N747BK"}
		sort.Strings(gotRegs)
		sort.Strings(wantRegs)
		if !equalStrSlices(gotRegs, wantRegs) {
			t.Errorf("aircraft regs: want %v, got %v", wantRegs, gotRegs)
		}
		// Spot-check that complex/HP flags survived the round-trip.
		for _, ac := range page.Data {
			if ac["registration"] == "D-MBKP" {
				assertBool(t, "D-MBKP isComplex", gb(ac, "isComplex"), true)
				assertBool(t, "D-MBKP isHighPerformance", gb(ac, "isHighPerformance"), true)
			}
		}
	}

	// Licenses + class ratings.
	var destPPLID string
	{
		r := dest.GET("/licenses")
		requireStatus(t, r, http.StatusOK)
		var lics []map[string]interface{}
		r.JSON(&lics)
		if len(lics) != 2 {
			t.Fatalf("expected 2 licenses, got %d", len(lics))
		}
		for _, lic := range lics {
			if lic["licenseNumber"] == "DE-PPL-BKP-001" {
				destPPLID = lic["id"].(string)
				assertStr(t, "PPL authority", lic["regulatoryAuthority"], "EASA")
			}
			if lic["licenseNumber"] == "US-ATP-BKP-001" {
				assertBool(t, "ATP separate logbook", gb(lic, "requiresSeparateLogbook"), true)
			}
		}
		if destPPLID == "" {
			t.Fatal("imported PPL license not found on destination")
		}
	}
	{
		r := dest.GET(fmt.Sprintf("/licenses/%s/ratings", destPPLID))
		requireStatus(t, r, http.StatusOK)
		var got []map[string]interface{}
		r.JSON(&got)
		if len(got) != len(ratings) {
			t.Fatalf("expected %d class ratings on PPL, got %d", len(ratings), len(got))
		}
		gotTypes := map[string]bool{}
		for _, cr := range got {
			gotTypes[cr["classType"].(string)] = true
		}
		for _, want := range []string{"SEP_LAND", "MEP_LAND", "IR"} {
			if !gotTypes[want] {
				t.Errorf("class rating %s missing after restore", want)
			}
		}
	}

	// Credentials.
	{
		r := dest.GET("/credentials")
		requireStatus(t, r, http.StatusOK)
		var got []map[string]interface{}
		r.JSON(&got)
		if len(got) != len(creds) {
			t.Fatalf("expected %d credentials, got %d", len(creds), len(got))
		}
		gotTypes := map[string]bool{}
		for _, cr := range got {
			gotTypes[cr["credentialType"].(string)] = true
		}
		for _, want := range []string{"EASA_CLASS1_MEDICAL", "LANG_ICAO_LEVEL5", "SEC_CLEARANCE_ZUP"} {
			if !gotTypes[want] {
				t.Errorf("credential %s missing after restore", want)
			}
		}
	}

	// Flights + crew — re-export the destination account and verify the
	// round-tripped backup contains the same flights, in the same shape,
	// with the same crew members. This doubles as a stronger round-trip
	// assertion: export → import → export must produce equivalent data.
	{
		r := dest.GET("/exports/json")
		requireStatus(t, r, http.StatusOK)
		var restored map[string]interface{}
		if err := json.Unmarshal(r.Body, &restored); err != nil {
			t.Fatalf("destination backup is not valid JSON: %v", err)
		}

		restoredFlights, _ := restored["flights"].([]interface{})
		if len(restoredFlights) != len(flights) {
			t.Fatalf("expected %d flights in destination backup, got %d", len(flights), len(restoredFlights))
		}

		// Index flights by date for easy lookup.
		byDate := map[string]map[string]interface{}{}
		for _, fv := range restoredFlights {
			fl := fv.(map[string]interface{})
			d, _ := fl["date"].(string)
			if len(d) >= 10 {
				byDate[d[:10]] = fl
			}
		}
		for _, want := range []string{"2024-06-01", "2024-06-15", "2024-07-02", "2024-08-20"} {
			if _, ok := byDate[want]; !ok {
				t.Errorf("flight dated %s missing in restored backup", want)
			}
		}

		// Airline leg: 2 crew with PIC/SIC roles preserved.
		airlineLeg, ok := byDate["2024-08-20"]
		if !ok {
			t.Fatal("airline leg missing from restored backup")
		}
		crewVal, _ := airlineLeg["crewMembers"].([]interface{})
		if len(crewVal) != 2 {
			t.Errorf("airline leg crew: want 2, got %d (%v)", len(crewVal), crewVal)
		}
		roles := map[string]string{}
		for _, m := range crewVal {
			mm := m.(map[string]interface{})
			roles[mm["role"].(string)] = mm["name"].(string)
		}
		if roles["PIC"] != "Captain Ada Lovelace" {
			t.Errorf("airline leg PIC: want Captain Ada Lovelace, got %q", roles["PIC"])
		}
		if roles["SIC"] != "FO Grace Hopper" {
			t.Errorf("airline leg SIC: want FO Grace Hopper, got %q", roles["SIC"])
		}

		// Dual MEP leg: instructor preserved.
		dualLeg, ok := byDate["2024-06-15"]
		if !ok {
			t.Fatal("dual MEP leg missing from restored backup")
		}
		dualCrew, _ := dualLeg["crewMembers"].([]interface{})
		if len(dualCrew) != 1 {
			t.Fatalf("dual MEP leg crew: want 1, got %d", len(dualCrew))
		}
		mm := dualCrew[0].(map[string]interface{})
		assertStr(t, "MEP instructor name", mm["name"], "Hans Müller")
		assertStr(t, "MEP instructor role", mm["role"], "Instructor")
	}

	// ----------------------------------------------------------------------
	// 5. Re-running the import should be additive: existing aircraft regs are
	//    skipped, but the rest is re-imported (no UNIQUE constraints on
	//    licenses/credentials/flights). This proves the endpoint is safely
	//    re-runnable without crashing on duplicate aircraft.
	// ----------------------------------------------------------------------
	secondResp := dest.Do("POST", "/imports/json", backupShape)
	requireStatus(t, secondResp, http.StatusOK)
	var second struct {
		AircraftImported int `json:"aircraftImported"`
		AircraftSkipped  int `json:"aircraftSkipped"`
		FlightsImported  int `json:"flightsImported"`
	}
	secondResp.JSON(&second)
	assertInt(t, "second-run aircraftImported", second.AircraftImported, 0)
	assertInt(t, "second-run aircraftSkipped", second.AircraftSkipped, len(aircraftFleet))
	assertInt(t, "second-run flightsImported", second.FlightsImported, len(flights))
}

// TestJSONImportRejectsForeignBackup ensures the import endpoint refuses input
// that wasn't produced by NinerLog's exporter — protecting against accidental
// imports of unrelated JSON files.
func TestJSONImportRejectsForeignBackup(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("bkp-foreign"), "SecurePass123!", "F")

	r := c.POST("/imports/json", map[string]interface{}{
		"format":  "Some Other Tool",
		"flights": []interface{}{},
	})
	assertStatus(t, r, http.StatusBadRequest)
}

// TestJSONImportRequiresAuth confirms the endpoint enforces bearer auth.
func TestJSONImportRequiresAuth(t *testing.T) {
	c := NewE2EClient(t)
	r := c.POST("/imports/json", map[string]interface{}{"format": "NinerLog JSON Backup"})
	assertStatus(t, r, http.StatusUnauthorized)
}

// ---------- helpers ----------

func registrations(items []map[string]interface{}) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		if reg, ok := it["registration"].(string); ok {
			out = append(out, reg)
		}
	}
	return out
}

func equalStrSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
