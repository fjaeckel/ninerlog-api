package models

import (
	"time"

	"github.com/google/uuid"
)

// BackupSchedule controls how often the scheduler runs a destination.
type BackupSchedule string

const (
	BackupScheduleManual  BackupSchedule = "manual"
	BackupScheduleDaily   BackupSchedule = "daily"
	BackupScheduleWeekly  BackupSchedule = "weekly"
	BackupScheduleMonthly BackupSchedule = "monthly"
)

// IsValid reports whether the schedule is a known value.
func (s BackupSchedule) IsValid() bool {
	switch s {
	case BackupScheduleManual, BackupScheduleDaily, BackupScheduleWeekly, BackupScheduleMonthly:
		return true
	default:
		return false
	}
}

// BackupDestinationStatus is the operational state of a destination.
type BackupDestinationStatus string

const (
	BackupStatusActive BackupDestinationStatus = "active"
	BackupStatusPaused BackupDestinationStatus = "paused"
	BackupStatusError  BackupDestinationStatus = "error"
)

// IsValid reports whether the status is a known value.
func (s BackupDestinationStatus) IsValid() bool {
	switch s {
	case BackupStatusActive, BackupStatusPaused, BackupStatusError:
		return true
	default:
		return false
	}
}

// BackupRunStatus is the outcome of a single backup run.
type BackupRunStatus string

const (
	BackupRunStatusSuccess BackupRunStatus = "success"
	BackupRunStatusFailed  BackupRunStatus = "failed"
	BackupRunStatusSkipped BackupRunStatus = "skipped"
)

// BackupRunTrigger captures whether a run was scheduled or user-triggered.
type BackupRunTrigger string

const (
	BackupRunTriggerScheduled BackupRunTrigger = "scheduled"
	BackupRunTriggerManual    BackupRunTrigger = "manual"
)

// BackupDestination is a user-owned cloud-storage target where NinerLog will
// upload JSON backups. Each destination is bound to one provider plugin (e.g.
// "s3") and carries provider-specific non-secret config in Config plus an
// encrypted credential blob.
type BackupDestination struct {
	ID                 uuid.UUID
	UserID             uuid.UUID
	Provider           string
	DisplayName        string
	Config             map[string]any
	CredentialHint     string
	CredentialsEnc     []byte
	CredentialsNonce   []byte
	Schedule           BackupSchedule
	ScheduleHourUTC    int
	ScheduleDayOfWeek  *int
	ScheduleDayOfMonth *int
	RetentionCount     int
	Status             BackupDestinationStatus
	LastError          string
	Enabled            bool
	ConsecutiveFailures int
	LastRunAt          *time.Time
	LastSuccessAt      *time.Time
	LastSuccessSHA256  string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// BackupRun is the audit record of a single backup attempt.
type BackupRun struct {
	ID              uuid.UUID
	DestinationID   uuid.UUID
	UserID          uuid.UUID
	Status          BackupRunStatus
	Trigger         BackupRunTrigger
	StartedAt       time.Time
	CompletedAt     *time.Time
	DurationMs      *int
	SizeBytes       *int64
	SHA256          string
	FlightCount     *int
	AircraftCount   *int
	LicenseCount    *int
	CredentialCount *int
	RemotePath      string
	ErrorMessage    string
	CreatedAt       time.Time
}
