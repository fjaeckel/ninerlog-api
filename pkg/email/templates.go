package email

// LocalizedTemplates provides email subject/body templates per locale.
// Each function returns (subject, body) for the given parameters.

type CredentialExpiryParams struct {
	UserName       string
	CredentialType string
	ExpiryDate     string
	DaysRemaining  int
}

type RatingExpiryParams struct {
	UserName      string
	LicenseType   string
	ClassType     string
	ExpiryDate    string
	DaysRemaining int
}

type CustomCurrencyParams struct {
	UserName string
	RuleName string
	// Expiring is true for a "will lapse soon" warning (ExpiresOn set), false
	// for a "no longer current" notice.
	Expiring  bool
	ExpiresOn string
}

type RevalidationParams struct {
	UserName      string
	LicenseType   string
	ClassType     string
	MessageKey    string
	MessageParams CurrencyMessageParams
}

type PassengerCurrencyParams struct {
	UserName  string
	ClassType string
	Landings  int
	Required  int
	Remaining int
	Period    string // "day" or "night"
}

type FlightReviewExpiryParams struct {
	UserName      string
	ExpiryDate    string
	DaysRemaining int
}

type FlightReviewRequiredParams struct {
	UserName      string
	MessageKey    string
	MessageParams CurrencyMessageParams
}

type VerifyEmailParams struct {
	UserName string
	Link     string
}

// VerificationReminderParams drives the follow-up sent when an account is
// still unverified a day after signup.
type VerificationReminderParams struct {
	UserName string
	Link     string
	// DeletionDays is how long the account survives from this reminder.
	DeletionDays int
	// LinkValidDays is how long the enclosed link works, which is shorter.
	LinkValidDays int
}

type PasswordResetParams struct {
	UserName string
	Link     string
	// TwoFactorEnabled warns the user up front that finishing the reset also
	// requires their authenticator code or a recovery code.
	TwoFactorEnabled bool
}

// PasswordChangedParams drives the security notice sent after a password reset
// completes.
type PasswordChangedParams struct {
	UserName string
	// TwoFactorEnabled reports whether 2FA is still active on the account.
	TwoFactorEnabled bool
}

// TwoFactorResetParams drives the notice sent when an administrator clears a
// user's 2FA enrolment.
type TwoFactorResetParams struct {
	UserName string
}

type SignatureRequestParams struct {
	OwnerName string
	// OwnerEmail is the requester's verified account address, shown alongside
	// the free-form OwnerName.
	OwnerEmail    string
	FlightSummary string // e.g. "12 Jul 2026 — D-EFGH (C172), 1h24m"
	Link          string
	ExpiresAt     string
}

type SignatureCompletedParams struct {
	OwnerName     string
	FlightSummary string
	SignerName    string
}

// Templates returns the email template functions for the given locale.
// Falls back to English if locale is not supported.
func Templates(locale string) templateSet {
	switch locale {
	case "de":
		return deTemplates
	default:
		return enTemplates
	}
}

type templateSet struct {
	CredentialExpiry     func(p CredentialExpiryParams) (subject, body string)
	RatingExpiry         func(p RatingExpiryParams) (subject, body string)
	Revalidation         func(p RevalidationParams) (subject, body string)
	CustomCurrency       func(p CustomCurrencyParams) (subject, body string)
	PassengerCurrency    func(p PassengerCurrencyParams) (subject, body string)
	FlightReviewExpiry   func(p FlightReviewExpiryParams) (subject, body string)
	FlightReviewRequired func(p FlightReviewRequiredParams) (subject, body string)
	VerifyEmail          func(p VerifyEmailParams) (subject, body string)
	VerificationReminder func(p VerificationReminderParams) (subject, body string)
	PasswordReset        func(p PasswordResetParams) (subject, body string)
	PasswordChanged      func(p PasswordChangedParams) (subject, body string)
	TwoFactorReset       func(p TwoFactorResetParams) (subject, body string)
	SignatureRequest     func(p SignatureRequestParams) (subject, body string)
	SignatureCompleted   func(p SignatureCompletedParams) (subject, body string)
}
