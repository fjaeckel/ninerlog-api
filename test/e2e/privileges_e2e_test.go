//go:build e2e

package e2e_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

type privilegeBody struct {
	ID        string  `json:"id"`
	LicenseID string  `json:"licenseId"`
	Kind      string  `json:"kind"`
	Detail    *string `json:"detail"`
	IssuedOn  *string `json:"issuedOn"`
	ExpiresOn *string `json:"expiresOn"`
	Notes     *string `json:"notes"`
}

func createPrivilege(t *testing.T, c *E2EClient, licID string, body map[string]interface{}) privilegeBody {
	t.Helper()
	resp := c.POST("/licenses/"+licID+"/privileges", body)
	requireStatus(t, resp, http.StatusCreated)
	var p privilegeBody
	if err := resp.JSON(&p); err != nil {
		t.Fatalf("decode privilege: %v", err)
	}
	return p
}

func listPrivileges(t *testing.T, c *E2EClient, licID string) []privilegeBody {
	t.Helper()
	resp := c.GET("/licenses/" + licID + "/privileges")
	requireStatus(t, resp, http.StatusOK)
	var out []privilegeBody
	if err := resp.JSON(&out); err != nil {
		t.Fatalf("decode privileges: %v", err)
	}
	return out
}

// privilegeCurrency returns the currency entry of a privilege.
func privilegeCurrency(t *testing.T, result map[string]interface{}, privilegeID string) map[string]interface{} {
	t.Helper()
	list, _ := result["privileges"].([]interface{})
	for _, p := range list {
		pc := p.(map[string]interface{})
		if pc["privilegeId"] == privilegeID {
			return pc
		}
	}
	t.Fatalf("privilege %s not in currency response %v", privilegeID, result["privileges"])
	return nil
}

// requirementByKey returns the requirement row with nameKey.
func requirementByKey(entry map[string]interface{}, nameKey string) map[string]interface{} {
	reqs, _ := entry["requirements"].([]interface{})
	for _, r := range reqs {
		row := r.(map[string]interface{})
		if row["nameKey"] == nameKey {
			return row
		}
	}
	return nil
}

func TestLicencePrivileges_CRUD(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("privileges"), "SecurePass123!", "Petra")
	ppl := createLicenseCur(t, c, "EASA", "PPL(A)")
	spl := createLicenseCur(t, c, "EASA", "SPL")
	base := "/licenses/" + ppl + "/privileges"

	var towing privilegeBody
	t.Run("P job 3 Petra records sailplane towing on her PPL", func(t *testing.T) {
		towing = createPrivilege(t, c, ppl, map[string]interface{}{
			"kind": "SAILPLANE_TOWING", "issuedOn": "2019-05-01", "notes": "DR400 D-EDRF",
		})
		if towing.LicenseID != ppl || towing.Kind != "SAILPLANE_TOWING" || towing.IssuedOn == nil || *towing.IssuedOn != "2019-05-01" {
			t.Errorf("unexpected privilege: %+v", towing)
		}
	})

	t.Run("validation", func(t *testing.T) {
		cases := []struct {
			name string
			body map[string]interface{}
		}{
			{"unknown kind", map[string]interface{}{"kind": "WINGWALKING"}},
			{"missing kind", map[string]interface{}{"detail": "x"}},
			{"launch method trained without a method", map[string]interface{}{"kind": "LAUNCH_METHOD_TRAINED"}},
			{"launch method trained with an unknown method", map[string]interface{}{"kind": "LAUNCH_METHOD_TRAINED", "detail": "catapult"}},
			{"UL towing with an unknown kind", map[string]interface{}{"kind": "UL_TOWING", "detail": "BALLOON"}},
			{"UL towing without a kind", map[string]interface{}{"kind": "UL_TOWING"}},
			{"Einweisung without a type", map[string]interface{}{"kind": "UL_TYPE_BRIEFING", "detail": "  "}},
			{"detail too long", map[string]interface{}{"kind": "UL_TYPE_BRIEFING", "detail": strings.Repeat("x", 101)}},
			{"notes too long", map[string]interface{}{"kind": "FI_S", "notes": strings.Repeat("x", 1001)}},
			{"expiry before issue", map[string]interface{}{"kind": "FI_S", "issuedOn": "2026-05-01", "expiresOn": "2026-04-30"}},
			{"malformed date", map[string]interface{}{"kind": "FI_S", "expiresOn": "next spring"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				assertStatus(t, c.POST(base, tc.body), http.StatusBadRequest)
			})
		}
		t.Run("launch method and UL kind are normalised", func(t *testing.T) {
			lm := createPrivilege(t, c, spl, map[string]interface{}{"kind": "LAUNCH_METHOD_TRAINED", "detail": " Aerotow "})
			if lm.Detail == nil || *lm.Detail != "aerotow" {
				t.Errorf("detail = %v, want aerotow", lm.Detail)
			}
			ul := createPrivilege(t, c, spl, map[string]interface{}{"kind": "UL_TOWING", "detail": "three_axis"})
			if ul.Detail == nil || *ul.Detail != "THREE_AXIS" {
				t.Errorf("detail = %v, want THREE_AXIS", ul.Detail)
			}
			brief := createPrivilege(t, c, spl, map[string]interface{}{"kind": "UL_TYPE_BRIEFING", "detail": strings.Repeat("x", 100)})
			if brief.Detail == nil || len(*brief.Detail) != 100 {
				t.Errorf("100-character type rejected or altered: %v", brief.Detail)
			}
		})
	})

	t.Run("list is per licence", func(t *testing.T) {
		if got := listPrivileges(t, c, ppl); len(got) != 1 || got[0].ID != towing.ID {
			t.Errorf("PPL privileges = %+v, want the towing privilege only", got)
		}
		if got := listPrivileges(t, c, spl); len(got) != 3 {
			t.Errorf("SPL privileges = %d, want 3", len(got))
		}
	})

	t.Run("patch updates and null clears", func(t *testing.T) {
		resp := c.PATCH(base+"/"+towing.ID, map[string]interface{}{"expiresOn": "2030-01-31"})
		requireStatus(t, resp, http.StatusOK)
		var p privilegeBody
		resp.JSON(&p)
		if p.ExpiresOn == nil || *p.ExpiresOn != "2030-01-31" || p.Notes == nil {
			t.Errorf("after patch: %+v", p)
		}
		resp = c.PATCH(base+"/"+towing.ID, map[string]interface{}{"notes": nil, "issuedOn": nil})
		requireStatus(t, resp, http.StatusOK)
		var cleared privilegeBody
		resp.JSON(&cleared)
		if cleared.Notes != nil || cleared.IssuedOn != nil || cleared.ExpiresOn == nil {
			t.Errorf("after clearing: %+v", cleared)
		}
		assertStatus(t, c.PATCH(base+"/"+towing.ID, map[string]interface{}{"kind": "LAUNCH_METHOD_TRAINED"}), http.StatusBadRequest)
		assertStatus(t, c.PATCH(base+"/"+towing.ID, map[string]interface{}{"issuedOn": "2031-01-01"}), http.StatusBadRequest)
	})

	t.Run("ownership", func(t *testing.T) {
		other := NewE2EClient(t)
		registerAndLogin(t, other, uniqueEmail("privileges-other"), "SecurePass123!", "Other")
		assertStatus(t, other.GET(base), http.StatusNotFound)
		assertStatus(t, other.POST(base, map[string]interface{}{"kind": "CLOUD_FLYING"}), http.StatusNotFound)
		assertStatus(t, other.PATCH(base+"/"+towing.ID, map[string]interface{}{"notes": "mine"}), http.StatusNotFound)
		assertStatus(t, other.DELETE(base+"/"+towing.ID), http.StatusNotFound)
		assertStatus(t, c.PATCH("/licenses/"+spl+"/privileges/"+towing.ID, map[string]interface{}{"notes": "x"}), http.StatusNotFound)
		assertStatus(t, c.DELETE("/licenses/"+spl+"/privileges/"+towing.ID), http.StatusNotFound)
		assertStatus(t, c.GET("/licenses/00000000-0000-0000-0000-000000000001/privileges"), http.StatusNotFound)
		assertStatus(t, c.DELETE(base+"/00000000-0000-0000-0000-000000000001"), http.StatusNotFound)
	})

	t.Run("unauthenticated", func(t *testing.T) {
		assertStatus(t, NewE2EClient(t).GET(base), http.StatusUnauthorized)
	})

	t.Run("delete", func(t *testing.T) {
		requireStatus(t, c.DELETE(base+"/"+towing.ID), http.StatusNoContent)
		assertStatus(t, c.DELETE(base+"/"+towing.ID), http.StatusNotFound)
	})

	t.Run("deleting the licence deletes its privileges", func(t *testing.T) {
		requireStatus(t, c.DELETE("/licenses/"+spl), http.StatusNoContent)
		assertStatus(t, c.GET("/licenses/"+spl+"/privileges"), http.StatusNotFound)
	})
}

func privGliderFlight(date, reg string, extra map[string]interface{}) map[string]interface{} {
	f := map[string]interface{}{
		"date": date, "aircraftReg": reg, "aircraftType": "ASG 29E",
		"departureIcao": "EDNY", "arrivalIcao": "EDNY",
		"departureTime": "12:00", "arrivalTime": "14:00", "landings": 1,
	}
	for k, v := range extra {
		f[k] = v
	}
	return f
}

func TestPrivileges_Currency(t *testing.T) {
	t.Run("P job 3 Petra towing 5 tows current", func(t *testing.T) {
		c := setupCurrencyUser(t, "priv-tow")
		createAircraftCur(t, c, "D-EDRF", "DR400", "SEP_LAND")
		ppl := createLicenseCur(t, c, "EASA", "PPL(A)")
		tow := createPrivilege(t, c, ppl, map[string]interface{}{"kind": "SAILPLANE_TOWING"})
		for i := 0; i < 4; i++ {
			createFlightCur(t, c, map[string]interface{}{
				"date": pastDate(10 + i), "aircraftReg": "D-EDRF", "aircraftType": "DR400",
				"departureIcao": "EDNY", "arrivalIcao": "EDNY",
				"offBlockTime": "09:00", "onBlockTime": "09:15", "landings": 1, "isTowFlight": true,
			})
		}
		pc := privilegeCurrency(t, getCurrencyStatus(t, c), tow.ID)
		assertStr(t, "4 tows status", pc["status"], "lapsed")
		row := requirementByKey(pc, "requirement.tows")
		if row == nil || gi(row, "current") != 4 || row["remedyKey"] != "remedy.privilege_with_instructor" {
			t.Errorf("tows row = %v", row)
		}

		createFlightCur(t, c, map[string]interface{}{
			"date": pastDate(5), "aircraftReg": "D-EDRF", "aircraftType": "DR400",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "09:00", "onBlockTime": "09:15", "landings": 1, "isTowFlight": true,
		})
		pc = privilegeCurrency(t, getCurrencyStatus(t, c), tow.ID)
		assertStr(t, "status", pc["status"], "current")
		assertStr(t, "messageKey", pc["messageKey"], "privilege.recency_current")
		assertStr(t, "ruleDescriptionKey", pc["ruleDescriptionKey"], "sfcl_205_towing")
		row = requirementByKey(pc, "requirement.tows")
		assertStr(t, "validUntil", row["validUntil"], time.Now().AddDate(0, 0, -13).AddDate(2, 0, -1).Format("2006-01-02"))
	})

	t.Run("P3 towing flights never feed the glider rating", func(t *testing.T) {
		c := setupCurrencyUser(t, "priv-tow-p3")
		createAircraftCur(t, c, "D-EDRF", "DR400", "SEP_LAND")
		spl := createLicenseCur(t, c, "EASA", "SPL")
		ratingID := createRatingCur(t, c, spl, "GLIDER", nil)
		for i := 0; i < 5; i++ {
			createFlightCur(t, c, map[string]interface{}{
				"date": pastDate(10 + i), "aircraftReg": "D-EDRF", "aircraftType": "DR400",
				"departureIcao": "EDNY", "arrivalIcao": "EDNY",
				"offBlockTime": "09:00", "onBlockTime": "09:15", "landings": 1, "isTowFlight": true,
			})
		}
		rc := ratingByID(getCurrencyStatus(t, c), ratingID)
		assertStr(t, "glider status", rc["status"], "lapsed")
	})

	t.Run("Petra cloud flying from IFR time on gliders", func(t *testing.T) {
		c := setupCurrencyUser(t, "priv-cloud")
		createAircraftCur(t, c, "D-KXYZ", "ASG 29E", "GLIDER")
		spl := createLicenseCur(t, c, "EASA", "SPL")
		cloud := createPrivilege(t, c, spl, map[string]interface{}{"kind": "CLOUD_FLYING"})
		createFlightCur(t, c, privGliderFlight(pastDate(20), "D-KXYZ", map[string]interface{}{"launchMethod": "self-launch", "picTime": 120, "ifrTime": 35}))
		createFlightCur(t, c, privGliderFlight(pastDate(40), "D-KXYZ", map[string]interface{}{"launchMethod": "self-launch", "picTime": 120, "ifrTime": 30}))
		pc := privilegeCurrency(t, getCurrencyStatus(t, c), cloud.ID)
		assertStr(t, "status", pc["status"], "current")
		assertStr(t, "ruleDescriptionKey", pc["ruleDescriptionKey"], "sfcl_215_cloud_flying")
		assertInt(t, "cloud minutes", gi(requirementByKey(pc, "requirement.cloud_flying_time"), "current"), 65)
		assertInt(t, "cloud flights", gi(requirementByKey(pc, "requirement.cloud_flying_flights"), "current"), 2)
	})

	t.Run("P job 3 Petra FI(S) instruction and refresher", func(t *testing.T) {
		c := setupCurrencyUser(t, "priv-fis")
		createAircraftCur(t, c, "D-1234", "ASK 21", "GLIDER")
		spl := createLicenseCur(t, c, "EASA", "SPL")
		fi := createPrivilege(t, c, spl, map[string]interface{}{"kind": "FI_S", "expiresOn": futureDate(400)})
		createFlightCur(t, c, privGliderFlight(pastDate(30), "D-1234", map[string]interface{}{
			"launchMethod": "winch", "launches": 60, "landings": 60, "dualGivenTime": 120,
		}))
		pc := privilegeCurrency(t, getCurrencyStatus(t, c), fi.ID)
		assertStr(t, "status", pc["status"], "current")
		assertInt(t, "instruction launches", gi(requirementByKey(pc, "requirement.instruction_launches"), "current"), 60)
		if ref := requirementByKey(pc, "requirement.fi_refresher"); ref == nil || ref["messageKey"] != "requirement.untracked" {
			t.Errorf("refresher row = %v", ref)
		}
		params, _ := pc["messageParams"].(map[string]interface{})
		assertStr(t, "expiry date param", params["date"], futureDate(400))

		requireStatus(t, c.PATCH("/licenses/"+spl+"/privileges/"+fi.ID, map[string]interface{}{"expiresOn": pastDate(1)}), http.StatusOK)
		pc = privilegeCurrency(t, getCurrencyStatus(t, c), fi.ID)
		assertStr(t, "expired status", pc["status"], "expired")
		assertStr(t, "expired key", pc["messageKey"], "privilege.expired")
	})

	t.Run("L4 trained launch methods show never-logged methods", func(t *testing.T) {
		c := setupCurrencyUser(t, "priv-launch")
		createAircraftCur(t, c, "D-1234", "ASK 21", "GLIDER")
		spl := createLicenseCur(t, c, "EASA", "SPL")
		ratingID := createRatingCur(t, c, spl, "GLIDER", nil)
		createPrivilege(t, c, spl, map[string]interface{}{"kind": "LAUNCH_METHOD_TRAINED", "detail": "aerotow"})
		createFlightCur(t, c, privGliderFlight(pastDate(10), "D-1234", map[string]interface{}{"launchMethod": "winch", "launches": 6, "landings": 6}))
		rc := ratingByID(getCurrencyStatus(t, c), ratingID)
		methods, _ := rc["launchMethodCurrency"].([]interface{})
		if len(methods) != 2 {
			t.Fatalf("launchMethodCurrency = %v, want winch and aerotow", methods)
		}
		winch := methods[0].(map[string]interface{})
		aerotow := methods[1].(map[string]interface{})
		assertStr(t, "first method", winch["method"], "winch")
		assertBool(t, "winch trained", gb(winch, "trained"), false)
		assertStr(t, "second method", aerotow["method"], "aerotow")
		assertBool(t, "aerotow trained", gb(aerotow, "trained"), true)
		assertInt(t, "aerotow launches", gi(aerotow, "launches"), 0)
		assertStr(t, "aerotow remedy", aerotow["remedyKey"], "remedy.launch_method_dual")
	})

	t.Run("M §45a passengers need authorisation", func(t *testing.T) {
		c := setupCurrencyUser(t, "priv-ulpax")
		createULAircraftCur(t, c, "D-MXYZ", "THREE_AXIS")
		lic := createLicenseCur(t, c, "DULV", "UL")
		createULRatingCur(t, c, lic, "THREE_AXIS")
		createFlightCur(t, c, map[string]interface{}{
			"date": pastDate(10), "aircraftReg": "D-MXYZ", "aircraftType": "C42",
			"departureIcao": "EDNY", "arrivalIcao": "EDNY",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 3,
		})
		createFlightCur(t, c, map[string]interface{}{
			"date": pastDate(400), "aircraftReg": "D-MXYZ", "aircraftType": "C42",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 2, "dualTime": 90,
			"crewMembers": []map[string]interface{}{{"name": "FI", "role": "Instructor"}},
		})

		pc := paxByKind(getCurrencyStatus(t, c))["THREE_AXIS"]
		assertStr(t, "dayStatus", pc["dayStatus"], "unknown")
		assertStr(t, "messageKey", pc["messageKey"], "pax.ul_authorisation_missing")
		assertInt(t, "dayLandings", gi(pc, "dayLandings"), 3)
		if row := requirementByKey(pc, "requirement.ul_xc_flights"); row == nil || gi(row, "current") != 1 || gi(row, "required") != 5 {
			t.Errorf("§84a cross-country row = %v", row)
		}
		if row := requirementByKey(pc, "requirement.ul_xc_landing_flights"); row == nil || gi(row, "current") != 1 {
			t.Errorf("§84a intermediate-landing row = %v", row)
		}
		if row := requirementByKey(pc, "requirement.ul_xc_distance"); row == nil || gi(row, "required") != 200 || row["unit"] != "km" {
			t.Errorf("§84a distance row = %v", row)
		}

		auth := createPrivilege(t, c, lic, map[string]interface{}{"kind": "UL_PASSENGER_AUTH", "issuedOn": "2021-06-01"})
		result := getCurrencyStatus(t, c)
		pc = paxByKind(result)["THREE_AXIS"]
		assertStr(t, "authorised dayStatus", pc["dayStatus"], "current")
		assertStr(t, "authorised messageKey", pc["messageKey"], "pax.current_day_no_night_privilege")
		if _, ok := pc["requirements"]; ok {
			t.Errorf("requirements = %v, want none once authorised", pc["requirements"])
		}
		assertStr(t, "authorisation status", privilegeCurrency(t, result, auth.ID)["status"], "current")
	})

	t.Run("A1 guard: no privileges section without privileges", func(t *testing.T) {
		c := setupCurrencyUser(t, "priv-none")
		createLicenseCur(t, c, "EASA", "ATPL(A)")
		if v, ok := getCurrencyStatus(t, c)["privileges"]; ok {
			t.Errorf("privileges = %v, want absent", v)
		}
	})
}

func TestPrivileges_ExportImportAndAdmin(t *testing.T) {
	src := setupCurrencyUser(t, "priv-export")
	lic := createLicenseCur(t, src, "EASA", "SPL")
	createRatingCur(t, src, lic, "GLIDER", nil)
	createPrivilege(t, src, lic, map[string]interface{}{"kind": "CLOUD_FLYING", "issuedOn": "2022-04-01", "notes": "Wolkenflug"})
	createPrivilege(t, src, lic, map[string]interface{}{"kind": "LAUNCH_METHOD_TRAINED", "detail": "winch"})
	createPrivilege(t, src, lic, map[string]interface{}{"kind": "FI_S", "expiresOn": "2028-03-31"})

	resp := src.GET("/exports/json")
	requireStatus(t, resp, http.StatusOK)
	var backup map[string]interface{}
	if err := json.Unmarshal(resp.Body, &backup); err != nil {
		t.Fatalf("backup is not valid JSON: %v", err)
	}
	t.Run("the export carries the privileges inside their licence", func(t *testing.T) {
		licenses := backup["licenses"].([]interface{})
		privs, _ := licenses[0].(map[string]interface{})["privileges"].([]interface{})
		if len(privs) != 3 {
			t.Fatalf("licenses[0].privileges = %v, want 3", privs)
		}
	})

	dst := setupCurrencyUser(t, "priv-import")
	t.Run("a fresh account restores them onto the restored licence", func(t *testing.T) {
		restore := dst.Do("POST", "/imports/json", backup)
		requireStatus(t, restore, http.StatusOK)
		var summary struct {
			LicensesImported          int `json:"licensesImported"`
			LicencePrivilegesImported int `json:"licencePrivilegesImported"`
		}
		restore.JSON(&summary)
		assertInt(t, "licensesImported", summary.LicensesImported, 1)
		assertInt(t, "licencePrivilegesImported", summary.LicencePrivilegesImported, 3)

		lr := dst.GET("/licenses")
		requireStatus(t, lr, http.StatusOK)
		var licenses []map[string]interface{}
		lr.JSON(&licenses)
		if len(licenses) != 1 {
			t.Fatalf("licenses = %d, want 1", len(licenses))
		}
		newLic := licenses[0]["id"].(string)
		if newLic == lic {
			t.Error("restored licence kept its id")
		}
		got := map[string]privilegeBody{}
		for _, p := range listPrivileges(t, dst, newLic) {
			got[p.Kind] = p
		}
		if p := got["CLOUD_FLYING"]; p.IssuedOn == nil || *p.IssuedOn != "2022-04-01" || p.Notes == nil || *p.Notes != "Wolkenflug" {
			t.Errorf("CLOUD_FLYING = %+v", p)
		}
		if p := got["LAUNCH_METHOD_TRAINED"]; p.Detail == nil || *p.Detail != "winch" {
			t.Errorf("LAUNCH_METHOD_TRAINED = %+v", p)
		}
		if p := got["FI_S"]; p.ExpiresOn == nil || *p.ExpiresOn != "2028-03-31" {
			t.Errorf("FI_S = %+v", p)
		}
	})

	t.Run("an invalid privilege in a backup is a 400", func(t *testing.T) {
		bad := setupCurrencyUser(t, "priv-import-bad")
		licenses := backup["licenses"].([]interface{})
		entry := licenses[0].(map[string]interface{})
		entry["privileges"] = []interface{}{map[string]interface{}{"kind": "LAUNCH_METHOD_TRAINED", "detail": "catapult"}}
		assertStatus(t, bad.Do("POST", "/imports/json", backup), http.StatusBadRequest)
	})

	t.Run("admin stats count privileges by kind", func(t *testing.T) {
		ac := getAdminClient(t)
		resp := ac.GET("/admin/stats")
		requireStatus(t, resp, http.StatusOK)
		var s struct {
			LicencePrivileges struct {
				Total  int            `json:"total"`
				ByKind map[string]int `json:"byKind"`
			} `json:"licencePrivileges"`
		}
		resp.JSON(&s)
		if s.LicencePrivileges.Total < 6 || s.LicencePrivileges.ByKind["CLOUD_FLYING"] < 2 || s.LicencePrivileges.ByKind["FI_S"] < 2 {
			t.Errorf("licencePrivileges = %+v", s.LicencePrivileges)
		}
		sum := 0
		for _, n := range s.LicencePrivileges.ByKind {
			sum += n
		}
		assertInt(t, "total = sum of byKind", s.LicencePrivileges.Total, sum)
	})
}
