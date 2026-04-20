package email

import "fmt"

var deTemplates = templateSet{
	CredentialExpiry: func(p CredentialExpiryParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: %s läuft in %d Tagen ab", p.CredentialType, p.DaysRemaining)
		body := fmt.Sprintf(`<h2>Nachweis-Ablaufwarnung</h2>
<p>Hallo %s,</p>
<p>Ihr <strong>%s</strong> läuft am <strong>%s</strong> ab (noch %d Tage).</p>
<p>Bitte erneuern Sie den Nachweis rechtzeitig, um die Gültigkeit aufrechtzuerhalten.</p>
<p>— NinerLog</p>`, p.UserName, p.CredentialType, p.ExpiryDate, p.DaysRemaining)
		return subject, body
	},

	RatingExpiry: func(p RatingExpiryParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: %s %s Berechtigung läuft in %d Tagen ab", p.LicenseType, p.ClassType, p.DaysRemaining)
		body := fmt.Sprintf(`<h2>Berechtigungs-Ablaufwarnung</h2>
<p>Hallo %s,</p>
<p>Ihre <strong>%s %s</strong> Berechtigung läuft am <strong>%s</strong> ab.</p>
<p>Stellen Sie die erforderlichen Verlängerungsflüge oder die Befähigungsüberprüfung vor dem Ablauf sicher.</p>
<p>— NinerLog</p>`, p.UserName, p.LicenseType, p.ClassType, p.ExpiryDate)
		return subject, body
	},

	Revalidation: func(p RevalidationParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: %s %s — Verlängerungsanforderungen erfordern Aufmerksamkeit", p.LicenseType, p.ClassType)
		body := fmt.Sprintf(`<h2>Verlängerungswarnung</h2>
<p>Hallo %s,</p>
<p>Ihre <strong>%s %s</strong> Berechtigungsgültigkeit erfordert Aufmerksamkeit: %s</p>
<p>— NinerLog</p>`, p.UserName, p.LicenseType, p.ClassType, p.Message)
		return subject, body
	},

	PassengerCurrency: func(p PassengerCurrencyParams) (string, string) {
		periodDE := "Tag"
		if p.Period == "night" {
			periodDE = "Nacht"
		}
		subject := fmt.Sprintf("NinerLog: %s %s-Passagiergültigkeit — %d weitere Landungen erforderlich", p.ClassType, periodDE, p.Remaining)
		body := fmt.Sprintf(`<h2>Passagier-Gültigkeitswarnung</h2>
<p>Hallo %s,</p>
<p>Ihre <strong>%s</strong> %s-Passagiergültigkeit erfordert Aufmerksamkeit.</p>
<p>Sie haben <strong>%d %s-Landungen</strong> in den letzten 90 Tagen. Sie benötigen <strong>%d weitere</strong> für die Passagiermitnahme.</p>
<p>— NinerLog</p>`, p.UserName, p.ClassType, periodDE, p.Landings, periodDE, p.Remaining)
		return subject, body
	},

	FlightReviewExpiry: func(p FlightReviewExpiryParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: Flugüberprüfung läuft in %d Tagen ab", p.DaysRemaining)
		body := fmt.Sprintf(`<h2>Flugüberprüfungs-Ablaufwarnung</h2>
<p>Hallo %s,</p>
<p>Ihre Flugüberprüfung läuft am <strong>%s</strong> ab.</p>
<p>Absolvieren Sie eine Flugüberprüfung (14 CFR §61.56) vor dem Ablauf, um Ihre Flugberechtigung aufrechtzuerhalten.</p>
<p>— NinerLog</p>`, p.UserName, p.ExpiryDate)
		return subject, body
	},

	FlightReviewRequired: func(p FlightReviewRequiredParams) (string, string) {
		subject := "NinerLog: Flugüberprüfung erforderlich"
		body := fmt.Sprintf(`<h2>Flugüberprüfung erforderlich</h2>
<p>Hallo %s,</p>
<p>%s</p>
<p>Absolvieren Sie eine Flugüberprüfung (14 CFR §61.56), um Ihre Flugberechtigung wiederzuerlangen.</p>
<p>— NinerLog</p>`, p.UserName, p.Message)
		return subject, body
	},
}
