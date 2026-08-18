package models

import (
	"time"

	"github.com/google/uuid"
)

// FlightImport is the persisted record of one logbook import run (the
// flight_imports table). Format and Status are stored as the import_format and
// import_status Postgres enums; both are validated upstream against the
// OpenAPI vocabulary before a record is written.
type FlightImport struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	FileName       string
	Format         string
	Status         string
	TotalRows      int
	ImportedCount  int
	SkippedCount   int
	ErrorCount     int
	DuplicateCount int
	// ImportedFlightIDs holds the imported flight UUIDs in Postgres array
	// literal form ("{id,id}"), matching the uuid[] column.
	ImportedFlightIDs string
	// Errors and ColumnMappings are stored as raw JSON.
	Errors         []byte
	ColumnMappings []byte
	CreatedAt      time.Time
}
