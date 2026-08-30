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
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.LicenseType), html.EscapeString(p.ClassType),
			html.EscapeString(renderCurrencyMessage(currencyMessagesEN, p.MessageKey, p.MessageParams)))
		return subject, body
	},

	CustomCurrency: func(p CustomCurrencyParams) (string, string) {
		if p.Expiring {
			subject := fmt.Sprintf("NinerLog: %s — expiring soon", p.RuleName)
			body := fmt.Sprintf(`<h2>Custom Currency Expiring</h2>
<p>Hi %s,</p>
<p>Your custom currency rule <strong>%s</strong> will lapse on <strong>%s</strong> unless you log qualifying flights.</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.RuleName), html.EscapeString(p.ExpiresOn))
			return subject, body
		}
		subject := fmt.Sprintf("NinerLog: %s — no longer current", p.RuleName)
		body := fmt.Sprintf(`<h2>Custom Currency Lapsed</h2>
<p>Hi %s,</p>
<p>Your custom currency rule <strong>%s</strong> is no longer current.</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.RuleName))
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
<p>— NinerLog</p>`, html.EscapeString(p.UserName),
			html.EscapeString(renderCurrencyMessage(currencyMessagesEN, p.MessageKey, p.MessageParams)))
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

	VerificationReminder: func(p VerificationReminderParams) (string, string) {
		subject := "NinerLog: Please confirm your email address"
		body := fmt.Sprintf(`<h2>Your NinerLog account is not active yet</h2>
<p>Hi %s,</p>
<p>Your email address has not been confirmed, so you cannot sign in yet. Here is a fresh link:</p>
<p><a href="%s">Verify my email</a></p>
<p>This link expires in %d hours.</p>
<p><strong>If the address is not confirmed within %d days, the account will be deleted.</strong>
Nothing is kept, and you can sign up again at any time.</p>
<p>If you did not create a NinerLog account, no action is needed — it will be removed on its own.</p>
<p>— NinerLog</p>`,
			html.EscapeString(p.UserName), html.EscapeString(p.Link), p.LinkValidDays*24, p.DeletionDays)
		return subject, body
	},

	PasswordReset: func(p PasswordResetParams) (string, string) {
		subject := "NinerLog: Password Reset"
		twoFactorNote := ""
		if p.TwoFactorEnabled {
			twoFactorNote = `<p>Your account uses two-factor authentication. You will also need a code from
your authenticator app — or one of your recovery codes — to complete the reset.
Resetting your password does not switch two-factor authentication off.</p>`
		}
		body := fmt.Sprintf(`<h2>Password Reset</h2>
<p>Hi %s,</p>
<p>You requested a password reset for your NinerLog account.</p>
<p><a href="%s">Click here to reset your password</a></p>
%s<p>This link expires in 1 hour. If you did not request this, you can ignore this email —
your password stays unchanged.</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), html.EscapeString(p.Link), twoFactorNote)
		return subject, body
	},

	PasswordChanged: func(p PasswordChangedParams) (string, string) {
		subject := "NinerLog: Your password was changed"
		twoFactorNote := "<p>Two-factor authentication is not enabled on your account. Enabling it in your security settings protects you if your password is ever exposed.</p>"
		if p.TwoFactorEnabled {
			twoFactorNote = "<p>Two-factor authentication is still enabled on your account and was not changed.</p>"
		}
		body := fmt.Sprintf(`<h2>Password Changed</h2>
<p>Hi %s,</p>
<p>The password for your NinerLog account was just reset using a password reset link.
You have been signed out on all devices.</p>
%s<p><strong>If this wasn't you</strong>, reset your password again immediately from the
sign-in page and contact your NinerLog administrator.</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName), twoFactorNote)
		return subject, body
	},

	TwoFactorReset: func(p TwoFactorResetParams) (string, string) {
		subject := "NinerLog: Two-factor authentication was reset"
		body := fmt.Sprintf(`<h2>Two-Factor Authentication Reset</h2>
<p>Hi %s,</p>
<p>An administrator has removed two-factor authentication from your NinerLog account.
Your password still works, but signing in no longer asks for an authenticator code,
and your previous recovery codes are no longer valid.</p>
<p>Set two-factor authentication up again in your security settings to restore protection.</p>
<p><strong>If you did not ask for this</strong>, contact your NinerLog administrator.</p>
<p>— NinerLog</p>`, html.EscapeString(p.UserName))
		return subject, body
	},

	SignatureRequest: func(p SignatureRequestParams) (string, string) {
		subject := fmt.Sprintf("NinerLog: %s asked you to sign a logbook entry", p.OwnerName)
		body := fmt.Sprintf(`<h2>Logbook Signature Request</h2>
<p>%s (NinerLog account: %s) has asked you to review and sign a logbook entry:</p>
<p><strong>%s</strong></p>
<p><a href="%s">Review and sign</a></p>
<p>This link expires on %s. If you weren't expecting this, you can ignore this email.</p>
<p style="color:#666;font-size:12px">This request was sent by a NinerLog user, not by NinerLog. The
display name above is chosen by that user and is not verified. Only sign if you
recognise the account address and expected this request. NinerLog will never ask
you for a password.</p>
<p>— NinerLog</p>`, html.EscapeString(p.OwnerName), html.EscapeString(p.OwnerEmail), html.EscapeString(p.FlightSummary), html.EscapeString(p.Link), html.EscapeString(p.ExpiresAt))
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
