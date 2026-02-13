package models

import (
	"time"

	"github.com/google/uuid"
)

// CrewRole represents a person's role on a flight
type CrewRole string

const (
	CrewRolePIC         CrewRole = "PIC"
	CrewRoleSIC         CrewRole = "SIC"
	CrewRoleInstructor  CrewRole = "Instructor"
	CrewRoleStudent     CrewRole = "Student"
	CrewRolePassenger   CrewRole = "Passenger"
	CrewRoleSafetyPilot CrewRole = "SafetyPilot"
	CrewRoleExaminer    CrewRole = "Examiner"
)

// ValidCrewRoles returns all valid crew roles
func ValidCrewRoles() []CrewRole {
	return []CrewRole{
		CrewRolePIC, CrewRoleSIC, CrewRoleInstructor, CrewRoleStudent,
		CrewRolePassenger, CrewRoleSafetyPilot, CrewRoleExaminer,
	}
}

// IsValidCrewRole checks if a crew role is valid
func IsValidCrewRole(role CrewRole) bool {
	for _, r := range ValidCrewRoles() {
		if r == role {
			return true
		}
	}
	return false
}

// Contact represents a reusable person that a user can reference across flights
type Contact struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"userId"`
	Name      string    `json:"name"`
	Email     *string   `json:"email,omitempty"`
	Phone     *string   `json:"phone,omitempty"`
	Notes     *string   `json:"notes,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// FlightCrewMember represents a person on board a specific flight
type FlightCrewMember struct {
	ID        uuid.UUID  `json:"id"`
	FlightID  uuid.UUID  `json:"flightId"`
	ContactID *uuid.UUID `json:"contactId,omitempty"`
	Name      string     `json:"name"`
	Role      CrewRole   `json:"role"`
}
