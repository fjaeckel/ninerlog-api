package models

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Discipline is a flying discipline a pilot profile resolves a status for.
type Discipline string

const (
	DisciplineAeroplane  Discipline = "AEROPLANE"
	DisciplineTMG        Discipline = "TMG"
	DisciplineSailplane  Discipline = "SAILPLANE"
	DisciplineUltralight Discipline = "ULTRALIGHT"
	DisciplineGyroplane  Discipline = "GYROPLANE"
	DisciplineHelicopter Discipline = "HELICOPTER"
	DisciplineIFR        Discipline = "IFR"
	DisciplineMultiCrew  Discipline = "MULTI_CREW"
	DisciplineInstructor Discipline = "INSTRUCTOR"
	DisciplineSimulator  Discipline = "SIMULATOR"
)

// AllDisciplines returns every discipline in stable enum order.
func AllDisciplines() []Discipline {
	return []Discipline{
		DisciplineAeroplane, DisciplineTMG, DisciplineSailplane, DisciplineUltralight,
		DisciplineGyroplane, DisciplineHelicopter, DisciplineIFR, DisciplineMultiCrew,
		DisciplineInstructor, DisciplineSimulator,
	}
}

// IsValid reports whether d is a known discipline.
func (d Discipline) IsValid() bool {
	for _, v := range AllDisciplines() {
		if v == d {
			return true
		}
	}
	return false
}

// DisciplineIntent is the pilot's stated intent for a discipline.
type DisciplineIntent string

const (
	IntentAuto DisciplineIntent = "auto"
	IntentOn   DisciplineIntent = "on"
	IntentOff  DisciplineIntent = "off"
	IntentGoal DisciplineIntent = "goal"
)

// IsValid reports whether i is a known intent.
func (i DisciplineIntent) IsValid() bool {
	switch i {
	case IntentAuto, IntentOn, IntentOff, IntentGoal:
		return true
	}
	return false
}

// DisciplineStatus is the resolved status of a discipline.
type DisciplineStatus string

const (
	StatusActive   DisciplineStatus = "active"
	StatusTraining DisciplineStatus = "training"
	StatusDormant  DisciplineStatus = "dormant"
	StatusOff      DisciplineStatus = "off"
)

// PilotProfileMode selects adaptive or show-everything display.
type PilotProfileMode string

const (
	ModeAdaptive   PilotProfileMode = "adaptive"
	ModeEverything PilotProfileMode = "everything"
)

// IsValid reports whether m is a known mode.
func (m PilotProfileMode) IsValid() bool {
	return m == ModeAdaptive || m == ModeEverything
}

// EvidenceSource names where a piece of discipline evidence came from.
type EvidenceSource string

const (
	EvidenceLicence            EvidenceSource = "LICENCE"
	EvidenceRating             EvidenceSource = "RATING"
	EvidenceAircraft           EvidenceSource = "AIRCRAFT"
	EvidenceFlights            EvidenceSource = "FLIGHTS"
	EvidenceFlightsDual        EvidenceSource = "FLIGHTS_DUAL"
	EvidenceFlightsInstructing EvidenceSource = "FLIGHTS_INSTRUCTING"
)

// EvidenceStrength grades a piece of discipline evidence.
type EvidenceStrength string

const (
	StrengthStrong  EvidenceStrength = "strong"
	StrengthRecent  EvidenceStrength = "recent"
	StrengthDormant EvidenceStrength = "dormant"
)

// ErrInvalidPilotProfile is returned for an unknown mode, discipline or intent.
var ErrInvalidPilotProfile = errors.New("invalid pilot profile")

// DisciplineSetting is the stored per-discipline intent and acknowledgement.
type DisciplineSetting struct {
	Intent         DisciplineIntent `json:"intent,omitempty"`
	AcknowledgedAt *time.Time       `json:"acknowledgedAt,omitempty"`
}

// EffectiveIntent returns the intent, defaulting to auto.
func (s DisciplineSetting) EffectiveIntent() DisciplineIntent {
	if s.Intent == "" {
		return IntentAuto
	}
	return s.Intent
}

// PilotProfile is the stored pilot profile row.
type PilotProfile struct {
	UserID      uuid.UUID                        `json:"-"`
	Mode        PilotProfileMode                 `json:"mode"`
	Disciplines map[Discipline]DisciplineSetting `json:"disciplines"`
	CreatedAt   time.Time                        `json:"-"`
	UpdatedAt   time.Time                        `json:"-"`
}

// DefaultPilotProfile returns the profile of a user with no stored row.
func DefaultPilotProfile(userID uuid.UUID) *PilotProfile {
	return &PilotProfile{UserID: userID, Mode: ModeAdaptive, Disciplines: map[Discipline]DisciplineSetting{}}
}

// Setting returns the stored setting for d, or the zero setting.
func (p *PilotProfile) Setting(d Discipline) DisciplineSetting {
	if p == nil || p.Disciplines == nil {
		return DisciplineSetting{}
	}
	return p.Disciplines[d]
}

// Validate checks the mode, and every stored discipline and intent.
func (p *PilotProfile) Validate() error {
	if !p.Mode.IsValid() {
		return ErrInvalidPilotProfile
	}
	for d, s := range p.Disciplines {
		if !d.IsValid() || !s.EffectiveIntent().IsValid() {
			return ErrInvalidPilotProfile
		}
	}
	return nil
}

// Compact drops settings that carry neither a non-auto intent nor an acknowledgement.
func (p *PilotProfile) Compact() {
	for d, s := range p.Disciplines {
		if s.EffectiveIntent() == IntentAuto {
			s.Intent = ""
		}
		if s.Intent == "" && s.AcknowledgedAt == nil {
			delete(p.Disciplines, d)
			continue
		}
		p.Disciplines[d] = s
	}
}

// DisciplineEvidence is one piece of evidence behind a discipline's status.
type DisciplineEvidence struct {
	Source   EvidenceSource   `json:"source"`
	Strength EvidenceStrength `json:"strength"`
	Ref      string           `json:"ref"`
	RefID    *uuid.UUID       `json:"refId,omitempty"`
	LastSeen *time.Time       `json:"lastSeen,omitempty"`
}

// DisciplineState is the derived state of one discipline.
type DisciplineState struct {
	Discipline     Discipline           `json:"discipline"`
	Status         DisciplineStatus     `json:"status"`
	Intent         DisciplineIntent     `json:"intent"`
	Evidence       []DisciplineEvidence `json:"evidence"`
	ULKinds        []ULKind             `json:"ulKinds"`
	AcknowledgedAt *time.Time           `json:"acknowledgedAt,omitempty"`
}

// DerivedPilotProfile is a pilot profile with every discipline resolved.
type DerivedPilotProfile struct {
	Mode                   PilotProfileMode  `json:"mode"`
	Disciplines            []DisciplineState `json:"disciplines"`
	PendingAcknowledgement []Discipline      `json:"pendingAcknowledgement"`
}

// DisciplineFlightGroup aggregates a user's flights for one normalised aircraft class and
// ultralight kind. AircraftClass is empty for flights on no fleet aircraft or an unclassed
// one. Every field except the simulator and passenger ones covers non-simulator,
// non-passenger flights only.
type DisciplineFlightGroup struct {
	AircraftClass string
	ULKind        *ULKind

	// Flights, LastFlight and DualReceivedFlights cover flights without a towed launch.
	Flights             int
	LastFlight          *time.Time
	DualReceivedFlights int
	// TowedFlights, LastTowed and TowedDualReceivedFlights cover winch, aerotow, car and
	// bungee launches.
	TowedFlights             int
	LastTowed                *time.Time
	TowedDualReceivedFlights int

	DualReceivedMinutes int
	DualGivenMinutes    int
	ExaminerMinutes     int
	InstructingFlights  int
	LastInstructing     *time.Time

	IFRMinutes int
	Approaches int
	IFRFlights int
	LastIFR    *time.Time

	MultiPilotMinutes int
	SICMinutes        int
	ReliefMinutes     int
	MultiCrewFlights  int
	LastMultiCrew     *time.Time

	SimulatorSessions int
	LastSimulator     *time.Time
	PassengerFlights  int
	LastPassenger     *time.Time
}
