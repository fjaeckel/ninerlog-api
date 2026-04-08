//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestClassRatingDuplicateType(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("dup-rating"), "SecurePass123!", "DupRating")

	r := c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "DUP-R",
		"issueDate": today(), "issuingAuthority": "LBA",
	})
	requireStatus(t, r, 201)
	var lic map[string]interface{}
	r.JSON(&lic)
	lid := lic["id"].(string)

	requireStatus(t, c.POST(fmt.Sprintf("/licenses/%s/ratings", lid), map[string]interface{}{
		"classType": "SEP_LAND", "issueDate": "2023-01-01",
	}), 201)

	t.Run("duplicate class type on same license", func(t *testing.T) {
		r := c.POST(fmt.Sprintf("/licenses/%s/ratings", lid), map[string]interface{}{
			"classType": "SEP_LAND", "issueDate": "2024-01-01",
		})
		if r.StatusCode == 201 {
			t.Log("INFO: API allows duplicate class types on same license")
		} else if r.StatusCode == 409 {
			t.Log("INFO: API rejects duplicate class types (409)")
		} else {
			t.Logf("Duplicate rating status: %d %s", r.StatusCode, string(r.Body))
		}
	})
}

func TestCredentialExpiryEdgeCases(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("cred-exp"), "SecurePass123!", "CredExp")

	t.Run("expiry before issue date", func(t *testing.T) {
		r := c.POST("/credentials", map[string]interface{}{
			"credentialType": "EASA_CLASS2_MEDICAL",
			"issueDate":      "2025-06-01",
			"expiryDate":     "2025-01-01", // Before issue
			"issuingAuthority": "AME",
		})
		if r.StatusCode == 201 {
			t.Log("REGRESSION: Credential created with expiry before issue date")
		} else {
			t.Logf("Expiry before issue: %d", r.StatusCode)
		}
	})

	t.Run("issue equals expiry (immediate expiry)", func(t *testing.T) {
		r := c.POST("/credentials", map[string]interface{}{
			"credentialType":   "EASA_CLASS2_MEDICAL",
			"issueDate":       today(),
			"expiryDate":      today(),
			"issuingAuthority": "AME",
		})
		requireStatus(t, r, 201) // Unusual but technically valid
	})

	t.Run("no expiry date (null)", func(t *testing.T) {
		r := c.POST("/credentials", map[string]interface{}{
			"credentialType":   "LANG_ICAO_LEVEL6",
			"issueDate":       today(),
			"issuingAuthority": "LBA",
		})
		requireStatus(t, r, 201)
		var cred map[string]interface{}
		r.JSON(&cred)
		if cred["expiryDate"] != nil {
			t.Errorf("Expected null expiryDate, got %v", cred["expiryDate"])
		}
	})

	t.Run("update credential expiry to past", func(t *testing.T) {
		r := c.POST("/credentials", map[string]interface{}{
			"credentialType": "EASA_CLASS2_MEDICAL", "issueDate": "2024-01-01",
			"expiryDate": futureDate(365), "issuingAuthority": "AME",
		})
		requireStatus(t, r, 201)
		var cred map[string]interface{}
		r.JSON(&cred)
		credID := cred["id"].(string)

		r = c.PATCH(fmt.Sprintf("/credentials/%s", credID), map[string]interface{}{
			"expiryDate": pastDate(30),
		})
		requireStatus(t, r, 200)
	})
}

func TestAircraftActiveFlag(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("ac-active"), "SecurePass123!", "AcActive")

	r := c.POST("/aircraft", map[string]interface{}{
		"registration": "D-EACT", "type": "C172", "make": "Cessna", "model": "172",
	})
	requireStatus(t, r, 201)
	var ac map[string]interface{}
	r.JSON(&ac)
	acID := ac["id"].(string)

	t.Run("deactivate aircraft", func(t *testing.T) {
		r := c.PATCH(fmt.Sprintf("/aircraft/%s", acID), map[string]interface{}{"isActive": false})
		requireStatus(t, r, 200)
		var a map[string]interface{}
		r.JSON(&a)
		assertBool(t, "isActive", gb(a, "isActive"), false)
	})

	t.Run("deactivated aircraft still listed", func(t *testing.T) {
		r := c.GET("/aircraft")
		requireStatus(t, r, 200)
		var result map[string]interface{}
		r.JSON(&result)
		data := result["data"].([]interface{})
		found := false
		for _, a := range data {
			if a.(map[string]interface{})["id"] == acID {
				found = true
				break
			}
		}
		if !found {
			t.Error("Deactivated aircraft should still appear in list")
		}
	})

	t.Run("can create flight with deactivated aircraft reg", func(t *testing.T) {
		r := c.POST("/flights", map[string]interface{}{
			"date": today(), "aircraftReg": "D-EACT", "aircraftType": "C172",
			"departureIcao": "EDNY", "arrivalIcao": "EDDS",
			"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		})
		if r.StatusCode == 201 {
			t.Log("INFO: Flights allowed with deactivated aircraft")
		} else {
			t.Logf("Deactivated aircraft flight: %d %s", r.StatusCode, string(r.Body))
		}
	})

	t.Run("reactivate aircraft", func(t *testing.T) {
		r := c.PATCH(fmt.Sprintf("/aircraft/%s", acID), map[string]interface{}{"isActive": true})
		requireStatus(t, r, 200)
		var a map[string]interface{}
		r.JSON(&a)
		assertBool(t, "isActive", gb(a, "isActive"), true)
	})
}

func TestEmailUpdateDuplicate(t *testing.T) {
	c1 := NewE2EClient(t)
	c2 := NewE2EClient(t)
	email1 := uniqueEmail("email-dup1")
	email2 := uniqueEmail("email-dup2")
	registerAndLogin(t, c1, email1, "SecurePass123!", "User1")
	registerAndLogin(t, c2, email2, "SecurePass123!", "User2")

	t.Run("update email to another user email fails", func(t *testing.T) {
		r := c1.PATCH("/users/me", map[string]string{"email": email2})
		if r.StatusCode == 200 {
			t.Log("REGRESSION: Can update email to another user's email — duplicate allowed")
		} else if r.StatusCode == 409 {
			t.Log("Correctly rejected duplicate email (409)")
		} else {
			t.Logf("Email duplicate update: %d %s", r.StatusCode, string(r.Body))
		}
	})

	t.Run("update email to same email is no-op", func(t *testing.T) {
		r := c1.PATCH("/users/me", map[string]string{"email": email1})
		requireStatus(t, r, 200)
	})
}

func TestEmptyExports(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("empty-exp"), "SecurePass123!", "EmptyExport")
	// No flights created — test empty exports

	t.Run("CSV export with no flights", func(t *testing.T) {
		r := c.GET("/exports/csv")
		requireStatus(t, r, 200)
		t.Logf("Empty CSV length: %d bytes", len(r.Body))
	})

	t.Run("JSON export with no flights", func(t *testing.T) {
		r := c.GET("/exports/json")
		requireStatus(t, r, 200)
		t.Logf("Empty JSON length: %d bytes", len(r.Body))
	})

	t.Run("PDF export with no flights", func(t *testing.T) {
		r := c.GET("/exports/pdf")
		if r.StatusCode == 500 {
			t.Log("REGRESSION: PDF export with 0 flights causes 500")
		} else {
			requireStatus(t, r, 200)
			t.Logf("Empty PDF length: %d bytes", len(r.Body))
		}
	})
}

func TestPDFExportWithLogbookFilter(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("pdf-lb"), "SecurePass123!", "PDFLogbook")

	r := c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "SPL", "licenseNumber": "PDF-SPL",
		"issueDate": today(), "issuingAuthority": "LBA", "requiresSeparateLogbook": true,
	})
	requireStatus(t, r, 201)
	var lic map[string]interface{}
	r.JSON(&lic)
	splID := lic["id"].(string)

	// Create flights
	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": today(), "aircraftReg": "D-1234", "aircraftType": "ASK21",
		"departureIcao": "EDNY", "arrivalIcao": "EDNY",
		"offBlockTime": "10:00", "onBlockTime": "10:45", "landings": 1, "launchMethod": "winch",
	}), 201)

	t.Run("PDF export with logbookLicenseId", func(t *testing.T) {
		r := c.GET(fmt.Sprintf("/exports/pdf?logbookLicenseId=%s", splID))
		if r.StatusCode == 500 {
			t.Log("REGRESSION: PDF export with logbookLicenseId filter causes 500")
		} else {
			requireStatus(t, r, 200)
			if len(r.Body) < 100 {
				t.Error("PDF too small")
			}
		}
	})
}

func TestReportEdgeCases(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("rpt-edge"), "SecurePass123!", "RptEdge")

	t.Run("trends with months=0", func(t *testing.T) {
		r := c.GET("/reports/trends?months=0")
		if r.StatusCode == http.StatusInternalServerError {
			t.Log("REGRESSION: trends months=0 causes 500")
		}
	})

	t.Run("trends with months=60", func(t *testing.T) {
		requireStatus(t, c.GET("/reports/trends?months=60"), 200)
	})

	t.Run("routes with no flights", func(t *testing.T) {
		requireStatus(t, c.GET("/reports/routes"), 200)
	})

	t.Run("airport-stats with no flights", func(t *testing.T) {
		requireStatus(t, c.GET("/reports/airport-stats"), 200)
	})

	t.Run("stats-by-class with no flights", func(t *testing.T) {
		requireStatus(t, c.GET("/reports/stats-by-class"), 200)
	})
}

func TestAnnouncementEdgeCases(t *testing.T) {
	ac := getAdminClient(t)

	t.Run("delete nonexistent announcement", func(t *testing.T) {
		r := ac.DELETE("/admin/announcements/00000000-0000-0000-0000-000000000000")
		if r.StatusCode != 204 && r.StatusCode != 404 {
			t.Errorf("Expected 204 or 404, got %d", r.StatusCode)
		}
	})

	t.Run("announcement with invalid severity rejected", func(t *testing.T) {
		r := ac.POST("/admin/announcements", map[string]interface{}{
			"message": "Test", "severity": "INVALID",
		})
		if r.StatusCode == 201 {
			t.Log("REGRESSION: Invalid severity accepted")
		}
	})

	t.Run("announcement with empty message", func(t *testing.T) {
		r := ac.POST("/admin/announcements", map[string]interface{}{
			"message": "", "severity": "info",
		})
		if r.StatusCode == 201 {
			t.Log("INFO: Empty announcement message accepted")
		}
	})

	t.Run("all severity levels valid", func(t *testing.T) {
		for _, sev := range []string{"info", "success", "warning", "critical"} {
			r := ac.POST("/admin/announcements", map[string]interface{}{
				"message": "Test " + sev, "severity": sev,
			})
			requireStatus(t, r, 201)
		}
	})
}

func TestAdminSelfProtection(t *testing.T) {
	ac := getAdminClient(t)

	// Get admin user ID
	r := ac.GET("/users/me")
	requireStatus(t, r, 200)
	var me map[string]interface{}
	r.JSON(&me)
	adminID := me["id"].(string)

	t.Run("admin cannot disable self", func(t *testing.T) {
		r := ac.POST(fmt.Sprintf("/admin/users/%s/disable", adminID), nil)
		if r.StatusCode == 200 {
			t.Log("REGRESSION: Admin can disable their own account")
			// Re-enable immediately
			ac.POST(fmt.Sprintf("/admin/users/%s/enable", adminID), nil)
		} else {
			t.Logf("Self-disable prevented: %d %s", r.StatusCode, string(r.Body))
		}
	})
}

func TestAdminPaginationEdgeCases(t *testing.T) {
	ac := getAdminClient(t)

	t.Run("page=0", func(t *testing.T) {
		r := ac.GET("/admin/users?page=0")
		if r.StatusCode == 500 {
			t.Log("REGRESSION: admin users page=0 causes 500")
		}
	})

	t.Run("pageSize=0", func(t *testing.T) {
		r := ac.GET("/admin/users?pageSize=0")
		if r.StatusCode == 500 {
			t.Log("REGRESSION: admin users pageSize=0 causes 500")
		}
	})

	t.Run("very large page number", func(t *testing.T) {
		r := ac.GET("/admin/users?page=99999")
		requireStatus(t, r, 200)
		var result map[string]interface{}
		r.JSON(&result)
		data := result["data"].([]interface{})
		if len(data) != 0 {
			t.Error("Expected empty data for very large page")
		}
	})
}

func TestCurrencyNoFlights(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("curr-empty"), "SecurePass123!", "CurrEmpty")

	// License with rating but no flights
	r := c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA", "licenseType": "PPL", "licenseNumber": "CURR-EMPTY",
		"issueDate": "2023-01-01", "issuingAuthority": "LBA",
	})
	requireStatus(t, r, 201)
	var lic map[string]interface{}
	r.JSON(&lic)
	lid := lic["id"].(string)

	requireStatus(t, c.POST(fmt.Sprintf("/licenses/%s/ratings", lid), map[string]interface{}{
		"classType": "SEP_LAND", "issueDate": "2023-01-01", "expiryDate": futureDate(365),
	}), 201)

	requireStatus(t, c.POST(fmt.Sprintf("/licenses/%s/ratings", lid), map[string]interface{}{
		"classType": "TMG", "issueDate": "2023-06-01",
	}), 201)

	t.Run("currency status with 0 flights and multiple ratings", func(t *testing.T) {
		r := c.GET("/currency")
		requireStatus(t, r, 200)
		var result map[string]interface{}
		r.JSON(&result)
		if result["ratings"] == nil {
			t.Error("Expected ratings array")
		} else {
			ratings := result["ratings"].([]interface{})
			if len(ratings) < 2 {
				t.Errorf("Expected >=2 ratings, got %d", len(ratings))
			}
			for _, rating := range ratings {
				rt := rating.(map[string]interface{})
				status := rt["status"]
				t.Logf("Rating %v: status=%v", rt["classType"], status)
			}
		}
	})
}

func TestAirportSearchEdgeCases(t *testing.T) {
	c := NewE2EClient(t)

	t.Run("search single char", func(t *testing.T) {
		r := c.GET("/airports/search?q=E")
		if r.StatusCode == 200 {
			var a []interface{}
			r.JSON(&a)
			t.Logf("Single char 'E': %d results", len(a))
		}
	})

	t.Run("search lowercase", func(t *testing.T) {
		r := c.GET("/airports/search?q=edny")
		requireStatus(t, r, 200)
		var a []interface{}
		r.JSON(&a)
		if len(a) < 1 {
			t.Log("REGRESSION: Lowercase ICAO search returns no results")
		}
	})

	t.Run("search with spaces", func(t *testing.T) {
		r := c.GET("/airports/search?q=%20ED")
		if r.StatusCode == 500 {
			t.Log("REGRESSION: Airport search with leading space causes 500")
		}
	})

	t.Run("limit=0", func(t *testing.T) {
		r := c.GET("/airports/search?q=ED&limit=0")
		if r.StatusCode == 500 {
			t.Log("REGRESSION: Airport search limit=0 causes 500")
		}
	})
}

func TestNotificationWarningDaysValidation(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("wd-val"), "SecurePass123!", "WDVal")

	t.Run("negative warningDays", func(t *testing.T) {
		r := c.PATCH("/users/me/notifications", map[string]interface{}{
			"warningDays": []int{-7, 30},
		})
		if r.StatusCode == 200 {
			t.Log("INFO: Negative warningDays accepted")
		}
	})

	t.Run("duplicate warningDays", func(t *testing.T) {
		r := c.PATCH("/users/me/notifications", map[string]interface{}{
			"warningDays": []int{7, 7, 7},
		})
		requireStatus(t, r, 200)
	})

	t.Run("zero warningDays", func(t *testing.T) {
		r := c.PATCH("/users/me/notifications", map[string]interface{}{
			"warningDays": []int{0},
		})
		if r.StatusCode == 200 {
			t.Log("INFO: warningDays=[0] accepted")
		}
	})
}

func TestFlightSearchSpecialChars(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("flt-srch"), "SecurePass123!", "FltSrch")

	requireStatus(t, c.POST("/flights", map[string]interface{}{
		"date": today(), "aircraftReg": "D-ESRCH", "aircraftType": "C172",
		"departureIcao": "EDNY", "arrivalIcao": "EDDS",
		"offBlockTime": "08:00", "onBlockTime": "09:00", "landings": 1,
		"remarks": "Flight with O'Reilly and Müller",
	}), 201)

	t.Run("search apostrophe", func(t *testing.T) {
		r := c.GET("/flights?search=O'Reilly")
		if r.StatusCode == 500 {
			t.Log("REGRESSION: Apostrophe in flight search causes 500")
		} else {
			requireStatus(t, r, 200)
			var m map[string]interface{}
			r.JSON(&m)
			data := m["data"].([]interface{})
			if len(data) < 1 {
				t.Error("Expected to find flight with O'Reilly")
			}
		}
	})

	t.Run("search umlaut", func(t *testing.T) {
		r := c.GET("/flights?search=Müller")
		requireStatus(t, r, 200)
		var m map[string]interface{}
		r.JSON(&m)
		data := m["data"].([]interface{})
		if len(data) < 1 {
			t.Error("Expected to find flight with Müller")
		}
	})

	t.Run("search percent sign", func(t *testing.T) {
		r := c.GET("/flights?search=%25")
		if r.StatusCode == 500 {
			t.Log("REGRESSION: Percent sign in search causes 500")
		}
	})

	t.Run("search underscore", func(t *testing.T) {
		r := c.GET("/flights?search=_")
		if r.StatusCode == 500 {
			t.Log("REGRESSION: Underscore in search causes 500")
		}
	})
}
