package email

import "fmt"

var enTemplates = templateSet{
	CredentialExpiry: func(p CredentialExpiryParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: %s expires in %d days", p.CredentialType, p.DaysRemaining)
		body := fmt.Sprintf(`<h2>Credential Expiry Warning</h2>
<p>Hi %s,</p>
<p>Your <strong>%s</strong> expires on <strong>%s</strong> (%d days from now).</p>
<p>Please renew it before it expires to maintain compliance.</p>
<p>— NinerLog</p>`, p.UserName, p.CredentialType, p.ExpiryDate, p.DaysRemaining)
		return subject, body
	},

	RatingExpiry: func(p RatingExpiryParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: %s %s rating expires in %d days", p.LicenseType, p.ClassType, p.DaysRemaining)
		body := fmt.Sprintf(`<h2>Class Rating Expiry Warning</h2>
<p>Hi %s,</p>
<p>Your <strong>%s %s</strong> rating expires on <strong>%s</strong>.</p>
<p>Complete the required revalidation flights or proficiency check before expiry.</p>
<p>— NinerLog</p>`, p.UserName, p.LicenseType, p.ClassType, p.ExpiryDate)
		return subject, body
	},

	Revalidation: func(p RevalidationParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: %s %s — revalidation requirements need attention", p.LicenseType, p.ClassType)
		body := fmt.Sprintf(`<h2>Currency Revalidation Warning</h2>
<p>Hi %s,</p>
<p>Your <strong>%s %s</strong> rating currency needs attention: %s</p>
<p>— NinerLog</p>`, p.UserName, p.LicenseType, p.ClassType, p.Message)
		return subject, body
	},

	PassengerCurrency: func(p PassengerCurrencyParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: %s %s passenger currency — %d more landings needed", p.ClassType, p.Period, p.Remaining)
		body := fmt.Sprintf(`<h2>Passenger Currency Warning</h2>
<p>Hi %s,</p>
<p>Your <strong>%s</strong> %s passenger currency requires attention.</p>
<p>You have <strong>%d %s landings</strong> in the last 90 days. You need <strong>%d more</strong> to carry passengers.</p>
<p>— NinerLog</p>`, p.UserName, p.ClassType, p.Period, p.Landings, p.Period, p.Remaining)
		return subject, body
	},

	FlightReviewExpiry: func(p FlightReviewExpiryParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: Flight review expires in %d days", p.DaysRemaining)
		body := fmt.Sprintf(`<h2>Flight Review Expiry Warning</h2>
<p>Hi %s,</p>
<p>Your flight review expires on <strong>%s</strong>.</p>
<p>Complete a flight review (14 CFR §61.56) before expiry to maintain flying privileges.</p>
<p>— NinerLog</p>`, p.UserName, p.ExpiryDate)
		return subject, body
	},

	FlightReviewRequired: func(p FlightReviewRequiredParams) (string, string) {
		subject := "NinerLog: Flight review required"
		body := fmt.Sprintf(`<h2>Flight Review Required</h2>
<p>Hi %s,</p>
<p>%s</p>
<p>Complete a flight review (14 CFR §61.56) to regain flying privileges.</p>
<p>— NinerLog</p>`, p.UserName, p.Message)
		return subject, body
	},
}
