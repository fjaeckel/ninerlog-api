package models

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AircraftReminderKind is the kind of a dated aircraft item.
type AircraftReminderKind string

const (
	ReminderKindAnnualInspection   AircraftReminderKind = "ANNUAL_INSPECTION"
	ReminderKindInsurance          AircraftReminderKind = "INSURANCE"
	ReminderKindRescueSystemRepack AircraftReminderKind = "RESCUE_SYSTEM_REPACK"
	ReminderKindRescueRocketExpiry AircraftReminderKind = "RESCUE_ROCKET_EXPIRY"
	ReminderKindARC                AircraftReminderKind = "ARC"
	ReminderKindELTBattery         AircraftReminderKind = "ELT_BATTERY"
	ReminderKindCustom             AircraftReminderKind = "CUSTOM"
)

// Bounds on aircraft reminder fields.
const (
	AircraftReminderLabelMaxLen   = 100
	AircraftReminderNotesMaxLen   = 1000
	AircraftReminderMinInterval   = 1
	AircraftReminderMaxInterval   = 240
	AircraftReminderDueSoonDays   = 30
	AircraftReminderStatusOK      = "ok"
	AircraftReminderStatusDueSoon = "due_soon"
	AircraftReminderStatusOverdue = "overdue"
	aircraftReminderHoursPerDay   = 24
)

// ErrInvalidAircraftReminder is returned when a reminder fails validation.
var ErrInvalidAircraftReminder = errors.New("invalid aircraft reminder")

// IsValid reports whether k is a known reminder kind.
func (k AircraftReminderKind) IsValid() bool {
	switch k {
	case ReminderKindAnnualInspection, ReminderKindInsurance, ReminderKindRescueSystemRepack,
		ReminderKindRescueRocketExpiry, ReminderKindARC, ReminderKindELTBattery, ReminderKindCustom:
		return true
	}
	return false
}

// AircraftReminder is a dated item on one of the user's aircraft.
// AircraftRegistration is read from the aircraft and never stored.
type AircraftReminder struct {
	ID                   uuid.UUID            `json:"id"`
	UserID               uuid.UUID            `json:"userId"`
	AircraftID           uuid.UUID            `json:"aircraftId"`
	AircraftRegistration string               `json:"aircraftRegistration"`
	Kind                 AircraftReminderKind `json:"kind"`
	Label                *string              `json:"label,omitempty"`
	DueDate              time.Time            `json:"dueDate"`
	IntervalMonths       *int                 `json:"intervalMonths,omitempty"`
	LastDoneOn           *time.Time           `json:"lastDoneOn,omitempty"`
	Notes                *string              `json:"notes,omitempty"`
	CreatedAt            time.Time            `json:"createdAt"`
	UpdatedAt            time.Time            `json:"updatedAt"`
}

// Validate checks kind, label, interval and text lengths. A blank label is
// normalised to nil.
func (r *AircraftReminder) Validate() error {
	if !r.Kind.IsValid() {
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidAircraftReminder, r.Kind)
	}
	if r.Label != nil {
		trimmed := strings.TrimSpace(*r.Label)
		if trimmed == "" {
			r.Label = nil
		} else {
			r.Label = &trimmed
		}
	}
	if r.Kind == ReminderKindCustom && r.Label == nil {
		return fmt.Errorf("%w: label is required for kind CUSTOM", ErrInvalidAircraftReminder)
	}
	if err := ValidateOptionalStringLength("label", r.Label, AircraftReminderLabelMaxLen); err != nil {
		return err
	}
	if err := ValidateOptionalStringLength("notes", r.Notes, AircraftReminderNotesMaxLen); err != nil {
		return err
	}
	if r.IntervalMonths != nil && (*r.IntervalMonths < AircraftReminderMinInterval || *r.IntervalMonths > AircraftReminderMaxInterval) {
		return fmt.Errorf("%w: intervalMonths must be between %d and %d",
			ErrInvalidAircraftReminder, AircraftReminderMinInterval, AircraftReminderMaxInterval)
	}
	if r.DueDate.IsZero() {
		return fmt.Errorf("%w: dueDate is required", ErrInvalidAircraftReminder)
	}
	return nil
}

// DateOnly returns t's calendar date at 00:00 UTC.
func DateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// DaysUntilDue returns whole calendar days from today to the due date;
// negative when overdue.
func (r *AircraftReminder) DaysUntilDue(today time.Time) int {
	diff := DateOnly(r.DueDate).Sub(DateOnly(today))
	return int(diff.Hours() / aircraftReminderHoursPerDay)
}

// Status returns ok, due_soon (within 30 days, including today) or overdue.
func (r *AircraftReminder) Status(today time.Time) string {
	days := r.DaysUntilDue(today)
	switch {
	case days < 0:
		return AircraftReminderStatusOverdue
	case days <= AircraftReminderDueSoonDays:
		return AircraftReminderStatusDueSoon
	default:
		return AircraftReminderStatusOK
	}
}

// DisplayName returns the label, or the kind when there is none.
func (r *AircraftReminder) DisplayName() string {
	if r.Label != nil && *r.Label != "" {
		return *r.Label
	}
	return string(r.Kind)
}

// AddMonthsClamped adds n calendar months to d, clamping the day to the last
// day of the target month (Jan 31 + 1 month = Feb 28/29).
func AddMonthsClamped(d time.Time, n int) time.Time {
	d = DateOnly(d)
	firstOfTarget := time.Date(d.Year(), d.Month()+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	lastDay := firstOfTarget.AddDate(0, 1, -1).Day()
	day := d.Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(firstOfTarget.Year(), firstOfTarget.Month(), day, 0, 0, 0, 0, time.UTC)
}
