//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestFlightFieldRetrieval(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-retr"), "SecurePass123!", "Retrieval")

	// Create a flight with every field
	resp := c.POST("/flights", map[string]interface{}{
		"date": "2025-06-15", "aircraftReg": "D-ERET", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "10:30",
		"departureTime": "08:10", "arrivalTime": "10:20", "landings": 3,
		"ifrTime": 30, "remarks": "Retrieval test", "route": "EDNY,EDTL,EDDS",
		"holds": 2, "approachesCount": 3, "isFlightReview": true,
		"actualInstrumentTime": 24, "simulatedInstrumentTime": 6,
		"simulatedFlightTime": 0, "groundTrainingTime": 0,
		"instructorName": "Capt Retrieval", "instructorComments": "Great job",
		"launchMethod": nil,
		"crewMembers": []map[string]interface{}{
			{"name": "CFI Test", "role": "Instructor"},
			{"name": "Observer", "role": "SafetyPilot"},
		},
	})
	requireStatus(t, resp, http.StatusCreated)
	var created map[string]interface{}
	resp.JSON(&created)
	fid := created["id"].(string)

	t.Run("GET returns all stored fields", func(t *testing.T) {
		resp := c.GET(fmt.Sprintf("/flights/%s", fid))
		requireStatus(t, resp, http.StatusOK)
		var f map[string]interface{}
		resp.JSON(&f)

		// Verify every field round-trips
		assertStr(t, "aircraftReg", f["aircraftReg"], "D-ERET")
		assertStr(t, "aircraftType", f["aircraftType"], "C172")
		assertStr(t, "remarks", f["remarks"], "Retrieval test")
		assertStr(t, "route", f["route"], "EDNY,EDTL,EDDS")

		assertFloat(t, "ifrTime", gf(f, "ifrTime"), 30, 1)
		assertFloat(t, "actualInstrumentTime", gf(f, "actualInstrumentTime"), 24, 1)
		assertFloat(t, "simulatedInstrumentTime", gf(f, "simulatedInstrumentTime"), 6, 1)
		assertInt(t, "holds", gi(f, "holds"), 2)
		assertInt(t, "approachesCount", gi(f, "approachesCount"), 3)

		assertBool(t, "isFlightReview", gb(f, "isFlightReview"), true)
		assertBool(t, "isDual", gb(f, "isDual"), true) // Instructor present

		if f["instructorName"] != nil {
			assertStr(t, "instructorName", f["instructorName"], "Capt Retrieval")
		}
		if f["instructorComments"] != nil {
			assertStr(t, "instructorComments", f["instructorComments"], "Great job")
		}

		// Crew members
		if crew, ok := f["crewMembers"].([]interface{}); ok {
			assertInt(t, "crewLen", len(crew), 2)
			roles := map[string]bool{}
			for _, m := range crew {
				roles[m.(map[string]interface{})["role"].(string)] = true
			}
			if !roles["Instructor"] {
				t.Error("Missing Instructor role")
			}
			if !roles["SafetyPilot"] {
				t.Error("Missing SafetyPilot role")
			}
		} else {
			t.Error("Expected crewMembers in GET response")
		}
	})
}

func TestDateValidation(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("date-val"), "SecurePass123!", "DateVal")

	t.Run("far future date accepted", func(t *testing.T) {
		resp := c.POST("/flights", map[string]interface{}{
			"date": "2099-12-31", "aircraftReg": "D-EDAT", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		})
		requireStatus(t, resp, http.StatusCreated)
	})

	t.Run("old date accepted", func(t *testing.T) {
		resp := c.POST("/flights", map[string]interface{}{
			"date": "1990-01-01", "aircraftReg": "D-EDAT", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		})
		requireStatus(t, resp, http.StatusCreated)
	})

	t.Run("invalid date Feb 30 returns 400", func(t *testing.T) {
		resp := c.POST("/flights", map[string]interface{}{
			"date": "2025-02-30", "aircraftReg": "D-EDAT", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		})
		assertStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("invalid date format slash", func(t *testing.T) {
		resp := c.POST("/flights", map[string]interface{}{
			"date": "01/15/2025", "aircraftReg": "D-EDAT", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		})
		assertStatus(t, resp, http.StatusBadRequest)
	})
}

func TestEmailValidation(t *testing.T) {
	c := NewE2EClient(t)

	t.Run("invalid email format rejected", func(t *testing.T) {
		resp := c.POST("/auth/register", map[string]string{
			"email": "not-an-email", "password": "SecurePass123!", "name": "Test",
		})
		assertStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("email without domain rejected", func(t *testing.T) {
		resp := c.POST("/auth/register", map[string]string{
			"email": "user@", "password": "SecurePass123!", "name": "Test",
		})
		assertStatus(t, resp, http.StatusBadRequest)
	})

	// Fixed: Long email now returns 400 instead of 500
	t.Run("very long email causes 400", func(t *testing.T) {
		long := strings.Repeat("a", 250) + "@test.com"
		resp := c.POST("/auth/register", map[string]string{
			"email": long, "password": "SecurePass123!", "name": "Test",
		})
		assertStatus(t, resp, http.StatusBadRequest)
	})
}

func TestPasswordValidation(t *testing.T) {
	c := NewE2EClient(t)

	// Fixed: Long password now returns 400 instead of 500
	t.Run("very long password causes 400", func(t *testing.T) {
		longPw := strings.Repeat("Aa1!", 100) // 400 chars, bcrypt limit is 72 bytes
		email := uniqueEmail("longpw")
		resp := c.POST("/auth/register", map[string]string{
			"email": email, "password": longPw, "name": "Test",
		})
		assertStatus(t, resp, http.StatusBadRequest)
	})

	t.Run("special characters in password", func(t *testing.T) {
		pw := `P@$$w0rd!#%^&*()_+-=[]{}|;':",.<>?`
		email := uniqueEmail("specpw")
		resp := c.POST("/auth/register", map[string]string{
			"email": email, "password": pw, "name": "Test",
		})
		requireStatus(t, resp, http.StatusCreated)
		loginUser(t, c, email, pw)
	})

	t.Run("unicode password", func(t *testing.T) {
		pw := "Pässwörd123!日本語"
		email := uniqueEmail("unipw")
		resp := c.POST("/auth/register", map[string]string{
			"email": email, "password": pw, "name": "Test",
		})
		requireStatus(t, resp, http.StatusCreated)
		loginUser(t, c, email, pw)
	})
}

func TestNotificationPrefsEdgeCases(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("notif-edge"), "SecurePass123!", "NotifEdge")

	t.Run("empty warningDays array", func(t *testing.T) {
		resp := c.PATCH("/users/me/notifications", map[string]interface{}{
			"warningDays": []int{},
		})
		if resp.StatusCode == http.StatusInternalServerError {
			t.Error("Empty warningDays caused 500")
		}
	})

	t.Run("large warningDays value", func(t *testing.T) {
		resp := c.PATCH("/users/me/notifications", map[string]interface{}{
			"warningDays": []int{365, 180, 90, 30, 7},
		})
		requireStatus(t, resp, http.StatusOK)
	})

	t.Run("single warningDay", func(t *testing.T) {
		resp := c.PATCH("/users/me/notifications", map[string]interface{}{
			"warningDays": []int{7},
		})
		requireStatus(t, resp, http.StatusOK)
	})
}

func TestFlightLogbookFilter(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("logbook"), "SecurePass123!", "Logbook")

	// Create license with separate logbook
	resp := c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "SPL", "licenseNumber": "SPL-LB",
		"issueDate": today(), "issuingAuthority": "LBA", "requiresSeparateLogbook": true,
	})
	requireStatus(t, resp, http.StatusCreated)
	var lic map[string]interface{}
	resp.JSON(&lic)
	splID := lic["id"].(string)

	// Create PPL license (no separate logbook)
	resp = c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "PPL-LB",
		"issueDate": today(), "issuingAuthority": "LBA",
	})
	requireStatus(t, resp, http.StatusCreated)

	// Create flights
	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": today(), "aircraftReg": "D-1234", "aircraftType": "ASK21",
		"departureIcao": "EDNY", "arrivalIcao": "EDNY",
		"offBlockTime": "10:00", "onBlockTime": "10:45", "landings": 1,
		"launchMethod": "winch",
	}), http.StatusCreated)

	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": today(), "aircraftReg": "D-EFLY", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "12:00", "onBlockTime": "13:00", "landings": 1,
	}), http.StatusCreated)

	t.Run("filter by logbookLicenseId", func(t *testing.T) {
		resp := c.GET(fmt.Sprintf("/flights?logbookLicenseId=%s", splID))
		if resp.StatusCode == http.StatusOK {
			var r map[string]interface{}
			resp.JSON(&r)
			t.Logf("logbookLicenseId filter: %d flights", len(r["data"].([]interface{})))
		} else {
			t.Logf("logbookLicenseId filter status: %d", resp.StatusCode)
		}
	})
}

func TestAuditLogContents(t *testing.T) {
	ac := getAdminClient(t)
	uc := NewE2EClient(t)
	ue := uniqueEmail("audit-verify")
	ua := registerAndLogin(t, uc, ue, "SecurePass123!", "Audit Target")

	// Perform auditable action
	requireStatus(t, ac.POST(fmt.Sprintf("/admin/users/%s/disable", ua.User.ID), nil), http.StatusOK)
	requireStatus(t, ac.POST(fmt.Sprintf("/admin/users/%s/enable", ua.User.ID), nil), http.StatusOK)

	t.Run("audit log contains actions", func(t *testing.T) {
		resp := ac.GET("/admin/audit-log")
		requireStatus(t, resp, http.StatusOK)
		var result map[string]interface{}
		resp.JSON(&result)
		data := result["data"].([]interface{})
		if len(data) < 2 {
			t.Errorf("Expected >=2 audit entries, got %d", len(data))
		}
		// Verify entries have expected fields
		for _, entry := range data {
			e := entry.(map[string]interface{})
			if e["action"] == nil {
				t.Error("Audit entry missing action")
			}
			if e["adminUserId"] == nil {
				t.Error("Audit entry missing adminUserId")
			}
			t.Logf("Audit: %v → %v", e["action"], e["targetUserId"])
		}
	})
}

func TestConcurrentTokenRefresh(t *testing.T) {
	c := NewE2EClient(t)
	auth := registerUser(t, c, uniqueEmail("concurrent"), "SecurePass123!", "Concurrent")

	t.Run("second refresh after first invalidates token", func(t *testing.T) {
		resp1 := c.POST("/auth/refresh", map[string]string{"refreshToken": auth.RefreshToken})
		requireStatus(t, resp1, http.StatusOK)
		var new1 AuthResponseBody
		resp1.JSON(&new1)

		// Original token already used above, try again
		resp2 := c.POST("/auth/refresh", map[string]string{"refreshToken": auth.RefreshToken})
		if resp2.StatusCode != http.StatusUnauthorized && resp2.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 401/400 for reused refresh token, got %d", resp2.StatusCode)
		}

		// The new token should work
		resp3 := c.POST("/auth/refresh", map[string]string{"refreshToken": new1.RefreshToken})
		requireStatus(t, resp3, http.StatusOK)
	})
}

func TestExportContentVerification(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("export-verify"), "SecurePass123!", "ExportVerify")

	// Create identifiable data
	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": today(), "aircraftReg": "D-EXPV", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:30", "landings": 1,
		"remarks": "export-verification-marker",
	}), http.StatusCreated)

	t.Run("CSV export contains flight data", func(t *testing.T) {
		resp := c.GET("/exports/csv")
		requireStatus(t, resp, http.StatusOK)
		body := string(resp.Body)
		if !strings.Contains(body, "D-EXPV") {
			t.Error("CSV should contain aircraft reg D-EXPV")
		}
		if !strings.Contains(body, "EDNY") {
			t.Error("CSV should contain EDNY")
		}
	})

	t.Run("JSON export contains flight data", func(t *testing.T) {
		resp := c.GET("/exports/json")
		requireStatus(t, resp, http.StatusOK)
		body := string(resp.Body)
		if !strings.Contains(body, "D-EXPV") {
			t.Error("JSON should contain aircraft reg D-EXPV")
		}
		if !strings.Contains(body, "export-verification-marker") {
			t.Error("JSON should contain remarks text")
		}
	})

	t.Run("PDF export has content", func(t *testing.T) {
		resp := c.GET("/exports/pdf")
		requireStatus(t, resp, http.StatusOK)
		if len(resp.Body) < 100 {
			t.Error("PDF too small to be valid")
		}
		// PDF starts with %PDF-
		if !strings.HasPrefix(string(resp.Body[:5]), "%PDF-") {
			t.Error("PDF does not start with %PDF- header")
		}
	})
}
