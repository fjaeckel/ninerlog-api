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
	UserName    string
	LicenseType string
	ClassType   string
	ExpiryDate  string
	DaysRemaining int
}

type RevalidationParams struct {
	UserName    string
	LicenseType string
	ClassType   string
	Message     string
}

type PassengerCurrencyParams struct {
	UserName    string
	ClassType   string
	Landings    int
	Required    int
	Remaining   int
	Period      string // "day" or "night"
}

type FlightReviewExpiryParams struct {
	UserName   string
	ExpiryDate string
	DaysRemaining int
}

type FlightReviewRequiredParams struct {
	UserName string
	Message  string
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
	PassengerCurrency    func(p PassengerCurrencyParams) (subject, body string)
	FlightReviewExpiry   func(p FlightReviewExpiryParams) (subject, body string)
	FlightReviewRequired func(p FlightReviewRequiredParams) (subject, body string)
}
