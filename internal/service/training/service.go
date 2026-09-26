// Package training evaluates syllabus templates (SPL, SPL TMG extension, German UL) against
// a pilot's flights.
package training

import (
	"context"
	"fmt"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// ProfileSource derives the pilot profile.
type ProfileSource interface {
	Get(ctx context.Context, userID uuid.UUID) (*models.DerivedPilotProfile, error)
}

// LicenceLister lists a user's licences.
type LicenceLister interface {
	GetByUserID(ctx context.Context, userID uuid.UUID, updatedSince *time.Time) ([]*models.License, error)
}

// Service computes training progress for the caller.
type Service struct {
	profiles ProfileSource
	licences LicenceLister
	flights  repository.TrainingRepository
}

// NewService creates a training progress service.
func NewService(profiles ProfileSource, licences LicenceLister, flights repository.TrainingRepository) *Service {
	return &Service{profiles: profiles, licences: licences, flights: flights}
}

// Progress returns one programme per discipline in status training plus every requested
// programme, in models.AllTrainingProgrammes order. An unknown requested programme returns
// models.ErrUnknownTrainingProgramme.
func (s *Service) Progress(ctx context.Context, userID uuid.UUID, requested []models.TrainingProgrammeID) (*models.TrainingProgress, error) {
	wanted := map[models.TrainingProgrammeID]bool{}
	for _, id := range requested {
		if !id.IsValid() {
			return nil, fmt.Errorf("%w: %q", models.ErrUnknownTrainingProgramme, id)
		}
		wanted[id] = true
	}
	profile, err := s.profiles.Get(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get pilot profile: %w", err)
	}
	for _, id := range ProgrammesInTraining(profile.Disciplines) {
		wanted[id] = true
	}

	out := &models.TrainingProgress{Programmes: []models.TrainingProgramme{}}
	if len(wanted) == 0 {
		return out, nil
	}
	flights, err := s.flights.ListTrainingFlights(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list training flights: %w", err)
	}
	var credit *Credit
	if wanted[models.TrainingSPL] {
		credit, err = s.credit(ctx, userID)
		if err != nil {
			return nil, err
		}
	}
	for _, id := range models.AllTrainingProgrammes() {
		if wanted[id] {
			out.Programmes = append(out.Programmes, Evaluate(id, flights, credit))
		}
	}
	return out, nil
}

// credit returns the SFCL.130(b) credit inputs, nil when the pilot holds no licence for
// another aircraft category.
func (s *Service) credit(ctx context.Context, userID uuid.UUID) (*Credit, error) {
	licences, err := s.licences.GetByUserID(ctx, userID, nil)
	if err != nil {
		return nil, fmt.Errorf("list licences: %w", err)
	}
	held := false
	for _, l := range licences {
		if IsOtherCategoryLicence(models.ClassifyLicence(l.LicenseType, l.RegulatoryAuthority)) {
			held = true
			break
		}
	}
	if !held {
		return nil, nil
	}
	pic, err := s.flights.OtherCategoryPICMinutes(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("other category PIC minutes: %w", err)
	}
	return &Credit{PICMinutes: pic}, nil
}

// IsOtherCategoryLicence reports whether a licence kind is for an aircraft category other
// than sailplanes and balloons (SFCL.130(b)).
func IsOtherCategoryLicence(k models.LicenceKind) bool {
	return k.IsAeroplane() || k == models.LicenceKindGPL || k == models.LicenceKindHelicopter
}

// ProgrammesInTraining maps the disciplines in status training to programmes: SAILPLANE to
// SPL, TMG to SPL_TMG_EXTENSION, ULTRALIGHT to UL_THREE_AXIS and UL_WEIGHT_SHIFT by the
// discipline's UL kinds.
func ProgrammesInTraining(states []models.DisciplineState) []models.TrainingProgrammeID {
	var out []models.TrainingProgrammeID
	for _, st := range states {
		if st.Status != models.StatusTraining {
			continue
		}
		switch st.Discipline {
		case models.DisciplineSailplane:
			out = append(out, models.TrainingSPL)
		case models.DisciplineTMG:
			out = append(out, models.TrainingSPLTMGExtension)
		case models.DisciplineUltralight:
			three, weight := false, false
			for _, k := range st.ULKinds {
				switch k {
				case models.ULKindThreeAxis, models.ULKindThreeAxisMotorglider:
					three = true
				case models.ULKindWeightShift:
					weight = true
				}
			}
			if three {
				out = append(out, models.TrainingULThreeAxis)
			}
			if weight {
				out = append(out, models.TrainingULWeightShift)
			}
		}
	}
	return out
}
