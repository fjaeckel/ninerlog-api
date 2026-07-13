package email

import (
	"fmt"
	"html"
)

var enTemplates = templateSet{
	CredentialExpiry: func(p CredentialExpiryParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: %s expires in %d days", p.CredentialType, p.DaysRemaining)
		body := fmt.Sprintf(`<h2>Credential Expiry Warning</h2>
<p>Hi %s,</p>
<p>Your <strong>%s</strong> expires on <strong>%s</strong> (%d days from now).</p>
<p>Please renew it before it expires to maintain compliance.</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.CredentialType), html.EscapeString(p.ExpiryDate), p.DaysRemaining)
		return subject, body
	},

	RatingExpiry: func(p RatingExpiryParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: %s %s rating expires in %d days", p.LicenseType, p.ClassType, p.DaysRemaining)
		body := fmt.Sprintf(`<h2>Class Rating Expiry Warning</h2>
<p>Hi %s,</p>
<p>Your <strong>%s %s</strong> rating expires on <strong>%s</strong>.</p>
<p>Complete the required revalidation flights or proficiency check before expiry.</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.LicenseType), html.EscapeString(p.ClassType), html.EscapeString(p.ExpiryDate))
		return subject, body
	},

	Revalidation: func(p RevalidationParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: %s %s — revalidation requirements need attention", p.LicenseType, p.ClassType)
		body := fmt.Sprintf(`<h2>Currency Revalidation Warning</h2>
<p>Hi %s,</p>
<p>Your <strong>%s %s</strong> rating currency needs attention: %s</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.LicenseType), html.EscapeString(p.ClassType), html.EscapeString(p.Message))
		return subject, body
	},

	PassengerCurrency: func(p PassengerCurrencyParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: %s %s passenger currency — %d more landings needed", p.ClassType, p.Period, p.Remaining)
		body := fmt.Sprintf(`<h2>Passenger Currency Warning</h2>
<p>Hi %s,</p>
<p>Your <strong>%s</strong> %s passenger currency requires attention.</p>
<p>You have <strong>%d %s landings</strong> in the last 90 days. You need <strong>%d more</strong> to carry passengers.</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.ClassType), html.EscapeString(p.Period), p.Landings, html.EscapeString(p.Period), p.Remaining)
		return subject, body
	},

	FlightReviewExpiry: func(p FlightReviewExpiryParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: Flight review expires in %d days", p.DaysRemaining)
		body := fmt.Sprintf(`<h2>Flight Review Expiry Warning</h2>
<p>Hi %s,</p>
<p>Your flight review expires on <strong>%s</strong>.</p>
<p>Complete a flight review (14 CFR §61.56) before expiry to maintain flying privileges.</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.ExpiryDate))
		return subject, body
	},

	FlightReviewRequired: func(p FlightReviewRequiredParams) (string, string) {
		subject := "NinerLog: Flight review required"
		body := fmt.Sprintf(`<h2>Flight Review Required</h2>
<p>Hi %s,</p>
<p>%s</p>
<p>Complete a flight review (14 CFR §61.56) to regain flying privileges.</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.Message))
		return subject, body
	},

	VerifyEmail: func(p VerifyEmailParams) (string, string) {
		subject := "NinerLog: Confirm your email address"
		body := fmt.Sprintf(`<h2>Welcome to NinerLog</h2>
<p>Hi %s,</p>
<p>Thanks for signing up. Please confirm your email address to activate your account:</p>
<p><a href="%s">Verify my email</a></p>
<p>This link expires in 24 hours. If you did not create a NinerLog account, you can ignore this email.</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.Link))
		return subject, body
	},

	SignatureRequest: func(p SignatureRequestParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: %s asked you to sign a logbook entry", p.OwnerName)
		body := fmt.Sprintf(`<h2>Logbook Signature Request</h2>
<p>%s has asked you to review and sign a logbook entry:</p>
<p><strong>%s</strong></p>
<p><a href="%s">Review and sign</a></p>
<p>This link expires on %s. If you weren't expecting this, you can ignore this email.</p>
<p>— NinerLog</p>`, html.EscapeString(p.OwnerName), html.EscapeString(p.FlightSummary), html.EscapeString(p.Link), html.EscapeString(p.ExpiresAt))
		return subject, body
	},

	SignatureCompleted: func(p SignatureCompletedParams) (string, string) {
		subject := "NinerLog: Your logbook entry has been signed"
		body := fmt.Sprintf(`<h2>Signature Recorded</h2>
<p>Hi %s,</p>
<p><strong>%s</strong> has signed your logbook entry:</p>
<p><strong>%s</strong></p>
<p>This entry is now locked. You can void the signature from the flight's detail page if you need to make changes (a new signature will then be required).</p>
<p>— NinerLog</p>`, html.EscapeString(p.OwnerName), html.EscapeString(p.SignerName), html.EscapeString(p.FlightSummary))
		return subject, body
	},
}
