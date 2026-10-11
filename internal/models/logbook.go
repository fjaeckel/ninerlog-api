package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Logbook mode switch triggers.
const (
	LogbookTriggerLicence = "licence"
	LogbookTriggerManual  = "manual"
	LogbookTriggerImport  = "import"
)

// Flight revision kinds.
const (
	RevisionKindUpdate = "update"
	RevisionKindDelete = "delete"
)

// LogbookModeEvent is one switch of strict logbook mode.
type LogbookModeEvent struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"userId"`
	Strict    bool       `json:"strict"`
	Trigger   string     `json:"trigger"`
	LicenseID *uuid.UUID `json:"licenseId,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// LogbookMode is a user's strict-mode state.
type LogbookMode struct {
	Strict bool       `json:"strict"`
	Since  *time.Time `json:"since,omitempty"`
}

// LogbookCertification is a "certified through" statement by the pilot.
type LogbookCertification struct {
	ID               uuid.UUID `json:"id"`
	UserID           uuid.UUID `json:"userId"`
	CertifiedThrough time.Time `json:"certifiedThrough"`
	CertifiedAt      time.Time `json:"certifiedAt"`
}

// RevisionCrew is a crew member as stored in a revision's crew snapshot.
type RevisionCrew struct {
	Name      string     `json:"name"`
	Role      string     `json:"role"`
	ContactID *uuid.UUID `json:"contactId,omitempty"`
}

// FlightRevision is the state of a flight before one recorded change.
type FlightRevision struct {
	ID           uuid.UUID       `json:"id"`
	UserID       uuid.UUID       `json:"userId"`
	FlightID     uuid.UUID       `json:"flightId"`
	RevisionNo   int             `json:"revisionNo"`
	Kind         string          `json:"kind"`
	Snapshot     json.RawMessage `json:"snapshot"`
	CrewSnapshot json.RawMessage `json:"crewSnapshot"`
	Source       string          `json:"source"`
	Reason       *string         `json:"reason,omitempty"`
	IsCorrection bool            `json:"isCorrection"`
	ChangedAt    time.Time       `json:"changedAt"`
}
