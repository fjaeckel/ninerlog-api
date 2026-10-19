//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// populateAccount logs two flights, an aircraft, a licence with a class
// rating, a credential, a contact, a custom rule and a custom report.
func populateAccount(t *testing.T, c *E2EClient, tag string) {
	t.Helper()
	requireStatus(t, c.POST("/aircraft", map[string]interface{}{
		"registration": "D-E" + strings.ToUpper(tag), "type": "C172", "make": "Cessna", "model": "172S", "aircraftClass": "SEP_LAND",
	}), http.StatusCreated)
	licID := createFileTestLicense(t, c, "DATA-"+tag)
	requireStatus(t, c.POST(fmt.Sprintf("/licenses/%s/ratings", licID), map[string]interface{}{
		"classType": "SEP_LAND", "issueDate": "2023-01-15",
	}), http.StatusCreated)
	requireStatus(t, c.POST("/credentials", map[string]interface{}{
		"credentialType": "EASA_CLASS2_MEDICAL", "issueDate": "2025-01-01", "issuingAuthority": "AME",
	}), http.StatusCreated)
	requireStatus(t, c.POST("/contacts", map[string]interface{}{"name": "Data Contact " + tag}), http.StatusCreated)
	createFlightWithCrew(t, c, "D-E"+strings.ToUpper(tag), []map[string]interface{}{{"name": "Crew " + tag, "role": "Instructor"}})
	requireStatus(t, c.POST("/flights", overrideFlightBody(map[string]interface{}{
		"aircraftReg": "D-E" + strings.ToUpper(tag), "nightTime": 30,
	})), http.StatusCreated)
	requireStatus(t, c.POST("/custom-currency", map[string]interface{}{
		"name": "Landings " + tag,
		"definition": map[string]interface{}{
			"window":       map[string]interface{}{"amount": 90, "unit": "days"},
			"requirements": []map[string]interface{}{{"metric": "landings", "min": 3}},
		},
	}), http.StatusCreated)
	requireStatus(t, c.POST("/reports/custom", map[string]interface{}{
		"name": "Report " + tag,
		"definition": map[string]interface{}{
			"filter": map[string]interface{}{}, "window": map[string]interface{}{"kind": "all"},
			"groupBy": "year", "metric": "flights",
		},
	}), http.StatusCreated)
}

func exportBackup(t *testing.T, c *E2EClient, path string) (map[string]interface{}, *Response) {
	t.Helper()
	r := c.GET(path)
	requireStatus(t, r, http.StatusOK)
	var backup map[string]interface{}
	if err := json.Unmarshal(r.Body, &backup); err != nil {
		t.Fatalf("backup is not valid JSON: %v", err)
	}
	return backup, r
}

func sectionLen(backup map[string]interface{}, section string) int {
	items, _ := backup[section].([]interface{})
	return len(items)
}

func TestUserData_ExportRestoreLifecycle(t *testing.T) {
	source := NewE2EClient(t)
	registerAndLogin(t, source, uniqueEmail("udata-src"), "SecurePass123!", "Source")
	populateAccount(t, source, "ud1")

	backup, resp := exportBackup(t, source, "/users/me/data")

	t.Run("export is an attachment in the backup format", func(t *testing.T) {
		if cd := resp.Headers.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment; filename=ninerlog_backup_") {
			t.Errorf("Content-Disposition = %q", cd)
		}
		if backup["format"] != "NinerLog JSON Backup" {
			t.Errorf("format = %v", backup["format"])
		}
		if backup["version"] != "1.0" {
			t.Errorf("version = %v", backup["version"])
		}
	})

	t.Run("deprecated export serves the same payload", func(t *testing.T) {
		old, oldResp := exportBackup(t, source, "/exports/json")
		if cd := oldResp.Headers.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment; filename=ninerlog_backup_") {
			t.Errorf("Content-Disposition = %q", cd)
		}
		for _, section := range []string{"flights", "aircraft", "licenses", "credentials", "contacts", "customCurrencyRules", "customReports"} {
			if got, want := sectionLen(old, section), sectionLen(backup, section); got != want {
				t.Errorf("%s: deprecated route has %d, new route %d", section, got, want)
			}
		}
	})

	dest := NewE2EClient(t)
	registerAndLogin(t, dest, uniqueEmail("udata-dst"), "SecurePass123!", "Destination")

	t.Run("restores into an empty account", func(t *testing.T) {
		r := dest.POST("/users/me/data", backup)
		requireStatus(t, r, http.StatusOK)
		var summary struct {
			Mode                        string        `json:"mode"`
			AircraftImported            int           `json:"aircraftImported"`
			FlightsImported             int           `json:"flightsImported"`
			CrewMembersImported         int           `json:"crewMembersImported"`
			CustomCurrencyRulesImported int           `json:"customCurrencyRulesImported"`
			CustomReportsImported       int           `json:"customReportsImported"`
			Warnings                    []interface{} `json:"warnings"`
		}
		r.JSON(&summary)
		if summary.Mode != "restore" {
			t.Errorf("mode = %q", summary.Mode)
		}
		assertInt(t, "aircraftImported", summary.AircraftImported, 1)
		assertInt(t, "flightsImported", summary.FlightsImported, 2)
		assertInt(t, "crewMembersImported", summary.CrewMembersImported, 1)
		assertInt(t, "customCurrencyRulesImported", summary.CustomCurrencyRulesImported, 1)
		assertInt(t, "customReportsImported", summary.CustomReportsImported, 1)
		if summary.Warnings == nil || len(summary.Warnings) != 0 {
			t.Errorf("warnings = %v, want an empty list", summary.Warnings)
		}

		restored, _ := exportBackup(t, dest, "/users/me/data")
		for _, section := range []string{"flights", "aircraft", "licenses", "credentials", "contacts", "customCurrencyRules", "customReports"} {
			if got, want := sectionLen(restored, section), sectionLen(backup, section); got != want {
				t.Errorf("%s after restore: %d, want %d", section, got, want)
			}
		}
	})

	t.Run("a second restore is refused and changes nothing", func(t *testing.T) {
		r := dest.POST("/users/me/data", backup)
		requireStatus(t, r, http.StatusConflict)
		assertErrorCode(t, r, "account_not_empty")

		r = dest.POST("/imports/json", backup)
		requireStatus(t, r, http.StatusConflict)
		assertErrorCode(t, r, "account_not_empty")

		after, _ := exportBackup(t, dest, "/users/me/data")
		assertInt(t, "flights", sectionLen(after, "flights"), 2)
	})

	t.Run("replace yields one copy", func(t *testing.T) {
		r := dest.POST("/users/me/data?mode=replace", backup)
		requireStatus(t, r, http.StatusOK)

		after, _ := exportBackup(t, dest, "/users/me/data")
		for _, section := range []string{"flights", "aircraft", "licenses", "credentials", "contacts", "customCurrencyRules", "customReports"} {
			if got, want := sectionLen(after, section), sectionLen(backup, section); got != want {
				t.Errorf("%s after replace: %d, want %d", section, got, want)
			}
		}
	})

	t.Run("replace through the deprecated route behaves the same", func(t *testing.T) {
		requireStatus(t, dest.POST("/imports/json?mode=replace", backup), http.StatusOK)
		after, _ := exportBackup(t, dest, "/users/me/data")
		assertInt(t, "flights", sectionLen(after, "flights"), 2)
	})

	t.Run("replace is refused while the account holds document files", func(t *testing.T) {
		licenses, _ := exportBackup(t, dest, "/users/me/data")
		licID := licenses["licenses"].([]interface{})[0].(map[string]interface{})["license"].(map[string]interface{})["id"].(string)
		requireStatus(t, uploadDocumentFile(t, dest, fmt.Sprintf("/licenses/%s/files", licID), "front.png", "image/png", "", e2ePNG(t, 40, 30)), http.StatusCreated)

		r := dest.POST("/users/me/data?mode=replace", backup)
		requireStatus(t, r, http.StatusConflict)
		assertErrorCode(t, r, "replace_would_drop_attachments")

		after, _ := exportBackup(t, dest, "/users/me/data")
		assertInt(t, "flights", sectionLen(after, "flights"), 2)
	})

	t.Run("an invalid mode is a 400", func(t *testing.T) {
		assertStatus(t, dest.POST("/users/me/data?mode=merge", backup), http.StatusBadRequest)
	})

	t.Run("a newer major version is a 422", func(t *testing.T) {
		fresh := NewE2EClient(t)
		registerAndLogin(t, fresh, uniqueEmail("udata-ver"), "SecurePass123!", "Version")
		newer := map[string]interface{}{"format": "NinerLog JSON Backup", "version": "2.0"}
		r := fresh.POST("/users/me/data", newer)
		requireStatus(t, r, http.StatusUnprocessableEntity)
		assertErrorCode(t, r, "backup_version_unsupported")
	})

	t.Run("a backup without a version restores", func(t *testing.T) {
		fresh := NewE2EClient(t)
		registerAndLogin(t, fresh, uniqueEmail("udata-nover"), "SecurePass123!", "NoVersion")
		legacy := map[string]interface{}{"format": "NinerLog JSON Backup", "flights": []interface{}{}}
		requireStatus(t, fresh.POST("/users/me/data", legacy), http.StatusOK)
	})

	t.Run("a foreign format is a 400 without internals", func(t *testing.T) {
		fresh := NewE2EClient(t)
		registerAndLogin(t, fresh, uniqueEmail("udata-foreign"), "SecurePass123!", "Foreign")
		r := fresh.POST("/users/me/data", map[string]interface{}{"format": "Other Tool"})
		requireStatus(t, r, http.StatusBadRequest)
	})

	t.Run("a stale custom rule restores paused with a warning", func(t *testing.T) {
		fresh := NewE2EClient(t)
		registerAndLogin(t, fresh, uniqueEmail("udata-warn"), "SecurePass123!", "Warn")
		stale := map[string]interface{}{
			"format": "NinerLog JSON Backup", "version": "1.0",
			"customCurrencyRules": []map[string]interface{}{{
				"name": "Stale", "enabled": true, "notify": true,
				"definition": map[string]interface{}{
					"window":       map[string]interface{}{"amount": 90, "unit": "fortnights"},
					"requirements": []map[string]interface{}{{"metric": "landings", "min": 3}},
				},
			}},
		}
		r := fresh.POST("/users/me/data", stale)
		requireStatus(t, r, http.StatusOK)
		var summary struct {
			CustomCurrencyRulesImported int `json:"customCurrencyRulesImported"`
			Warnings                    []struct {
				Section string `json:"section"`
				Index   int    `json:"index"`
			} `json:"warnings"`
		}
		r.JSON(&summary)
		assertInt(t, "customCurrencyRulesImported", summary.CustomCurrencyRulesImported, 1)
		if len(summary.Warnings) != 1 || summary.Warnings[0].Section != "customCurrencyRules" || summary.Warnings[0].Index != 0 {
			t.Fatalf("warnings = %+v", summary.Warnings)
		}

		after, _ := exportBackup(t, fresh, "/users/me/data")
		rule := after["customCurrencyRules"].([]interface{})[0].(map[string]interface{})
		if rule["enabled"] != false || rule["notify"] != false {
			t.Errorf("stale rule restored enabled=%v notify=%v, want both false", rule["enabled"], rule["notify"])
		}
	})
}

func TestUserData_RequiresAuth(t *testing.T) {
	c := NewE2EClient(t)
	assertStatus(t, c.GET("/users/me/data"), http.StatusUnauthorized)
	assertStatus(t, c.POST("/users/me/data", map[string]interface{}{"format": "NinerLog JSON Backup"}), http.StatusUnauthorized)
	assertStatus(t, c.DELETE("/users/me/data"), http.StatusUnauthorized)
}

func TestUserData_DeleteScopes(t *testing.T) {
	t.Run("scope=flights keeps everything else", func(t *testing.T) {
		c := NewE2EClient(t)
		registerAndLogin(t, c, uniqueEmail("udata-del-flights"), "SecurePass123!", "DelFlights")
		populateAccount(t, c, "df1")

		r := c.DELETE("/users/me/data?scope=flights")
		requireStatus(t, r, http.StatusOK)
		var body struct {
			Message        string `json:"message"`
			DeletedFlights *int   `json:"deletedFlights"`
		}
		r.JSON(&body)
		if body.DeletedFlights == nil {
			t.Fatalf("deletedFlights missing: %s", r.Body)
		}
		assertInt(t, "deletedFlights", *body.DeletedFlights, 2)

		after, _ := exportBackup(t, c, "/users/me/data")
		assertInt(t, "flights", sectionLen(after, "flights"), 0)
		assertInt(t, "aircraft", sectionLen(after, "aircraft"), 1)
		assertInt(t, "licenses", sectionLen(after, "licenses"), 1)
		assertInt(t, "contacts", sectionLen(after, "contacts"), 2)
		assertInt(t, "customCurrencyRules", sectionLen(after, "customCurrencyRules"), 1)
	})

	t.Run("default scope wipes the logbook but not the account", func(t *testing.T) {
		c := NewE2EClient(t)
		registerAndLogin(t, c, uniqueEmail("udata-del-all"), "SecurePass123!", "DelAll")
		populateAccount(t, c, "da1")

		r := c.DELETE("/users/me/data")
		requireStatus(t, r, http.StatusOK)
		var body map[string]interface{}
		r.JSON(&body)
		if _, ok := body["deletedFlights"]; ok {
			t.Errorf("scope=all reported deletedFlights: %s", r.Body)
		}
		assertStatus(t, c.GET("/users/me"), http.StatusOK)

		after, _ := exportBackup(t, c, "/users/me/data")
		for _, section := range []string{"flights", "aircraft", "licenses", "credentials", "contacts"} {
			assertInt(t, section, sectionLen(after, section), 0)
		}
	})

	t.Run("scope=all matches the old behaviour for rules and reports", func(t *testing.T) {
		c := NewE2EClient(t)
		registerAndLogin(t, c, uniqueEmail("udata-del-scope"), "SecurePass123!", "DelScope")
		populateAccount(t, c, "ds1")

		requireStatus(t, c.DELETE("/users/me/data?scope=all"), http.StatusOK)

		after, _ := exportBackup(t, c, "/users/me/data")
		assertInt(t, "customCurrencyRules", sectionLen(after, "customCurrencyRules"), 1)
		assertInt(t, "customReports", sectionLen(after, "customReports"), 1)
	})

	t.Run("an unknown scope is a 400", func(t *testing.T) {
		c := NewE2EClient(t)
		registerAndLogin(t, c, uniqueEmail("udata-del-bad"), "SecurePass123!", "DelBad")
		assertStatus(t, c.DELETE("/users/me/data?scope=users"), http.StatusBadRequest)
	})

	t.Run("deprecated DELETE /flights/delete-all still works", func(t *testing.T) {
		c := NewE2EClient(t)
		registerAndLogin(t, c, uniqueEmail("udata-del-old"), "SecurePass123!", "DelOld")
		populateAccount(t, c, "do1")

		r := c.DELETE("/flights/delete-all")
		requireStatus(t, r, http.StatusOK)
		var body struct {
			Deleted *int `json:"deleted"`
		}
		r.JSON(&body)
		if body.Deleted == nil {
			t.Fatalf("deleted missing: %s", r.Body)
		}
		assertInt(t, "deleted", *body.Deleted, 2)

		after, _ := exportBackup(t, c, "/users/me/data")
		assertInt(t, "flights", sectionLen(after, "flights"), 0)
		assertInt(t, "aircraft", sectionLen(after, "aircraft"), 1)
	})
}
