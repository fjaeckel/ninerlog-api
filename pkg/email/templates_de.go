package email

import (
	"fmt"
	"html"
)

var deTemplates = templateSet{
	CredentialExpiry: func(p CredentialExpiryParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: %s läuft in %d Tagen ab", p.CredentialType, p.DaysRemaining)
		body := fmt.Sprintf(`<h2>Nachweis-Ablaufwarnung</h2>
<p>Hallo %s,</p>
<p>Ihr <strong>%s</strong> läuft am <strong>%s</strong> ab (noch %d Tage).</p>
<p>Bitte erneuern Sie den Nachweis rechtzeitig, um die Gültigkeit aufrechtzuerhalten.</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.CredentialType), html.EscapeString(p.ExpiryDate), p.DaysRemaining)
		return subject, body
	},

	RatingExpiry: func(p RatingExpiryParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: %s %s Berechtigung läuft in %d Tagen ab", p.LicenseType, p.ClassType, p.DaysRemaining)
		body := fmt.Sprintf(`<h2>Berechtigungs-Ablaufwarnung</h2>
<p>Hallo %s,</p>
<p>Ihre <strong>%s %s</strong> Berechtigung läuft am <strong>%s</strong> ab.</p>
<p>Stellen Sie die erforderlichen Verlängerungsflüge oder die Befähigungsüberprüfung vor dem Ablauf sicher.</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.LicenseType), html.EscapeString(p.ClassType), html.EscapeString(p.ExpiryDate))
		return subject, body
	},

	Revalidation: func(p RevalidationParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: %s %s — Verlängerungsanforderungen erfordern Aufmerksamkeit", p.LicenseType, p.ClassType)
		body := fmt.Sprintf(`<h2>Verlängerungswarnung</h2>
<p>Hallo %s,</p>
<p>Ihre <strong>%s %s</strong> Berechtigungsgültigkeit erfordert Aufmerksamkeit: %s</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.LicenseType), html.EscapeString(p.ClassType), html.EscapeString(p.Message))
		return subject, body
	},

	CustomCurrency: func(p CustomCurrencyParams) (string, string) {
		if p.Expiring {
			subject := fmt.Sprintf("NinerLog: %s — läuft bald ab", p.RuleName)
			body := fmt.Sprintf(`<h2>Eigene Aktualität läuft ab</h2>
<p>Hallo %s,</p>
<p>Deine eigene Aktualitätsregel <strong>%s</strong> verfällt am <strong>%s</strong>, sofern du keine anrechenbaren Flüge einträgst.</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.RuleName), html.EscapeString(p.ExpiresOn))
			return subject, body
		}
		subject := fmt.Sprintf("NinerLog: %s — nicht mehr aktuell", p.RuleName)
		body := fmt.Sprintf(`<h2>Eigene Aktualität abgelaufen</h2>
<p>Hallo %s,</p>
<p>Deine eigene Aktualitätsregel <strong>%s</strong> ist nicht mehr aktuell.</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.RuleName))
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
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.ClassType), periodDE, p.Landings, periodDE, p.Remaining)
		return subject, body
	},

	FlightReviewExpiry: func(p FlightReviewExpiryParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: Flugüberprüfung läuft in %d Tagen ab", p.DaysRemaining)
		body := fmt.Sprintf(`<h2>Flugüberprüfungs-Ablaufwarnung</h2>
<p>Hallo %s,</p>
<p>Ihre Flugüberprüfung läuft am <strong>%s</strong> ab.</p>
<p>Absolvieren Sie eine Flugüberprüfung (14 CFR §61.56) vor dem Ablauf, um Ihre Flugberechtigung aufrechtzuerhalten.</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.ExpiryDate))
		return subject, body
	},

	FlightReviewRequired: func(p FlightReviewRequiredParams) (string, string) {
		subject := "NinerLog: Flugüberprüfung erforderlich"
		body := fmt.Sprintf(`<h2>Flugüberprüfung erforderlich</h2>
<p>Hallo %s,</p>
<p>%s</p>
<p>Absolvieren Sie eine Flugüberprüfung (14 CFR §61.56), um Ihre Flugberechtigung wiederzuerlangen.</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.Message))
		return subject, body
	},

	VerifyEmail: func(p VerifyEmailParams) (string, string) {
		subject := "NinerLog: E-Mail-Adresse bestätigen"
		body := fmt.Sprintf(`<h2>Willkommen bei NinerLog</h2>
<p>Hallo %s,</p>
<p>Vielen Dank für Ihre Registrierung. Bitte bestätigen Sie Ihre E-Mail-Adresse, um Ihr Konto zu aktivieren:</p>
<p><a href="%s">E-Mail-Adresse bestätigen</a></p>
<p>Der Link ist 24 Stunden gültig. Wenn Sie kein NinerLog-Konto erstellt haben, können Sie diese E-Mail ignorieren.</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.Link))
		return subject, body
	},

	SignatureRequest: func(p SignatureRequestParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: %s bittet Sie, einen Logbucheintrag zu unterschreiben", p.OwnerName)
		body := fmt.Sprintf(`<h2>Logbuch-Unterschriftsanfrage</h2>
<p>%s bittet Sie, einen Logbucheintrag zu prüfen und zu unterschreiben:</p>
<p><strong>%s</strong></p>
<p><a href="%s">Prüfen und unterschreiben</a></p>
<p>Dieser Link läuft am %s ab. Falls Sie dies nicht erwartet haben, können Sie diese E-Mail ignorieren.</p>
<p>— NinerLog</p>`, html.EscapeString(p.OwnerName), html.EscapeString(p.FlightSummary), html.EscapeString(p.Link), html.EscapeString(p.ExpiresAt))
		return subject, body
	},

	SignatureCompleted: func(p SignatureCompletedParams) (string, string) {
		subject := "NinerLog: Ihr Logbucheintrag wurde unterschrieben"
		body := fmt.Sprintf(`<h2>Unterschrift erfasst</h2>
<p>Hallo %s,</p>
<p><strong>%s</strong> hat Ihren Logbucheintrag unterschrieben:</p>
<p><strong>%s</strong></p>
<p>Dieser Eintrag ist nun gesperrt. Sie können die Unterschrift auf der Detailseite des Flugs widerrufen, falls Änderungen nötig sind (danach ist eine neue Unterschrift erforderlich).</p>
<p>— NinerLog</p>`, html.EscapeString(p.OwnerName), html.EscapeString(p.SignerName), html.EscapeString(p.FlightSummary))
		return subject, body
	},
}
