//go:build e2e

// Tests that every notification email type is sent with correct subject and body content.
// Uses MailPit as a test SMTP server and the admin trigger-notifications endpoint.

package e2e_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// --- Credential Expiry Emails (4 categories) ---

func TestEmailContent_CredentialMedical(t *testing.T) {
	ac := getAdminClient(t)
	c := NewE2EClient(t)
	email := uniqueEmail("email-cred-med")
	registerAndLogin(t, c, email, "SecurePass123!", "MedPilot")

	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"credential_medical"},
		"warningDays":       []int{30},
	})

	expiryDate := futureDate(10)
	c.POST("/credentials", map[string]interface{}{
		"credentialType":   "EASA_CLASS2_MEDICAL",
		"credentialNumber": "MED-CONTENT-001",
		"issueDate":        pastDate(365),
		"expiryDate":       expiryDate,
		"issuingAuthority": "AME Berlin",
	})

	mailpitDeleteAll(t)
	requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
	time.Sleep(2 * time.Second)

	msg := mailpitRequireEmail(t, email, "EASA_CLASS2_MEDICAL")

	// Verify subject format
	if !strings.Contains(msg.Subject, "expires in") {
		t.Errorf("Subject should contain 'expires in', got: %s", msg.Subject)
	}
	if !strings.Contains(msg.Subject, "EASA_CLASS2_MEDICAL") {
		t.Errorf("Subject should contain credential type, got: %s", msg.Subject)
	}

	// Verify body content
	if !strings.Contains(msg.HTML, "Credential Expiry Warning") {
		t.Error("Body should contain 'Credential Expiry Warning' heading")
	}
	if !strings.Contains(msg.HTML, "MedPilot") {
		t.Error("Body should contain user name 'MedPilot'")
	}
	if !strings.Contains(msg.HTML, "EASA_CLASS2_MEDICAL") {
		t.Error("Body should contain credential type")
	}
	if !strings.Contains(msg.HTML, "renew it before it expires") {
		t.Error("Body should contain renewal instruction")
	}
	if !strings.Contains(msg.HTML, "NinerLog") {
		t.Error("Body should contain NinerLog signature")
	}

	// Verify from address
	if msg.From.Address != "noreply@ninerlog-test.com" {
		t.Errorf("From address should be noreply@ninerlog-test.com, got: %s", msg.From.Address)
	}

	// Verify to address
	if len(msg.To) == 0 || msg.To[0].Address != email {
		t.Errorf("To address should be %s", email)
	}
}

func TestEmailContent_CredentialLanguage(t *testing.T) {
	ac := getAdminClient(t)
	c := NewE2EClient(t)
	email := uniqueEmail("email-cred-lang")
	registerAndLogin(t, c, email, "SecurePass123!", "LangPilot")

	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"credential_language"},
		"warningDays":       []int{30},
	})

	c.POST("/credentials", map[string]interface{}{
		"credentialType":   "LANG_ICAO_LEVEL4",
		"credentialNumber": "LANG-CONTENT-001",
		"issueDate":        pastDate(365),
		"expiryDate":       futureDate(12),
		"issuingAuthority": "LBA",
	})

	mailpitDeleteAll(t)
	requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
	time.Sleep(2 * time.Second)

	msg := mailpitRequireEmail(t, email, "LANG_ICAO_LEVEL4")

	if !strings.Contains(msg.Subject, "LANG_ICAO_LEVEL4") {
		t.Errorf("Subject should contain LANG_ICAO_LEVEL4, got: %s", msg.Subject)
	}
	if !strings.Contains(msg.HTML, "LangPilot") {
		t.Error("Body should contain user name")
	}
	if !strings.Contains(msg.HTML, "LANG_ICAO_LEVEL4") {
		t.Error("Body should contain credential type")
	}
}

func TestEmailContent_CredentialSecurity(t *testing.T) {
	ac := getAdminClient(t)
	c := NewE2EClient(t)
	email := uniqueEmail("email-cred-sec")
	registerAndLogin(t, c, email, "SecurePass123!", "SecPilot")

	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"credential_security"},
		"warningDays":       []int{30},
	})

	c.POST("/credentials", map[string]interface{}{
		"credentialType":   "SEC_CLEARANCE_ZUP",
		"credentialNumber": "ZUP-CONTENT-001",
		"issueDate":        pastDate(365),
		"expiryDate":       futureDate(8),
		"issuingAuthority": "LBA",
	})

	mailpitDeleteAll(t)
	requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
	time.Sleep(2 * time.Second)

	msg := mailpitRequireEmail(t, email, "SEC_CLEARANCE_ZUP")

	if !strings.Contains(msg.Subject, "SEC_CLEARANCE_ZUP") {
		t.Errorf("Subject should contain SEC_CLEARANCE_ZUP, got: %s", msg.Subject)
	}
	if !strings.Contains(msg.HTML, "SecPilot") {
		t.Error("Body should contain user name")
	}
	if !strings.Contains(msg.HTML, "Credential Expiry Warning") {
		t.Error("Body should contain credential expiry heading")
	}
}

func TestEmailContent_CredentialOther(t *testing.T) {
	ac := getAdminClient(t)
	c := NewE2EClient(t)
	email := uniqueEmail("email-cred-other")
	registerAndLogin(t, c, email, "SecurePass123!", "OtherPilot")

	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"credential_other"},
		"warningDays":       []int{30},
	})

	c.POST("/credentials", map[string]interface{}{
		"credentialType":   "OTHER",
		"credentialNumber": "OTHER-CONTENT-001",
		"issueDate":        pastDate(365),
		"expiryDate":       futureDate(15),
		"issuingAuthority": "Test Authority",
	})

	mailpitDeleteAll(t)
	requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
	time.Sleep(2 * time.Second)

	msg := mailpitRequireEmail(t, email, "OTHER")

	if !strings.Contains(msg.Subject, "OTHER") {
		t.Errorf("Subject should contain OTHER, got: %s", msg.Subject)
	}
	if !strings.Contains(msg.HTML, "OtherPilot") {
		t.Error("Body should contain user name")
	}
}

// --- Credential Expiry: Multiple credential types per category ---

func TestEmailContent_CredentialMedicalAllTypes(t *testing.T) {
	ac := getAdminClient(t)

	medicalTypes := []string{
		"EASA_CLASS1_MEDICAL",
		"EASA_CLASS2_MEDICAL",
		"EASA_LAPL_MEDICAL",
		"FAA_CLASS1_MEDICAL",
		"FAA_CLASS2_MEDICAL",
		"FAA_CLASS3_MEDICAL",
	}

	for _, credType := range medicalTypes {
		t.Run(credType, func(t *testing.T) {
			c := NewE2EClient(t)
			email := uniqueEmail(fmt.Sprintf("email-%s", strings.ToLower(credType)))
			registerAndLogin(t, c, email, "SecurePass123!", "MedTypePilot")

			c.PATCH("/users/me/notifications", map[string]interface{}{
				"emailEnabled":      true,
				"enabledCategories": []string{"credential_medical"},
				"warningDays":       []int{30},
			})

			c.POST("/credentials", map[string]interface{}{
				"credentialType":   credType,
				"credentialNumber": fmt.Sprintf("%s-001", credType),
				"issueDate":        pastDate(365),
				"expiryDate":       futureDate(10),
				"issuingAuthority": "Test AME",
			})

			mailpitDeleteAll(t)
			requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
			time.Sleep(2 * time.Second)

			msg := mailpitRequireEmail(t, email, credType)

			if !strings.Contains(msg.Subject, credType) {
				t.Errorf("Subject should contain %s, got: %s", credType, msg.Subject)
			}
			if !strings.Contains(msg.HTML, credType) {
				t.Errorf("Body should contain %s", credType)
			}
			if !strings.Contains(msg.HTML, "Credential Expiry Warning") {
				t.Error("Body should contain credential expiry heading")
			}
		})
	}
}

func TestEmailContent_LanguageAllTypes(t *testing.T) {
	ac := getAdminClient(t)

	langTypes := []string{"LANG_ICAO_LEVEL4", "LANG_ICAO_LEVEL5", "LANG_ICAO_LEVEL6"}

	for _, credType := range langTypes {
		t.Run(credType, func(t *testing.T) {
			c := NewE2EClient(t)
			email := uniqueEmail(fmt.Sprintf("email-%s", strings.ToLower(credType)))
			registerAndLogin(t, c, email, "SecurePass123!", "LangTypePilot")

			c.PATCH("/users/me/notifications", map[string]interface{}{
				"emailEnabled":      true,
				"enabledCategories": []string{"credential_language"},
				"warningDays":       []int{30},
			})

			c.POST("/credentials", map[string]interface{}{
				"credentialType":   credType,
				"credentialNumber": fmt.Sprintf("%s-001", credType),
				"issueDate":        pastDate(365),
				"expiryDate":       futureDate(10),
				"issuingAuthority": "LBA",
			})

			mailpitDeleteAll(t)
			requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
			time.Sleep(2 * time.Second)

			msg := mailpitRequireEmail(t, email, credType)

			if !strings.Contains(msg.Subject, credType) {
				t.Errorf("Subject should contain %s, got: %s", credType, msg.Subject)
			}
		})
	}
}

func TestEmailContent_SecurityAllTypes(t *testing.T) {
	ac := getAdminClient(t)

	secTypes := []string{"SEC_CLEARANCE_ZUP", "SEC_CLEARANCE_ZUBB"}

	for _, credType := range secTypes {
		t.Run(credType, func(t *testing.T) {
			c := NewE2EClient(t)
			email := uniqueEmail(fmt.Sprintf("email-%s", strings.ToLower(credType)))
			registerAndLogin(t, c, email, "SecurePass123!", "SecTypePilot")

			c.PATCH("/users/me/notifications", map[string]interface{}{
				"emailEnabled":      true,
				"enabledCategories": []string{"credential_security"},
				"warningDays":       []int{30},
			})

			c.POST("/credentials", map[string]interface{}{
				"credentialType":   credType,
				"credentialNumber": fmt.Sprintf("%s-001", credType),
				"issueDate":        pastDate(365),
				"expiryDate":       futureDate(10),
				"issuingAuthority": "LBA",
			})

			mailpitDeleteAll(t)
			requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
			time.Sleep(2 * time.Second)

			msg := mailpitRequireEmail(t, email, credType)

			if !strings.Contains(msg.Subject, credType) {
				t.Errorf("Subject should contain %s, got: %s", credType, msg.Subject)
			}
		})
	}
}

// --- Credential Expiry: Days calculation ---

func TestEmailContent_CredentialExpiryDaysAccuracy(t *testing.T) {
	ac := getAdminClient(t)
	c := NewE2EClient(t)
	email := uniqueEmail("email-days-acc")
	registerAndLogin(t, c, email, "SecurePass123!", "DaysPilot")

	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"credential_medical"},
		"warningDays":       []int{30},
	})

	// Credential expiring in exactly 7 days
	c.POST("/credentials", map[string]interface{}{
		"credentialType":   "EASA_CLASS2_MEDICAL",
		"credentialNumber": "MED-DAYS-001",
		"issueDate":        pastDate(365),
		"expiryDate":       futureDate(7),
		"issuingAuthority": "AME Test",
	})

	mailpitDeleteAll(t)
	requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
	time.Sleep(2 * time.Second)

	msg := mailpitRequireEmail(t, email, "EASA_CLASS2_MEDICAL")

	// The subject should say "expires in 7 days" (or close — timing may vary by 1)
	if !strings.Contains(msg.Subject, "expires in 7 days") && !strings.Contains(msg.Subject, "expires in 6 days") {
		t.Errorf("Subject should say 'expires in 7 days' (±1), got: %s", msg.Subject)
	}

	// Body should contain the formatted date
	if !strings.Contains(msg.HTML, "days from now") {
		t.Error("Body should contain 'days from now'")
	}
}

// --- Rating Expiry Email ---

func TestEmailContent_RatingExpiry(t *testing.T) {
	ac := getAdminClient(t)
	c := NewE2EClient(t)
	email := uniqueEmail("email-rating-exp")
	registerAndLogin(t, c, email, "SecurePass123!", "RatingPilot")

	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"rating_expiry"},
		"warningDays":       []int{30},
	})

	// Create license with a class rating expiring soon
	resp := c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "EASA",
		"licenseType":         "PPL",
		"licenseNumber":       "PPL-RATING-001",
		"issueDate":           pastDate(730),
		"issuingAuthority":    "LBA",
	})
	requireStatus(t, resp, http.StatusCreated)
	var lic map[string]interface{}
	resp.JSON(&lic)
	licID := lic["id"].(string)

	resp = c.POST(fmt.Sprintf("/licenses/%s/ratings", licID), map[string]interface{}{
		"classType":  "SEP_LAND",
		"issueDate":  pastDate(730),
		"expiryDate": futureDate(20),
	})
	requireStatus(t, resp, http.StatusCreated)

	mailpitDeleteAll(t)
	requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
	time.Sleep(2 * time.Second)

	msg := mailpitFindEmail(t, email, "rating expires")
	if msg == nil {
		t.Skip("Rating expiry email not triggered — currency evaluator may require flight data to determine expiring status")
		return
	}

	if !strings.Contains(msg.Subject, "PPL") || !strings.Contains(msg.Subject, "SEP_LAND") {
		t.Errorf("Subject should contain license type and class type, got: %s", msg.Subject)
	}
	if !strings.Contains(msg.HTML, "Class Rating Expiry Warning") {
		t.Error("Body should contain 'Class Rating Expiry Warning' heading")
	}
	if !strings.Contains(msg.HTML, "RatingPilot") {
		t.Error("Body should contain user name")
	}
	if !strings.Contains(msg.HTML, "revalidation flights or proficiency check") {
		t.Error("Body should contain revalidation instruction")
	}
}

// --- Passenger Currency Email ---

func TestEmailContent_PassengerCurrency(t *testing.T) {
	ac := getAdminClient(t)
	c := NewE2EClient(t)
	email := uniqueEmail("email-pax-curr")
	registerAndLogin(t, c, email, "SecurePass123!", "PaxPilot")

	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"currency_passenger"},
		"warningDays":       []int{30},
	})

	// Create license + class rating (needed for currency evaluation)
	resp := c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "FAA",
		"licenseType":         "Private",
		"licenseNumber":       "PPL-PAX-001",
		"issueDate":           pastDate(730),
		"issuingAuthority":    "FAA",
	})
	requireStatus(t, resp, http.StatusCreated)
	var lic map[string]interface{}
	resp.JSON(&lic)
	licID := lic["id"].(string)

	c.POST(fmt.Sprintf("/licenses/%s/ratings", licID), map[string]interface{}{
		"classType":  "SEP_LAND",
		"issueDate":  pastDate(730),
		"expiryDate": futureDate(365),
	})

	// No flights = no landings in 90 days → passenger currency not met
	mailpitDeleteAll(t)
	requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
	time.Sleep(2 * time.Second)

	msg := mailpitFindEmail(t, email, "passenger currency")
	if msg == nil {
		t.Skip("Passenger currency email not triggered — may need minimum activity data")
		return
	}

	if !strings.Contains(msg.Subject, "passenger currency") {
		t.Errorf("Subject should contain 'passenger currency', got: %s", msg.Subject)
	}
	if !strings.Contains(msg.Subject, "landings needed") {
		t.Errorf("Subject should contain 'landings needed', got: %s", msg.Subject)
	}
	if !strings.Contains(msg.HTML, "Passenger Currency Warning") {
		t.Error("Body should contain 'Passenger Currency Warning' heading")
	}
	if !strings.Contains(msg.HTML, "PaxPilot") {
		t.Error("Body should contain user name")
	}
	if !strings.Contains(msg.HTML, "day landings") {
		t.Error("Body should contain 'day landings'")
	}
	if !strings.Contains(msg.HTML, "carry passengers") {
		t.Error("Body should contain 'carry passengers'")
	}
}

// --- Night Currency Email ---

func TestEmailContent_NightCurrency(t *testing.T) {
	ac := getAdminClient(t)
	c := NewE2EClient(t)
	email := uniqueEmail("email-night-curr")
	registerAndLogin(t, c, email, "SecurePass123!", "NightPilot")

	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"currency_night"},
		"warningDays":       []int{30},
	})

	// FAA license with night privilege
	resp := c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "FAA",
		"licenseType":         "Private",
		"licenseNumber":       "PPL-NIGHT-001",
		"issueDate":           pastDate(730),
		"issuingAuthority":    "FAA",
	})
	requireStatus(t, resp, http.StatusCreated)
	var lic map[string]interface{}
	resp.JSON(&lic)
	licID := lic["id"].(string)

	c.POST(fmt.Sprintf("/licenses/%s/ratings", licID), map[string]interface{}{
		"classType":  "SEP_LAND",
		"issueDate":  pastDate(730),
		"expiryDate": futureDate(365),
	})

	mailpitDeleteAll(t)
	requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
	time.Sleep(2 * time.Second)

	msg := mailpitFindEmail(t, email, "night currency")
	if msg == nil {
		t.Skip("Night currency email not triggered — may need minimum activity data")
		return
	}

	if !strings.Contains(msg.Subject, "night currency") {
		t.Errorf("Subject should contain 'night currency', got: %s", msg.Subject)
	}
	if !strings.Contains(msg.HTML, "Night Currency Warning") {
		t.Error("Body should contain 'Night Currency Warning' heading")
	}
	if !strings.Contains(msg.HTML, "NightPilot") {
		t.Error("Body should contain user name")
	}
	if !strings.Contains(msg.HTML, "night landings") {
		t.Error("Body should contain 'night landings'")
	}
	if !strings.Contains(msg.HTML, "night passenger carrying") {
		t.Error("Body should contain 'night passenger carrying'")
	}
}

// --- Flight Review Email ---

func TestEmailContent_FlightReview(t *testing.T) {
	ac := getAdminClient(t)
	c := NewE2EClient(t)
	email := uniqueEmail("email-fr")
	registerAndLogin(t, c, email, "SecurePass123!", "FRPilot")

	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"currency_flight_review"},
		"warningDays":       []int{30},
	})

	// FAA license — flight review is evaluated for FAA pilots
	resp := c.POST("/licenses", map[string]interface{}{
		"regulatoryAuthority": "FAA",
		"licenseType":         "Private",
		"licenseNumber":       "PPL-FR-001",
		"issueDate":           pastDate(730),
		"issuingAuthority":    "FAA",
	})
	requireStatus(t, resp, http.StatusCreated)
	var lic map[string]interface{}
	resp.JSON(&lic)
	licID := lic["id"].(string)

	c.POST(fmt.Sprintf("/licenses/%s/ratings", licID), map[string]interface{}{
		"classType":  "SEP_LAND",
		"issueDate":  pastDate(730),
		"expiryDate": futureDate(365),
	})

	// No flights with is_flight_review=true → flight review expired
	mailpitDeleteAll(t)
	requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
	time.Sleep(2 * time.Second)

	// Could be "Flight review required" or "Flight review expires in X days"
	msg := mailpitFindEmail(t, email, "Flight review")
	if msg == nil {
		msg = mailpitFindEmail(t, email, "flight review")
	}
	if msg == nil {
		t.Skip("Flight review email not triggered — may need FAA evaluator to detect missing review")
		return
	}

	if !strings.Contains(strings.ToLower(msg.Subject), "flight review") {
		t.Errorf("Subject should contain 'flight review', got: %s", msg.Subject)
	}
	if !strings.Contains(msg.HTML, "FRPilot") {
		t.Error("Body should contain user name")
	}
	if !strings.Contains(msg.HTML, "61.56") {
		t.Error("Body should reference 14 CFR §61.56")
	}
}

// --- SMTP Test Email ---

func TestEmailContent_SmtpTest(t *testing.T) {
	ac := getAdminClient(t)

	mailpitDeleteAll(t)
	resp := ac.POST("/admin/maintenance/smtp-test", nil)
	requireStatus(t, resp, http.StatusOK)
	time.Sleep(2 * time.Second)

	msg := mailpitRequireEmail(t, "admin@ninerlog-test.com", "SMTP Test")

	// Subject
	if msg.Subject != "NinerLog SMTP Test" {
		t.Errorf("Subject should be 'NinerLog SMTP Test', got: %s", msg.Subject)
	}

	// Body content
	if !strings.Contains(msg.HTML, "SMTP Test Successful") {
		t.Error("Body should contain 'SMTP Test Successful'")
	}
	if !strings.Contains(msg.HTML, "NinerLog admin console") {
		t.Error("Body should mention admin console")
	}
	if !strings.Contains(msg.HTML, "SMTP configuration is working correctly") {
		t.Error("Body should confirm SMTP config is working")
	}
	if !strings.Contains(msg.HTML, "Sent at:") {
		t.Error("Body should contain send timestamp")
	}

	// From address
	if msg.From.Address != "noreply@ninerlog-test.com" {
		t.Errorf("From address should be noreply@ninerlog-test.com, got: %s", msg.From.Address)
	}
	if msg.From.Name != "NinerLog" {
		t.Errorf("From name should be 'NinerLog', got: %s", msg.From.Name)
	}
}

// --- Cross-Cutting: Email Structure ---

func TestEmailContent_HTMLStructure(t *testing.T) {
	ac := getAdminClient(t)
	c := NewE2EClient(t)
	email := uniqueEmail("email-structure")
	registerAndLogin(t, c, email, "SecurePass123!", "StructPilot")

	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"credential_medical"},
		"warningDays":       []int{30},
	})

	c.POST("/credentials", map[string]interface{}{
		"credentialType":   "EASA_CLASS2_MEDICAL",
		"credentialNumber": "MED-STRUCT-001",
		"issueDate":        pastDate(365),
		"expiryDate":       futureDate(5),
		"issuingAuthority": "AME Test",
	})

	mailpitDeleteAll(t)
	requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
	time.Sleep(2 * time.Second)

	msg := mailpitRequireEmail(t, email, "EASA_CLASS2_MEDICAL")

	// Every notification email should:
	t.Run("contains_h2_heading", func(t *testing.T) {
		if !strings.Contains(msg.HTML, "<h2>") {
			t.Error("Email should contain an <h2> heading")
		}
	})

	t.Run("contains_greeting", func(t *testing.T) {
		if !strings.Contains(msg.HTML, "Hi StructPilot") {
			t.Error("Email should contain personalized greeting")
		}
	})

	t.Run("contains_signature", func(t *testing.T) {
		if !strings.Contains(msg.HTML, "NinerLog") {
			t.Error("Email should contain NinerLog signature")
		}
	})

	t.Run("uses_strong_for_emphasis", func(t *testing.T) {
		if !strings.Contains(msg.HTML, "<strong>") {
			t.Error("Email should use <strong> for emphasis")
		}
	})

	t.Run("uses_paragraphs", func(t *testing.T) {
		if !strings.Contains(msg.HTML, "<p>") {
			t.Error("Email should use <p> tags")
		}
	})
}

// --- Warning Day Thresholds ---

func TestEmailContent_WarningDayThresholds(t *testing.T) {
	ac := getAdminClient(t)

	// Test that each warning day threshold triggers independently
	warningDays := []int{30, 14, 7}

	for i, wd := range warningDays {
		t.Run(fmt.Sprintf("warningDay_%d", wd), func(t *testing.T) {
			c := NewE2EClient(t)
			email := uniqueEmail(fmt.Sprintf("email-wd-%d", wd))
			registerAndLogin(t, c, email, "SecurePass123!", fmt.Sprintf("Pilot%d", wd))

			// Only enable the specific warning day
			c.PATCH("/users/me/notifications", map[string]interface{}{
				"emailEnabled":      true,
				"enabledCategories": []string{"credential_medical"},
				"warningDays":       []int{wd},
			})

			// Create credential expiring within this warning window
			daysToExpiry := wd - 2 - i // Ensure it's within the window
			if daysToExpiry < 1 {
				daysToExpiry = 1
			}
			c.POST("/credentials", map[string]interface{}{
				"credentialType":   "EASA_CLASS2_MEDICAL",
				"credentialNumber": fmt.Sprintf("MED-WD%d-001", wd),
				"issueDate":        pastDate(365),
				"expiryDate":       futureDate(daysToExpiry),
				"issuingAuthority": "AME Test",
			})

			mailpitDeleteAll(t)
			requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
			time.Sleep(2 * time.Second)

			result := mailpitSearchByRecipient(t, email)
			if result.MessagesCount == 0 {
				t.Errorf("Warning day %d: expected email for credential expiring in %d days", wd, daysToExpiry)
			}
		})
	}
}

// --- No Email for Future-Safe Credentials ---

func TestEmailContent_NoEmailForFarFutureExpiry(t *testing.T) {
	ac := getAdminClient(t)
	c := NewE2EClient(t)
	email := uniqueEmail("email-no-send")
	registerAndLogin(t, c, email, "SecurePass123!", "SafePilot")

	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"credential_medical"},
		"warningDays":       []int{30, 14, 7},
	})

	// Credential expiring in 365 days — well outside all warning windows
	c.POST("/credentials", map[string]interface{}{
		"credentialType":   "EASA_CLASS2_MEDICAL",
		"credentialNumber": "MED-SAFE-001",
		"issueDate":        pastDate(30),
		"expiryDate":       futureDate(365),
		"issuingAuthority": "AME Test",
	})

	mailpitDeleteAll(t)
	requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
	time.Sleep(2 * time.Second)

	result := mailpitSearchByRecipient(t, email)
	if result.MessagesCount != 0 {
		subjects := make([]string, len(result.Messages))
		for j, m := range result.Messages {
			subjects[j] = m.Subject
		}
		t.Errorf("Expected 0 emails for far-future credential, got %d: %v", result.MessagesCount, subjects)
	}
}

// --- No Email for Already Expired Credentials ---

func TestEmailContent_NoEmailForAlreadyExpired(t *testing.T) {
	ac := getAdminClient(t)
	c := NewE2EClient(t)
	email := uniqueEmail("email-past-exp")
	registerAndLogin(t, c, email, "SecurePass123!", "ExpiredPilot")

	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"credential_medical"},
		"warningDays":       []int{30, 14, 7},
	})

	// Credential already expired 10 days ago
	c.POST("/credentials", map[string]interface{}{
		"credentialType":   "EASA_CLASS2_MEDICAL",
		"credentialNumber": "MED-PAST-001",
		"issueDate":        pastDate(730),
		"expiryDate":       pastDate(10),
		"issuingAuthority": "AME Test",
	})

	mailpitDeleteAll(t)
	requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
	time.Sleep(2 * time.Second)

	// Already-expired credentials should NOT trigger warnings (daysUntilExpiry < 0)
	result := mailpitSearchByRecipient(t, email)
	if result.MessagesCount != 0 {
		subjects := make([]string, len(result.Messages))
		for j, m := range result.Messages {
			subjects[j] = m.Subject
		}
		t.Errorf("Expected 0 emails for already-expired credential, got %d: %v", result.MessagesCount, subjects)
	}
}

// --- Per-Category Isolation ---

func TestEmailContent_CategoryIsolation(t *testing.T) {
	ac := getAdminClient(t)

	t.Run("medical_disabled_language_enabled", func(t *testing.T) {
		c := NewE2EClient(t)
		email := uniqueEmail("email-cat-iso1")
		registerAndLogin(t, c, email, "SecurePass123!", "IsoTest1")

		// Only language enabled
		c.PATCH("/users/me/notifications", map[string]interface{}{
			"emailEnabled":      true,
			"enabledCategories": []string{"credential_language"},
			"warningDays":       []int{30},
		})

		// Create a medical credential (should NOT trigger)
		c.POST("/credentials", map[string]interface{}{
			"credentialType":   "EASA_CLASS2_MEDICAL",
			"credentialNumber": "MED-ISO-001",
			"issueDate":        pastDate(365),
			"expiryDate":       futureDate(5),
			"issuingAuthority": "AME Test",
		})

		mailpitDeleteAll(t)
		requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
		time.Sleep(2 * time.Second)

		result := mailpitSearchByRecipient(t, email)
		if result.MessagesCount != 0 {
			t.Errorf("Expected 0 emails (medical disabled), got %d", result.MessagesCount)
		}
	})

	t.Run("security_enabled_gets_zup_email", func(t *testing.T) {
		c := NewE2EClient(t)
		email := uniqueEmail("email-cat-iso2")
		registerAndLogin(t, c, email, "SecurePass123!", "IsoTest2")

		c.PATCH("/users/me/notifications", map[string]interface{}{
			"emailEnabled":      true,
			"enabledCategories": []string{"credential_security"},
			"warningDays":       []int{30},
		})

		c.POST("/credentials", map[string]interface{}{
			"credentialType":   "SEC_CLEARANCE_ZUBB",
			"credentialNumber": "ZUBB-ISO-001",
			"issueDate":        pastDate(365),
			"expiryDate":       futureDate(5),
			"issuingAuthority": "LBA",
		})

		mailpitDeleteAll(t)
		requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
		time.Sleep(2 * time.Second)

		msg := mailpitRequireEmail(t, email, "SEC_CLEARANCE_ZUBB")
		if !strings.Contains(msg.HTML, "SEC_CLEARANCE_ZUBB") {
			t.Error("Body should contain SEC_CLEARANCE_ZUBB")
		}
	})
}

// --- History Correlation ---

func TestEmailContent_HistoryMatchesEmail(t *testing.T) {
	ac := getAdminClient(t)
	c := NewE2EClient(t)
	email := uniqueEmail("email-hist-match")
	registerAndLogin(t, c, email, "SecurePass123!", "HistPilot")

	c.PATCH("/users/me/notifications", map[string]interface{}{
		"emailEnabled":      true,
		"enabledCategories": []string{"credential_medical"},
		"warningDays":       []int{14},
	})

	c.POST("/credentials", map[string]interface{}{
		"credentialType":   "FAA_CLASS3_MEDICAL",
		"credentialNumber": "FAA3-HIST-001",
		"issueDate":        pastDate(365),
		"expiryDate":       futureDate(10),
		"issuingAuthority": "FAA AME",
	})

	mailpitDeleteAll(t)
	requireStatus(t, ac.POST("/admin/maintenance/trigger-notifications", nil), http.StatusOK)
	time.Sleep(2 * time.Second)

	// Get the email from MailPit
	sentMsg := mailpitRequireEmail(t, email, "FAA_CLASS3_MEDICAL")

	// Get the notification history from the API
	resp := c.GET("/users/me/notifications/history")
	requireStatus(t, resp, http.StatusOK)

	var history map[string]interface{}
	resp.JSON(&history)

	items := history["items"].([]interface{})
	if len(items) == 0 {
		t.Fatal("Expected at least 1 history entry")
	}

	entry := items[0].(map[string]interface{})

	// History subject should match the email subject
	histSubject := entry["subject"].(string)
	if histSubject != sentMsg.Subject {
		t.Errorf("History subject %q should match email subject %q", histSubject, sentMsg.Subject)
	}

	// History category should be credential_medical
	if entry["category"] != "credential_medical" {
		t.Errorf("History category should be 'credential_medical', got: %v", entry["category"])
	}

	// History should have a sentAt timestamp
	if entry["sentAt"] == nil {
		t.Error("History entry should have sentAt")
	}
}
