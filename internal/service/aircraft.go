package service

import (
	"context"
	"errors"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/repository"
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

func (s *AircraftService) UpdateAircraft(ctx context.Context, aircraft *models.Aircraft, userID uuid.UUID) error {
	existing, err := s.aircraftRepo.GetByID(ctx, aircraft.ID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrAircraftNotFound
		}
		return err
	}
	if existing.UserID != userID {
		return ErrUnauthorizedAircraft
	}
	if err := aircraft.Validate(); err != nil {
		return err
	}
	err = s.aircraftRepo.Update(ctx, aircraft)
	if errors.Is(err, repository.ErrDuplicateRegistration) {
		return ErrDuplicateRegistration
	}
	return err
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
