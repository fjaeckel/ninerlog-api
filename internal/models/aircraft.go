package models

import (
	"time"

	"github.com/google/uuid"
)

// Aircraft represents an aircraft in a user's fleet
type Aircraft struct {
	ID                uuid.UUID `json:"id"`
	UserID            uuid.UUID `json:"userId"`
	Registration      string    `json:"registration"`
	Type              string    `json:"type"`
	Make              string    `json:"make"`
	Model             string    `json:"model"`
	IsComplex         bool      `json:"isComplex"`
	IsHighPerformance bool      `json:"isHighPerformance"`
	IsTailwheel       bool      `json:"isTailwheel"`
	Notes             *string   `json:"notes,omitempty"`
	IsActive          bool      `json:"isActive"`
	AircraftClass     *string   `json:"aircraftClass,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// Validate checks basic Aircraft field requirements
func (a *Aircraft) Validate() error {
	if a.Registration == "" {
		return ErrAircraftRegistrationRequired
	}
	if a.Type == "" {
		return ErrAircraftTypeRequired
	}
	if a.Make == "" {
		return ErrAircraftMakeRequired
	}
	if a.Model == "" {
		return ErrAircraftModelRequired
	}
	return nil
}
