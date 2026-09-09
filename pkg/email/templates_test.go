package email

import (
	"strings"
	"testing"
)

func TestTemplates_EnglishDefault(t *testing.T) {
	ts := Templates("en")
	if ts.CredentialExpiry == nil {
		t.Error("English CredentialExpiry template is nil")
	}
}

func TestTemplates_German(t *testing.T) {
	ts := Templates("de")
	if ts.CredentialExpiry == nil {
		t.Error("German CredentialExpiry template is nil")
	}
}

func TestTemplates_UnknownLocaleFallsBackToEnglish(t *testing.T) {
	ts := Templates("fr")
	subj, _ := ts.CredentialExpiry(CredentialExpiryParams{
		UserName:       "Test",
		CredentialType: "Medical",
		ExpiryDate:     "2026-12-31",
		DaysRemaining:  30,
	})
	if !strings.Contains(subj, "expires") {
		t.Errorf("Fallback locale should produce English subject, got %q", subj)
	}
}

func TestCredentialExpiry_English(t *testing.T) {
	ts := Templates("en")
	subj, body := ts.CredentialExpiry(CredentialExpiryParams{
		UserName:       "Alice",
		CredentialType: "Medical Certificate",
		ExpiryDate:     "2026-06-15",
		DaysRemaining:  30,
	})

	if !strings.Contains(subj, "Medical Certificate") {
		t.Errorf("Subject should contain credential type, got %q", subj)
	}
	if !strings.Contains(subj, "30 days") {
		t.Errorf("Subject should contain days remaining, got %q", subj)
	}
	if !strings.Contains(body, "Alice") {
		t.Errorf("Body should contain user name, got %q", body)
	}
	if !strings.Contains(body, "2026-06-15") {
		t.Errorf("Body should contain expiry date, got %q", body)
	}
}

func TestCredentialExpiry_German(t *testing.T) {
	ts := Templates("de")
	subj, body := ts.CredentialExpiry(CredentialExpiryParams{
		UserName:       "Bob",
		CredentialType: "Tauglichkeitszeugnis",
		ExpiryDate:     "2026-06-15",
		DaysRemaining:  14,
	})

	if !strings.Contains(subj, "läuft") {
		t.Errorf("German subject should contain 'läuft', got %q", subj)
	}
	if !strings.Contains(body, "Hallo Bob") {
		t.Errorf("German body should contain 'Hallo Bob', got %q", body)
	}
}

func TestRatingExpiry_English(t *testing.T) {
	ts := Templates("en")
	subj, body := ts.RatingExpiry(RatingExpiryParams{
		UserName:      "Charlie",
		LicenseType:   "PPL",
		ClassType:     "SEP",
		ExpiryDate:    "2026-12-01",
		DaysRemaining: 60,
	})

	if !strings.Contains(subj, "PPL") || !strings.Contains(subj, "SEP") {
		t.Errorf("Subject should contain license/class type, got %q", subj)
	}
	if !strings.Contains(body, "Charlie") {
		t.Errorf("Body should contain user name, got %q", body)
	}
}

func TestRevalidation_English(t *testing.T) {
	ts := Templates("en")
	subj, body := ts.Revalidation(RevalidationParams{
		UserName:    "Dana",
		LicenseType: "PPL",
		ClassType:   "SEP",
		MessageKey:  "rating.revalidation_not_met",
	})

	if !strings.Contains(subj, "revalidation") {
		t.Errorf("Subject should contain 'revalidation', got %q", subj)
	}
	if !strings.Contains(body, "revalidation requirements are not fully met") {
		t.Errorf("Body should contain the rendered message, got %q", body)
	}
}

func TestPassengerCurrency_English(t *testing.T) {
	ts := Templates("en")
	subj, body := ts.PassengerCurrency(PassengerCurrencyParams{
		UserName:  "Eve",
		ClassType: "SEP",
		Landings:  1,
		Required:  3,
		Remaining: 2,
		Period:    "day",
	})

	if !strings.Contains(subj, "2 more landings") {
		t.Errorf("Subject should contain remaining landings, got %q", subj)
	}
	if !strings.Contains(body, "Eve") {
		t.Errorf("Body should contain user name, got %q", body)
	}
	if !strings.Contains(body, "90 days") {
		t.Errorf("Body should reference 90 day period, got %q", body)
	}
}

func TestPassengerCurrency_German_Night(t *testing.T) {
	ts := Templates("de")
	subj, body := ts.PassengerCurrency(PassengerCurrencyParams{
		UserName:  "Franz",
		ClassType: "SEP",
		Landings:  0,
		Required:  3,
		Remaining: 3,
		Period:    "night",
	})

	if !strings.Contains(subj, "Nacht") {
		t.Errorf("German night subject should contain 'Nacht', got %q", subj)
	}
	if !strings.Contains(body, "Nacht-Landungen") {
		t.Errorf("German body should contain 'Nacht-Landungen', got %q", body)
	}
}

func TestFlightReviewExpiry_English(t *testing.T) {
	ts := Templates("en")
	subj, body := ts.FlightReviewExpiry(FlightReviewExpiryParams{
		UserName:      "Frank",
		ExpiryDate:    "2026-09-01",
		DaysRemaining: 45,
	})

	if !strings.Contains(subj, "45 days") {
		t.Errorf("Subject should contain days remaining, got %q", subj)
	}
	if !strings.Contains(body, "14 CFR §61.56") {
		t.Errorf("Body should reference regulation, got %q", body)
	}
}

func TestFlightReviewRequired_English(t *testing.T) {
	ts := Templates("en")
	subj, body := ts.FlightReviewRequired(FlightReviewRequiredParams{
		UserName:      "Grace",
		MessageKey:    "flight_review.expired",
		MessageParams: CurrencyMessageParams{Date: strPtr("2026-01-15")},
	})

	if !strings.Contains(subj, "required") {
		t.Errorf("Subject should contain 'required', got %q", subj)
	}
	if !strings.Contains(body, "Grace") {
		t.Errorf("Body should contain user name, got %q", body)
	}
	if !strings.Contains(body, "flight review has expired") || !strings.Contains(body, "2026-01-15") {
		t.Errorf("Body should contain the rendered message, got %q", body)
	}
}

func TestFlightReviewExpiry_German(t *testing.T) {
	ts := Templates("de")
	subj, body := ts.FlightReviewExpiry(FlightReviewExpiryParams{
		UserName:      "Hans",
		ExpiryDate:    "2026-09-01",
		DaysRemaining: 30,
	})

	if !strings.Contains(subj, "Flugüberprüfung") {
		t.Errorf("German subject should contain 'Flugüberprüfung', got %q", subj)
	}
	if !strings.Contains(body, "Hallo Hans") {
		t.Errorf("German body should contain 'Hallo Hans', got %q", body)
	}
}

func TestFlightReviewRequired_German(t *testing.T) {
	ts := Templates("de")
	subj, body := ts.FlightReviewRequired(FlightReviewRequiredParams{
		UserName:      "Inge",
		MessageKey:    "flight_review.expired",
		MessageParams: CurrencyMessageParams{Date: strPtr("2026-01-15")},
	})

	if !strings.Contains(subj, "erforderlich") {
		t.Errorf("German subject should contain 'erforderlich', got %q", subj)
	}
	if !strings.Contains(body, "Hallo Inge") {
		t.Errorf("German body should contain 'Hallo Inge', got %q", body)
	}
}

func TestRevalidation_German(t *testing.T) {
	ts := Templates("de")
	subj, body := ts.Revalidation(RevalidationParams{
		UserName:    "Karl",
		LicenseType: "PPL",
		ClassType:   "SEP",
		MessageKey:  "rating.revalidation_not_met",
	})

	if !strings.Contains(subj, "Verlängerung") {
		t.Errorf("German subject should contain 'Verlängerung', got %q", subj)
	}
	if !strings.Contains(body, "Hallo Karl") {
		t.Errorf("German body should contain 'Hallo Karl', got %q", body)
	}
}

func TestRatingExpiry_German(t *testing.T) {
	ts := Templates("de")
	subj, body := ts.RatingExpiry(RatingExpiryParams{
		UserName:      "Lisa",
		LicenseType:   "PPL",
		ClassType:     "SEP",
		ExpiryDate:    "2026-12-01",
		DaysRemaining: 45,
	})

	if !strings.Contains(subj, "Berechtigung") {
		t.Errorf("German subject should contain 'Berechtigung', got %q", subj)
	}
	if !strings.Contains(body, "Hallo Lisa") {
		t.Errorf("German body should contain 'Hallo Lisa', got %q", body)
	}
}

func TestPassengerCurrency_German_Day(t *testing.T) {
	ts := Templates("de")
	subj, _ := ts.PassengerCurrency(PassengerCurrencyParams{
		UserName:  "Max",
		ClassType: "SEP",
		Landings:  2,
		Required:  3,
		Remaining: 1,
		Period:    "day",
	})

	if !strings.Contains(subj, "Tag") {
		t.Errorf("German day subject should contain 'Tag', got %q", subj)
	}
}

// TestTemplates_HTMLEscapesUserInput asserts user-controlled values are
// HTML-escaped in every email body.
func TestTemplates_HTMLEscapesUserInput(t *testing.T) {
	const payload = `<script>alert('xss')</script>`
	const escaped = `&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;`

	for _, locale := range []string{"en", "de"} {
		ts := Templates(locale)

		bodies := map[string]string{}
		_, bodies["CredentialExpiry"] = ts.CredentialExpiry(CredentialExpiryParams{UserName: payload, CredentialType: payload, ExpiryDate: payload})
		_, bodies["RatingExpiry"] = ts.RatingExpiry(RatingExpiryParams{UserName: payload, LicenseType: payload, ClassType: payload, ExpiryDate: payload})
		_, bodies["Revalidation"] = ts.Revalidation(RevalidationParams{UserName: payload, LicenseType: payload, ClassType: payload, MessageKey: payload})
		_, bodies["PassengerCurrency"] = ts.PassengerCurrency(PassengerCurrencyParams{UserName: payload, ClassType: payload, Period: "day"})
		_, bodies["FlightReviewExpiry"] = ts.FlightReviewExpiry(FlightReviewExpiryParams{UserName: payload, ExpiryDate: payload})
		_, bodies["FlightReviewRequired"] = ts.FlightReviewRequired(FlightReviewRequiredParams{UserName: payload, MessageKey: payload})
		_, bodies["VerifyEmail"] = ts.VerifyEmail(VerifyEmailParams{UserName: payload, Link: payload})

		for name, body := range bodies {
			if strings.Contains(body, payload) {
				t.Errorf("%s/%s body must not contain raw HTML payload, got %q", locale, name, body)
			}
			if !strings.Contains(body, escaped) {
				t.Errorf("%s/%s body should contain escaped payload, got %q", locale, name, body)
			}
		}
	}
}

// TestVerifyEmail_EscapesLinkAttribute asserts the verification link cannot
// break out of the href attribute.
func TestVerifyEmail_EscapesLinkAttribute(t *testing.T) {
	for _, locale := range []string{"en", "de"} {
		ts := Templates(locale)
		_, body := ts.VerifyEmail(VerifyEmailParams{
			UserName: "Mallory",
			Link:     `https://x/"><script>alert(1)</script>`,
		})
		if strings.Contains(body, `"><script>`) {
			t.Errorf("%s VerifyEmail must escape href attribute breakout, got %q", locale, body)
		}
	}
}

// TestCustomCurrency_EscapesMaliciousRuleName asserts the HTML body escapes
// the rule name and the subject carries no raw CR/LF.
func TestCustomCurrency_EscapesMaliciousRuleName(t *testing.T) {
	payload := `<script>alert('x')</script>`
	for _, locale := range []string{"en", "de"} {
		ts := Templates(locale)
		for _, expiring := range []bool{true, false} {
			subject, body := ts.CustomCurrency(CustomCurrencyParams{
				UserName:  "Pilot",
				RuleName:  payload,
				Expiring:  expiring,
				ExpiresOn: "2026-07-30",
			})
			if strings.Contains(body, "<script>") {
				t.Errorf("[%s expiring=%v] body must escape the raw script tag: %q", locale, expiring, body)
			}
			if !strings.Contains(body, "&lt;script&gt;") {
				t.Errorf("[%s expiring=%v] body should contain the escaped payload", locale, expiring)
			}
			if strings.ContainsAny(subject, "\r\n") {
				t.Errorf("[%s expiring=%v] subject must not contain CR/LF", locale, expiring)
			}
		}
	}
}

func strPtr(s string) *string { return &s }

// TestCurrencyMessagesAreLocalised pins the bug the message-key contract fixes:
// a German template must not render an English currency sentence.
func TestCurrencyMessagesAreLocalised(t *testing.T) {
	en := renderCurrencyMessage(currencyMessagesEN, "rating.revalidation_not_met", CurrencyMessageParams{})
	de := renderCurrencyMessage(currencyMessagesDE, "rating.revalidation_not_met", CurrencyMessageParams{})
	if en == de {
		t.Errorf("English and German render identically: %q", en)
	}
	if !strings.Contains(de, "Verlängerungsanforderungen") {
		t.Errorf("German rendering = %q, want German prose", de)
	}
}

// TestCurrencyMessageCataloguesMatch keeps the two locales in step.
func TestCurrencyMessageCataloguesMatch(t *testing.T) {
	for key := range currencyMessagesEN {
		if _, ok := currencyMessagesDE[key]; !ok {
			t.Errorf("key %q missing from the German catalogue", key)
		}
	}
	for key := range currencyMessagesDE {
		if _, ok := currencyMessagesEN[key]; !ok {
			t.Errorf("key %q missing from the English catalogue", key)
		}
	}
}

// TestCurrencyMessageParamsInterpolate checks the positional param contract.
func TestCurrencyMessageParamsInterpolate(t *testing.T) {
	days := 42
	got := renderCurrencyMessage(currencyMessagesEN, "rating.expiring", CurrencyMessageParams{Days: &days})
	if !strings.Contains(got, "42") {
		t.Errorf("rendering = %q, want it to interpolate days", got)
	}
	needed := 2
	got = renderCurrencyMessage(currencyMessagesEN, "rating.pax_not_current", CurrencyMessageParams{Needed: &needed})
	if !strings.Contains(got, "2") {
		t.Errorf("rendering = %q, want it to interpolate needed", got)
	}
}

// TestUnknownCurrencyKeyIsVisible — a missing translation renders the key, not blank.
func TestUnknownCurrencyKeyIsVisible(t *testing.T) {
	if got := renderCurrencyMessage(currencyMessagesEN, "rating.does_not_exist", CurrencyMessageParams{}); got != "rating.does_not_exist" {
		t.Errorf("unknown key rendered as %q", got)
	}
}
