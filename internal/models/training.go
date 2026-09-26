package models

import "errors"

// TrainingProgrammeID identifies a syllabus template.
type TrainingProgrammeID string

const (
	TrainingSPL             TrainingProgrammeID = "SPL"
	TrainingSPLTMGExtension TrainingProgrammeID = "SPL_TMG_EXTENSION"
	TrainingULThreeAxis     TrainingProgrammeID = "UL_THREE_AXIS"
	TrainingULWeightShift   TrainingProgrammeID = "UL_WEIGHT_SHIFT"
)

// ErrUnknownTrainingProgramme is returned for a programme ID that is not a template.
var ErrUnknownTrainingProgramme = errors.New("unknown training programme")

// AllTrainingProgrammes returns every programme in response order.
func AllTrainingProgrammes() []TrainingProgrammeID {
	return []TrainingProgrammeID{TrainingSPL, TrainingSPLTMGExtension, TrainingULThreeAxis, TrainingULWeightShift}
}

// IsValid reports whether id is a known programme.
func (id TrainingProgrammeID) IsValid() bool {
	for _, p := range AllTrainingProgrammes() {
		if p == id {
			return true
		}
	}
	return false
}

// TrainingUnit is the unit of a training item's required and current values.
type TrainingUnit string

const (
	TrainingUnitMinutes  TrainingUnit = "minutes"
	TrainingUnitLaunches TrainingUnit = "launches"
	TrainingUnitLandings TrainingUnit = "landings"
	TrainingUnitFlights  TrainingUnit = "flights"
	TrainingUnitKM       TrainingUnit = "km"
)

// TrainingItem is one syllabus requirement and the pilot's progress toward it.
type TrainingItem struct {
	Key           string       `json:"key"`
	Required      int          `json:"required"`
	Current       int          `json:"current"`
	Unit          TrainingUnit `json:"unit"`
	Met           bool         `json:"met"`
	Informational bool         `json:"informational"`
	MessageKey    string       `json:"messageKey"`
}

// TrainingProgramme is a syllabus template evaluated against the pilot's flights.
type TrainingProgramme struct {
	ID            TrainingProgrammeID `json:"id"`
	Discipline    Discipline          `json:"discipline"`
	TitleKey      string              `json:"titleKey"`
	LegalBasis    string              `json:"legalBasis"`
	Items         []TrainingItem      `json:"items"`
	AllMet        bool                `json:"allMet"`
	SignedFlights int                 `json:"signedFlights"`
}

// TrainingProgress is the GET /training/progress response.
type TrainingProgress struct {
	Programmes []TrainingProgramme `json:"programmes"`
}
