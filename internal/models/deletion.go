package models

import (
	"time"

	"github.com/google/uuid"
)

// DeletionEntityType names the collection a tombstoned record belonged to.
// The values are the wire format and are also written by the database trigger
// in migration 000054 — they must not drift apart.
type DeletionEntityType string

const (
	DeletionEntityFlight     DeletionEntityType = "flight"
	DeletionEntityAircraft   DeletionEntityType = "aircraft"
	DeletionEntityContact    DeletionEntityType = "contact"
	DeletionEntityCredential DeletionEntityType = "credential"
	DeletionEntityLicense    DeletionEntityType = "license"
)

// ValidDeletionEntityTypes lists every entity a sync client can be told about.
func ValidDeletionEntityTypes() []DeletionEntityType {
	return []DeletionEntityType{
		DeletionEntityFlight,
		DeletionEntityAircraft,
		DeletionEntityContact,
		DeletionEntityCredential,
		DeletionEntityLicense,
	}
}

// IsValid reports whether the value is one the trigger can produce.
func (e DeletionEntityType) IsValid() bool {
	for _, valid := range ValidDeletionEntityTypes() {
		if e == valid {
			return true
		}
	}
	return false
}

// Deletion is a tombstone: the record identified by EntityType/EntityID no
// longer exists.
type Deletion struct {
	UserID     uuid.UUID          `json:"-"`
	EntityType DeletionEntityType `json:"entity"`
	EntityID   uuid.UUID          `json:"id"`
	DeletedAt  time.Time          `json:"deletedAt"`
}
