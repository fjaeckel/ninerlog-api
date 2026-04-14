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
	StatusUnknown  Status = "unknown"
)

// ClassRatingCurrency holds the currency evaluation result for one class rating
type ClassRatingCurrency struct {
	ClassRatingID        uuid.UUID              `json:"classRatingId"`
	ClassType            models.ClassType       `json:"classType"`
	LicenseID            uuid.UUID              `json:"licenseId"`
	RegulatoryAuthority  string                 `json:"regulatoryAuthority"`
	LicenseType          string                 `json:"licenseType"`
	Status               Status                 `json:"status"`
	ExpiryDate           *string                `json:"expiryDate,omitempty"`
	Message              string                 `json:"message"`
	RuleDescription      string                 `json:"ruleDescription,omitempty"`
	RuleDescriptionKey   string                 `json:"ruleDescriptionKey,omitempty"`
	Progress             *Progress              `json:"progress,omitempty"`
	Requirements         []Requirement          `json:"requirements,omitempty"`
	LaunchMethodCurrency []LaunchMethodCurrency `json:"launchMethodCurrency,omitempty"`
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
	RequiredMinutes   int `json:"requiredMinutes,omitempty"`
	RequiredLandings  int `json:"requiredLandings,omitempty"`
}

// Requirement represents a single currency requirement with progress
type Requirement struct {
	Name     string  `json:"name"`
	Met      bool    `json:"met"`
	Current  float64 `json:"current"`
	Required float64 `json:"required"`
	Unit     string  `json:"unit"`
	Message  string  `json:"message"`
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
	ClassType           models.ClassType    `json:"classType"`
	RegulatoryAuthority string              `json:"regulatoryAuthority"`
	DayStatus           Status              `json:"dayStatus"`
	NightStatus         Status              `json:"nightStatus"`
	DayLandings         int                 `json:"dayLandings"`
	NightLandings       int                 `json:"nightLandings"`
	DayRequired         int                 `json:"dayRequired"`
	NightRequired       int                 `json:"nightRequired"`
	NightPrivilege      bool                `json:"nightPrivilege"`
	Message             string              `json:"message"`
	RuleDescription     string              `json:"ruleDescription"`
	RuleDescriptionKey  string              `json:"ruleDescriptionKey,omitempty"`
	PassengerPrivilege  *PassengerPrivilege `json:"passengerPrivilege,omitempty"`
}

// PassengerPrivilege holds informational passenger-carrying privilege status.
// Some license types (LAPL, SPL, UL) require additional experience before
// the pilot can carry passengers at all — separate from 90-day recency.
type PassengerPrivilege struct {
	Eligible bool   `json:"eligible"`
	Message  string `json:"message"`
}

// FlightReviewStatus tracks FAA §61.56 flight review currency (24 calendar months).
type FlightReviewStatus struct {
	LastCompleted *string `json:"lastCompleted,omitempty"`
	ExpiresOn     *string `json:"expiresOn,omitempty"`
	Status        Status  `json:"status"`
	Message       string  `json:"message"`
}

// LaunchMethodCurrency tracks SPL launch method currency per FCL.140.S(b)(1).
// 5 launches per method (winch, aerotow, self-launch) in rolling 24 months.
type LaunchMethodCurrency struct {
	Method   string `json:"method"`
	Launches int    `json:"launches"`
	Required int    `json:"required"`
	Met      bool   `json:"met"`
	Message  string `json:"message"`
}
