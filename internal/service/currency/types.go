package currency

import (
	"github.com/fjaeckel/pilotlog-api/internal/models"
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
	ClassRatingID       uuid.UUID        `json:"classRatingId"`
	ClassType           models.ClassType `json:"classType"`
	LicenseID           uuid.UUID        `json:"licenseId"`
	RegulatoryAuthority string           `json:"regulatoryAuthority"`
	LicenseType         string           `json:"licenseType"`
	Status              Status           `json:"status"`
	ExpiryDate          *string          `json:"expiryDate,omitempty"`
	Message             string           `json:"message"`
	Progress            *Progress        `json:"progress,omitempty"`
	Requirements        []Requirement    `json:"requirements,omitempty"`
}

// Progress holds progress metrics toward currency requirements
type Progress struct {
	TotalHours       float64 `json:"totalHours"`
	PICHours         float64 `json:"picHours"`
	IFRHours         float64 `json:"ifrHours"`
	InstructorHours  float64 `json:"instructorHours"`
	NightHours       float64 `json:"nightHours"`
	Landings         int     `json:"landings"`
	DayLandings      int     `json:"dayLandings"`
	NightLandings    int     `json:"nightLandings"`
	Flights          int     `json:"flights"`
	RequiredHours    float64 `json:"requiredHours,omitempty"`
	RequiredLandings int     `json:"requiredLandings,omitempty"`
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

// CurrencyStatusResponse is the full response from the currency endpoint
type CurrencyStatusResponse struct {
	Ratings []ClassRatingCurrency `json:"ratings"`
}
