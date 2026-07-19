package service

import (
	"context"
	"errors"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrAircraftNotFound      = errors.New("aircraft not found")
	ErrUnauthorizedAircraft  = errors.New("unauthorized access to aircraft")
	ErrDuplicateRegistration = errors.New("aircraft registration already exists")
)

type AircraftService struct {
	aircraftRepo repository.AircraftRepository
}

func NewAircraftService(aircraftRepo repository.AircraftRepository) *AircraftService {
	return &AircraftService{aircraftRepo: aircraftRepo}
}

func (s *AircraftService) CreateAircraft(ctx context.Context, aircraft *models.Aircraft) error {
	if err := aircraft.Validate(); err != nil {
		return err
	}
	if err := models.ValidateAircraftTextFields(aircraft); err != nil {
		return err
	}
	err := s.aircraftRepo.Create(ctx, aircraft)
	if errors.Is(err, repository.ErrDuplicateRegistration) {
		return ErrDuplicateRegistration
	}
	return err
}

func (s *AircraftService) GetAircraft(ctx context.Context, id, userID uuid.UUID) (*models.Aircraft, error) {
	aircraft, err := s.aircraftRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrAircraftNotFound
		}
		return nil, err
	}
	if aircraft.UserID != userID {
		return nil, ErrUnauthorizedAircraft
	}
	return aircraft, nil
}

func (s *AircraftService) ListAircraft(ctx context.Context, userID uuid.UUID) ([]*models.Aircraft, error) {
	return s.aircraftRepo.GetByUserID(ctx, userID)
}

// UpdateAircraft updates an aircraft. When renameFlights is true and the
// registration changes, flights logged under the old registration are
// repointed to the new one; the returned count is the number of flights
// updated.
func (s *AircraftService) UpdateAircraft(ctx context.Context, aircraft *models.Aircraft, userID uuid.UUID, renameFlights bool) (int, error) {
	existing, err := s.aircraftRepo.GetByID(ctx, aircraft.ID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return 0, ErrAircraftNotFound
		}
		return 0, err
	}
	if existing.UserID != userID {
		return 0, ErrUnauthorizedAircraft
	}
	if err := aircraft.Validate(); err != nil {
		return 0, err
	}
	if err := models.ValidateAircraftTextFields(aircraft); err != nil {
		return 0, err
	}

	flightsUpdated := 0
	if renameFlights && existing.Registration != aircraft.Registration {
		flightsUpdated, err = s.aircraftRepo.UpdateWithFlightRename(ctx, aircraft, existing.Registration)
	} else {
		err = s.aircraftRepo.Update(ctx, aircraft)
	}
	if errors.Is(err, repository.ErrDuplicateRegistration) {
		return 0, ErrDuplicateRegistration
	}
	if err != nil {
		return 0, err
	}
	return flightsUpdated, nil
}

func (s *AircraftService) DeleteAircraft(ctx context.Context, id, userID uuid.UUID) error {
	aircraft, err := s.aircraftRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrAircraftNotFound
		}
		return err
	}
	if aircraft.UserID != userID {
		return ErrUnauthorizedAircraft
	}
	return s.aircraftRepo.Delete(ctx, id)
}

func (s *AircraftService) CountAircraft(ctx context.Context, userID uuid.UUID) (int, error) {
	return s.aircraftRepo.CountByUserID(ctx, userID)
}

// AircraftStatsResult bundles per-registration and per-type flight statistics
type AircraftStatsResult struct {
	ByRegistration []*models.AircraftStats
	ByType         []*models.AircraftTypeStats
}

// GetAircraftStats returns flight statistics aggregated per registration and
// per aircraft type, including informational 90-day landing recency
func (s *AircraftService) GetAircraftStats(ctx context.Context, userID uuid.UUID) (*AircraftStatsResult, error) {
	regStats, err := s.aircraftRepo.GetStatsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	typeStats, err := s.aircraftRepo.GetTypeStatsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	recencyRows, err := s.aircraftRepo.GetRecencyRowsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	applyRecency(regStats, typeStats, recencyRows)
	return &AircraftStatsResult{ByRegistration: regStats, ByType: typeStats}, nil
}

// recencyAgg accumulates 90-day landing counts for one registration or type
type recencyAgg struct {
	landings int
	lapsesOn *time.Time
}

// add processes one per-day row; rows must arrive newest-first so the lapse
// date anchors on the day the cumulative count first reaches 3 (the 3rd-most
// recent landing), which stays countable for 90 days.
func (a *recencyAgg) add(date time.Time, landings int) {
	if landings <= 0 {
		return
	}
	a.landings += landings
	if a.lapsesOn == nil && a.landings >= 3 {
		d := date.AddDate(0, 0, 90)
		a.lapsesOn = &d
	}
}

// applyRecency fills the 90-day recency fields on the stats slices from
// per-day landing rows (newest first, preceding 90 days only)
func applyRecency(regStats []*models.AircraftStats, typeStats []*models.AircraftTypeStats, rows []*models.AircraftRecencyRow) {
	byReg := make(map[string]*recencyAgg)
	byType := make(map[string]*recencyAgg)
	for _, row := range rows {
		if byReg[row.Registration] == nil {
			byReg[row.Registration] = &recencyAgg{}
		}
		byReg[row.Registration].add(row.Date, row.Landings)
		if byType[row.AircraftType] == nil {
			byType[row.AircraftType] = &recencyAgg{}
		}
		byType[row.AircraftType].add(row.Date, row.Landings)
	}
	for _, s := range regStats {
		if agg := byReg[s.Registration]; agg != nil {
			s.LandingsLast90Days = agg.landings
			s.RecencyLapsesOn = agg.lapsesOn
		}
	}
	for _, s := range typeStats {
		if agg := byType[s.AircraftType]; agg != nil {
			s.LandingsLast90Days = agg.landings
			s.RecencyLapsesOn = agg.lapsesOn
		}
	}
}
