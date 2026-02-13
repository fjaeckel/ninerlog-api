package models

import (
	"time"

	"github.com/google/uuid"
)

// EngineType represents the type of engine on an aircraft
type EngineType string

const (
	EngineTypePiston    EngineType = "piston"
	EngineTypeTurboprop EngineType = "turboprop"
	EngineTypeJet       EngineType = "jet"
	EngineTypeElectric  EngineType = "electric"
)

// Aircraft represents an aircraft in a user's fleet
type Aircraft struct {
	ID                uuid.UUID   `json:"id"`
	UserID            uuid.UUID   `json:"userId"`
	Registration      string      `json:"registration"`
	Type              string      `json:"type"`
	Make              string      `json:"make"`
	Model             string      `json:"model"`
	Category          *string     `json:"category,omitempty"`
	EngineType        *EngineType `json:"engineType,omitempty"`
	IsComplex         bool        `json:"isComplex"`
	IsHighPerformance bool        `json:"isHighPerformance"`
	IsTailwheel       bool        `json:"isTailwheel"`
	Notes             *string     `json:"notes,omitempty"`
	IsActive          bool        `json:"isActive"`
	CreatedAt         time.Time   `json:"createdAt"`
	UpdatedAt         time.Time   `json:"updatedAt"`
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
	if a.EngineType != nil {
		switch *a.EngineType {
		case EngineTypePiston, EngineTypeTurboprop, EngineTypeJet, EngineTypeElectric:
			// valid
		default:
			return ErrAircraftInvalidEngineType
		}
	}
	return nil
}
