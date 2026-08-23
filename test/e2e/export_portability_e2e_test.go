//go:build e2e

package e2e_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Everything a user owns must survive a move to another installation.
// TestJSONExportImportRoundTrip covers the logbook itself; this covers the
// sections that were stored per user but missing from the backup — contacts,
// custom currency rules, notification preferences and the carried-forward
// hours baseline — because losing them was silent: the export succeeded, it
// was just incomplete.

func TestBackupCarriesEveryUserOwnedSection(t *testing.T) {
	source := NewE2EClient(t)
	registerAndLogin(t, source, uniqueEmail("portsrc"), "SecurePass123!", "Port Source")

	// --- Seed one row in each of the newly portable sections. ---

	requireStatus(t, source.POST("/contacts", map[string]interface{}{
		"name":  "Alex Kestrel",
		"email": "alex.kestrel@example.com",
		"phone": "+49 170 1234567",
		"notes": "Type rating examiner",
	}), http.StatusCreated)

	ruleID := createCustomRule(t, source, "Night landings")
	requireStatus(t, source.PUT("/custom-currency/"+ruleID+"/notify",
		map[string]interface{}{"notify": true}), http.StatusOK)

	pausedID := createCustomRule(t, source, "Paused rule")
	requireStatus(t, source.PUT("/custom-currency/"+pausedID+"/enabled",
		map[string]interface{}{"enabled": false}), http.StatusOK)

	requireStatus(t, source.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"credential_medical", "currency_night"},
		"warningDays":       []int{45, 14},
		"checkHour":         7,
	}), http.StatusOK)

	requireStatus(t, source.PUT("/users/me/baseline", map[string]interface{}{
		"baselineDate": "2019-06-30",
		"totalFlights": 210,
		"totalMinutes": 24600,
		"picMinutes":   18000,
		"notes":        "Paper logbook up to mid-2019",
	}), http.StatusOK)

	// --- The export carries all of it. ---

	backupResp := source.GET("/exports/json")
	requireStatus(t, backupResp, http.StatusOK)

	var backup map[string]interface{}
	if err := json.Unmarshal(backupResp.Body, &backup); err != nil {
		t.Fatalf("backup is not valid JSON: %v", err)
	}

	for _, section := range []string{
		"contacts", "customCurrencyRules", "notificationPreferences", "flightBaseline",
	} {
		if _, ok := backup[section]; !ok {
			t.Fatalf("backup is missing the %q section — a user's data would be lost on a move", section)
		}
	}

	if n := len(backup["contacts"].([]interface{})); n != 1 {
		t.Errorf("contacts in backup = %d, want 1", n)
	}
	rules, _ := backup["customCurrencyRules"].([]interface{})
	if len(rules) != 2 {
		t.Fatalf("customCurrencyRules in backup = %d, want 2", len(rules))
	}
	// A share token is unique to the installation that minted it and must not
	// ride along in a portable backup.
	for _, r := range rules {
		rule := r.(map[string]interface{})
		if _, leaked := rule["shareToken"]; leaked {
			t.Error("backup carries a share token")
		}
		if _, leaked := rule["isShared"]; leaked {
			t.Error("backup carries sharing state")
		}
	}

	// --- A fresh account restores all of it. ---

	dest := NewE2EClient(t)
	registerAndLogin(t, dest, uniqueEmail("portdst"), "SecurePass123!", "Port Dest")

	restoreResp := dest.Do("POST", "/imports/json", backup)
	requireStatus(t, restoreResp, http.StatusOK)

	var summary struct {
		ContactsImported                int  `json:"contactsImported"`
		ContactsSkipped                 int  `json:"contactsSkipped"`
		CustomCurrencyRulesImported     int  `json:"customCurrencyRulesImported"`
		NotificationPreferencesImported bool `json:"notificationPreferencesImported"`
		FlightBaselineImported          bool `json:"flightBaselineImported"`
	}
	if err := restoreResp.JSON(&summary); err != nil {
		t.Fatalf("invalid summary: %v — body=%s", err, string(restoreResp.Body))
	}
	assertInt(t, "contactsImported", summary.ContactsImported, 1)
	assertInt(t, "customCurrencyRulesImported", summary.CustomCurrencyRulesImported, 2)
	if !summary.NotificationPreferencesImported {
		t.Error("notification preferences were not restored")
	}
	if !summary.FlightBaselineImported {
		t.Error("flight baseline was not restored")
	}

	// Contacts.
	{
		r := dest.GET("/contacts")
		requireStatus(t, r, http.StatusOK)
		var contacts []map[string]interface{}
		if err := r.JSON(&contacts); err != nil {
			t.Fatalf("decode contacts: %v", err)
		}
		if len(contacts) != 1 || contacts[0]["name"] != "Alex Kestrel" {
			t.Fatalf("contact not restored: %v", contacts)
		}
		if contacts[0]["email"] != "alex.kestrel@example.com" {
			t.Errorf("contact email lost: %v", contacts[0]["email"])
		}
	}

	// Custom currency rules, including the paused one and the notify opt-in.
	{
		r := dest.GET("/custom-currency")
		requireStatus(t, r, http.StatusOK)
		var list []map[string]interface{}
		if err := r.JSON(&list); err != nil {
			t.Fatalf("decode rules: %v", err)
		}
		if len(list) != 2 {
			t.Fatalf("rules restored = %d, want 2", len(list))
		}
		byName := map[string]map[string]interface{}{}
		for _, entry := range list {
			rule := entry["rule"].(map[string]interface{})
			byName[rule["name"].(string)] = rule
		}
		night, ok := byName["Night landings"]
		if !ok {
			t.Fatalf("restored rules: %v", byName)
		}
		if night["notify"] != true {
			t.Errorf("notify opt-in lost on restore: %v", night["notify"])
		}
		if night["isShared"] != false {
			t.Errorf("restored rule must be private: %v", night["isShared"])
		}
		paused, ok := byName["Paused rule"]
		if !ok {
			t.Fatalf("paused rule missing: %v", byName)
		}
		if paused["enabled"] != false {
			t.Errorf("paused state lost on restore: %v", paused["enabled"])
		}
	}

	// Notification preferences.
	{
		r := dest.GET("/users/me/notifications")
		requireStatus(t, r, http.StatusOK)
		var prefs map[string]interface{}
		if err := r.JSON(&prefs); err != nil {
			t.Fatalf("decode prefs: %v", err)
		}
		if prefs["checkHour"] != float64(7) {
			t.Errorf("checkHour = %v, want 7", prefs["checkHour"])
		}
		if prefs["emailEnabled"] != true {
			t.Errorf("emailEnabled = %v, want true", prefs["emailEnabled"])
		}
	}

	// Flight baseline.
	{
		r := dest.GET("/users/me/baseline")
		requireStatus(t, r, http.StatusOK)
		var baseline map[string]interface{}
		if err := r.JSON(&baseline); err != nil {
			t.Fatalf("decode baseline: %v", err)
		}
		if baseline["totalFlights"] != float64(210) {
			t.Errorf("totalFlights = %v, want 210", baseline["totalFlights"])
		}
		if baseline["totalMinutes"] != float64(24600) {
			t.Errorf("totalMinutes = %v, want 24600", baseline["totalMinutes"])
		}
	}
}

// A restore into an account that already holds a contact of the same name
// skips it rather than failing or duplicating.
func TestBackupRestoreSkipsContactsAlreadyHeld(t *testing.T) {
	source := NewE2EClient(t)
	registerAndLogin(t, source, uniqueEmail("dupsrc"), "SecurePass123!", "Dup Source")
	requireStatus(t, source.POST("/contacts", map[string]interface{}{
		"name": "Robin Vega",
	}), http.StatusCreated)

	backupResp := source.GET("/exports/json")
	requireStatus(t, backupResp, http.StatusOK)
	var backup map[string]interface{}
	if err := json.Unmarshal(backupResp.Body, &backup); err != nil {
		t.Fatalf("backup is not valid JSON: %v", err)
	}

	dest := NewE2EClient(t)
	registerAndLogin(t, dest, uniqueEmail("dupdst"), "SecurePass123!", "Dup Dest")
	requireStatus(t, dest.POST("/contacts", map[string]interface{}{
		"name": "Robin Vega",
	}), http.StatusCreated)

	restoreResp := dest.Do("POST", "/imports/json", backup)
	requireStatus(t, restoreResp, http.StatusOK)

	var summary struct {
		ContactsImported int `json:"contactsImported"`
		ContactsSkipped  int `json:"contactsSkipped"`
	}
	if err := restoreResp.JSON(&summary); err != nil {
		t.Fatalf("invalid summary: %v", err)
	}
	assertInt(t, "contactsImported", summary.ContactsImported, 0)
	assertInt(t, "contactsSkipped", summary.ContactsSkipped, 1)

	r := dest.GET("/contacts")
	requireStatus(t, r, http.StatusOK)
	var contacts []map[string]interface{}
	if err := r.JSON(&contacts); err != nil {
		t.Fatalf("decode contacts: %v", err)
	}
	if len(contacts) != 1 {
		t.Errorf("restore duplicated a contact: %d held", len(contacts))
	}
}
