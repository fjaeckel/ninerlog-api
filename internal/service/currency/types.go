package currency

import (
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// Status represents the currency status of a class rating
type Status string

const (
	StatusCurrent  Status = "current"
	StatusExpiring Status = "expiring"
	StatusExpired  Status = "expired"
	// StatusLapsed is a rolling recency rule with unmet requirements: the
	// licence stays valid, its privileges are not exercisable until recency
	// is restored.
	StatusLapsed  Status = "lapsed"
	StatusUnknown Status = "unknown"
)

// ClassRatingCurrency holds the currency evaluation result for one class rating
type ClassRatingCurrency struct {
	ClassRatingID       uuid.UUID        `json:"classRatingId"`
	ClassType           models.ClassType `json:"classType"`
	LicenseID           uuid.UUID        `json:"licenseId"`
	RegulatoryAuthority string           `json:"regulatoryAuthority"`
	LicenseType         string           `json:"licenseType"`
	Status              Status           `json:"status"`
	ExpiryDate          *string          `json:"expiryDate,omitempty"`
	// WindowOpensAt is set for expiry-anchored revalidation rules
	// (EASA FCL.740.A SEP/TMG/MEP/SET, FCL.625.A IR). It is the date on which
	// the 12-month experience-counting window opens (expiry − 12 months).
	// Omitted for rolling-window rules (LAPL/SPL) and expiry-only fallbacks.
	WindowOpensAt *string `json:"windowOpensAt,omitempty"`
	// WindowOpen is only meaningful when WindowOpensAt is set. It is true
	// once now >= WindowOpensAt; while false, flight experience does not
	// yet count toward this rating's revalidation and Requirements is
	// suppressed.
	WindowOpen           bool                   `json:"windowOpen"`
	MessageKey           string                 `json:"messageKey"`
	MessageParams        *MessageParams         `json:"messageParams,omitempty"`
	RuleDescription      string                 `json:"ruleDescription,omitempty"`
	RuleDescriptionKey   string                 `json:"ruleDescriptionKey,omitempty"`
	Progress             *Progress              `json:"progress,omitempty"`
	Requirements         []Requirement          `json:"requirements,omitempty"`
	LaunchMethodCurrency []LaunchMethodCurrency `json:"launchMethodCurrency,omitempty"`
	// CountedClasses lists the aircraft classes whose flights count toward
	// this rating, when that is more than the rating's own class.
	CountedClasses []models.ClassType `json:"countedClasses,omitempty"`
	// CreditedULKinds lists the ultralight kinds whose flights also count
	// toward this rating.
	CreditedULKinds []models.ULKind `json:"creditedUltralightKinds,omitempty"`
	// UnclassifiedFlights counts flights in the window on ULTRALIGHT aircraft
	// with no kind that the rule did not count.
	UnclassifiedFlights int `json:"unclassifiedFlights,omitempty"`
}

// Progress holds progress metrics toward currency requirements (all times in minutes)
type Progress struct {
	TotalMinutes      int `json:"totalMinutes"`
	PICMinutes        int `json:"picMinutes"`
	IFRMinutes        int `json:"ifrMinutes"`
	InstructorMinutes int `json:"instructorMinutes"`
	NightMinutes      int `json:"nightMinutes"`
	Landings          int `json:"landings"`
	DayLandings       int `json:"dayLandings"`
	NightLandings     int `json:"nightLandings"`
	Flights           int `json:"flights"`
	Approaches        int `json:"approaches"`
	Holds             int `json:"holds"`
	// Launches counts the flights' launches; a flight stored without a launch
	// count counts its take-offs, at least one.
	Launches int `json:"launches"`
	// SPICMinutes is student pilot-in-command (supervised solo) time.
	SPICMinutes int `json:"spicMinutes"`
	// TrainingFlights counts flights with dual time received.
	TrainingFlights int `json:"trainingFlights"`
	// LongestTrainingFlightMinutes is the longest total time of a flight with dual time received.
	LongestTrainingFlightMinutes int `json:"longestTrainingFlightMinutes"`
	RequiredMinutes              int `json:"requiredMinutes,omitempty"`
	RequiredLandings             int `json:"requiredLandings,omitempty"`
}

// Requirement represents a single currency requirement with progress
type Requirement struct {
	// Name is set only for custom currency rules, where it is author-supplied
	// user data. Regulatory requirements carry NameKey instead.
	Name          string         `json:"name,omitempty"`
	NameKey       string         `json:"nameKey,omitempty"`
	Met           bool           `json:"met"`
	Current       float64        `json:"current"`
	Required      float64        `json:"required"`
	Unit          string         `json:"unit"`
	MessageKey    string         `json:"messageKey"`
	MessageParams *MessageParams `json:"messageParams,omitempty"`
}

// CurrencyStatusResponse is the full response from the currency endpoint.
// It separates rating currency (Tier 1 — can you fly?) from passenger currency
// (Tier 2 — can you carry passengers?). These are independent evaluations.
type CurrencyStatusResponse struct {
	// Tier 1: Rating/license currency — determines whether the pilot can fly at all in this class
	Ratings []ClassRatingCurrency `json:"ratings"`

	// Tier 2: Passenger currency — determines whether the pilot can carry passengers (rolling from now)
	PassengerCurrency []PassengerCurrency `json:"passengerCurrency"`

	// Flight review status (FAA §61.56) — per-pilot, not per-rating
	FlightReview *FlightReviewStatus `json:"flightReview,omitempty"`
}

// PassengerCurrency holds passenger-carrying currency for one class type.
// EASA: FCL.060(b) — 3 T&L in preceding 90 days in same type/class.
// FAA: §61.57(a)/(b) — 3 T&L day / 3 full-stop night T&L in 90 days.
type PassengerCurrency struct {
	ClassType models.ClassType `json:"classType"`
	// ULKind is the ultralight kind the result covers, for ULTRALIGHT only.
	ULKind              *models.ULKind `json:"ulKind,omitempty"`
	RegulatoryAuthority string         `json:"regulatoryAuthority"`
	DayStatus           Status         `json:"dayStatus"`
	NightStatus         Status         `json:"nightStatus"`
	DayLandings         int            `json:"dayLandings"`
	NightLandings       int            `json:"nightLandings"`
	DayRequired         int            `json:"dayRequired"`
	NightRequired       int            `json:"nightRequired"`
	NightPrivilege      bool           `json:"nightPrivilege"`
	// DayExpiresOn is the last date the day requirement stays met with no
	// further flying — the oldest qualifying landing plus 90 days. Omitted
	// when the requirement is not currently met.
	DayExpiresOn *string `json:"dayExpiresOn,omitempty"`
	// NightExpiresOn is the same for the night requirement. Omitted when the
	// requirement is not met, not applicable, or waived (EASA IR holders).
	NightExpiresOn     *string        `json:"nightExpiresOn,omitempty"`
	MessageKey         string         `json:"messageKey"`
	MessageParams      *MessageParams `json:"messageParams,omitempty"`
	RuleDescription    string         `json:"ruleDescription"`
	RuleDescriptionKey string         `json:"ruleDescriptionKey,omitempty"`
}

// FlightReviewStatus tracks FAA §61.56 flight review currency (24 calendar months).
type FlightReviewStatus struct {
	LastCompleted *string        `json:"lastCompleted,omitempty"`
	ExpiresOn     *string        `json:"expiresOn,omitempty"`
	Status        Status         `json:"status"`
	MessageKey    string         `json:"messageKey"`
	MessageParams *MessageParams `json:"messageParams,omitempty"`
}

// LaunchMethodCurrency tracks launch method recency per SFCL.155(c): 5 launches
// per method (2 for bungee) in the rolling 24 months.
type LaunchMethodCurrency struct {
	Method     string `json:"method"`
	Launches   int    `json:"launches"`
	Required   int    `json:"required"`
	Met        bool   `json:"met"`
	MessageKey string `json:"messageKey"`
}
