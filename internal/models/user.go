package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// User represents a user in the system
type User struct {
	ID                  uuid.UUID      `json:"id"`
	Email               string         `json:"email"`
	PasswordHash        string         `json:"-"`
	Name                string         `json:"name"`
	EmailVerified       bool           `json:"emailVerified"`
	TwoFactorEnabled    bool           `json:"twoFactorEnabled"`
	TwoFactorSecret     *string        `json:"-"` // never exposed in JSON
	RecoveryCodes       pq.StringArray `json:"-"` // never exposed in JSON
	FailedLoginAttempts int            `json:"-"`
	LockedUntil         *time.Time     `json:"-"`
	Disabled            bool           `json:"disabled"`
	LastLoginAt         *time.Time     `json:"lastLoginAt,omitempty"`
	TimeDisplayFormat   string         `json:"timeDisplayFormat"`
	DateFormat          string         `json:"dateFormat"`
	DecimalSeparator    string         `json:"decimalSeparator"`
	PreferredLocale     string         `json:"preferredLocale"`
	// Informational 90-day recency indicator preferences (FCL.060(b)-style)
	RecencyPerModel        bool `json:"recencyPerModel"`
	RecencyPerRegistration bool `json:"recencyPerRegistration"`
	// Which optional columns the flights list shows. In FlightListColumnModeAuto
	// the client picks them from the data on the page; in
	// FlightListColumnModeCustom, FlightListColumns is the user's own choice and
	// an empty list means "none of the optional columns".
	FlightListColumnMode string         `json:"flightListColumnMode"`
	FlightListColumns    pq.StringArray `json:"flightListColumns"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

// Flight list column modes.
const (
	FlightListColumnModeAuto   = "auto"
	FlightListColumnModeCustom = "custom"
)

// FlightListColumns is the set of flights-list columns a user may switch on or
// off. Date, route, aircraft and total time are the identity of a logbook row
// and are always shown, so they are deliberately absent here.
//
// The order is the display order the client uses, and — for the time columns —
// the priority order in which they survive on a narrow screen. Keep it in sync
// with the enum in api-spec/openapi.yaml and the registry in the frontend's
// src/components/flights/flightTableColumns.ts.
var FlightListColumns = []string{
	"offOnBlock",
	"picTime",
	"nightTime",
	"dualTime",
	"ifrTime",
	"crossCountryTime",
	"sicTime",
	"dualGivenTime",
	"multiPilotTime",
	"soloTime",
	"simulatedFlightTime",
	"function",
	"landings",
	"remarks",
}

// NormalizeFlightListColumns drops unknown and duplicate column keys and
// returns the remainder in canonical display order. Unknown keys are ignored
// rather than rejected, matching how the other display preferences treat a
// value they do not recognise.
func NormalizeFlightListColumns(columns []string) pq.StringArray {
	selected := make(map[string]bool, len(columns))
	for _, c := range columns {
		selected[c] = true
	}

	normalized := make(pq.StringArray, 0, len(columns))
	for _, c := range FlightListColumns {
		if selected[c] {
			normalized = append(normalized, c)
		}
	}
	return normalized
}

// NormalizeFlightListColumnMode maps an unrecognised mode back to the default.
func NormalizeFlightListColumnMode(mode string) string {
	if mode == FlightListColumnModeCustom {
		return FlightListColumnModeCustom
	}
	return FlightListColumnModeAuto
}

// RefreshToken represents a refresh token in the system
type RefreshToken struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PasswordResetToken represents a password reset token
type PasswordResetToken struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
	CreatedAt time.Time `json:"created_at"`
}

// EmailVerificationToken represents a single-use email-verification token sent
// to a user after registration (or via the resend endpoint).
type EmailVerificationToken struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
	CreatedAt time.Time `json:"created_at"`
}
