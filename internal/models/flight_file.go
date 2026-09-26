package models

import (
	"time"

	"github.com/google/uuid"
)

// FlightFileKind names the format of a file attached to a flight.
type FlightFileKind string

// FlightFileKindIGC is an FAI IGC flight recorder file.
const FlightFileKindIGC FlightFileKind = "IGC"

const (
	// MaxFlightFileBytes caps a single flight file.
	MaxFlightFileBytes = 5 * 1024 * 1024
	// MaxFlightFilesPerFlight caps how many files one flight can carry.
	MaxFlightFilesPerFlight = 5
	// MaxFlightFileFilenameLen bounds the display filename.
	MaxFlightFileFilenameLen = 255
	// FlightFileContentTypeIGC is the content type an IGC download is served
	// with.
	FlightFileContentTypeIGC = "application/octet-stream"
)

// FlightFile is a flight recorder file attached to a flight. Content is
// populated only on the download and export paths.
type FlightFile struct {
	ID        uuid.UUID      `json:"id"`
	UserID    uuid.UUID      `json:"userId"`
	FlightID  uuid.UUID      `json:"flightId"`
	Kind      FlightFileKind `json:"kind"`
	Filename  string         `json:"filename"`
	SizeBytes int            `json:"sizeBytes"`
	SHA256    string         `json:"sha256"`
	Content   []byte         `json:"-"`
	CreatedAt time.Time      `json:"createdAt"`
}
