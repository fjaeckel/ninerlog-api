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
	UserName    string
	LicenseType string
	ClassType   string
	Message     string
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
	UserName string
	Message  string
}

type VerifyEmailParams struct {
	UserName string
	Link     string
}

type SignatureRequestParams struct {
	OwnerName     string
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
	SignatureRequest     func(p SignatureRequestParams) (subject, body string)
	SignatureCompleted   func(p SignatureCompletedParams) (subject, body string)
}
