package pilotprofile

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// RatingLister lists the class ratings on a licence.
type RatingLister interface {
	GetByLicenseID(ctx context.Context, licenseID uuid.UUID) ([]*models.ClassRating, error)
}

// PrivilegeLister lists a user's licence privileges.
type PrivilegeLister interface {
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*models.LicencePrivilege, error)
}

// Service reads and updates the caller's own pilot profile.
type Service struct {
	profiles   repository.PilotProfileRepository
	evidence   repository.DisciplineEvidenceSource
	licences   repository.LicenseRepository
	ratings    RatingLister
	aircraft   repository.AircraftRepository
	privileges PrivilegeLister
	now        func() time.Time
}

// NewService creates a pilot profile service.
func NewService(
	profiles repository.PilotProfileRepository,
	evidence repository.DisciplineEvidenceSource,
	licences repository.LicenseRepository,
	ratings RatingLister,
	aircraft repository.AircraftRepository,
) *Service {
	return &Service{
		profiles: profiles, evidence: evidence, licences: licences,
		ratings: ratings, aircraft: aircraft, now: time.Now,
	}
}

// SetPrivilegeSource wires the licence privileges. With none wired, privileges give no evidence.
func (s *Service) SetPrivilegeSource(l PrivilegeLister) {
	s.privileges = l
}

// SetClock replaces the service clock.
func (s *Service) SetClock(now func() time.Time) {
	s.now = now
}

// Update is a partial merge into the stored profile.
type Update struct {
	Mode        *models.PilotProfileMode
	Intents     map[models.Discipline]models.DisciplineIntent
	Acknowledge []models.Discipline
}

// Validate returns models.ErrInvalidPilotProfile for an unknown mode, discipline or intent.
func (u Update) Validate() error {
	if u.Mode != nil && !u.Mode.IsValid() {
		return fmt.Errorf("%w: unknown mode %q", models.ErrInvalidPilotProfile, *u.Mode)
	}
	for d, i := range u.Intents {
		if !d.IsValid() {
			return fmt.Errorf("%w: unknown discipline %q", models.ErrInvalidPilotProfile, d)
		}
		if !i.IsValid() {
			return fmt.Errorf("%w: unknown intent %q", models.ErrInvalidPilotProfile, i)
		}
	}
	for _, d := range u.Acknowledge {
		if !d.IsValid() {
			return fmt.Errorf("%w: unknown discipline %q", models.ErrInvalidPilotProfile, d)
		}
	}
	return nil
}

// Settings returns the user's stored profile, or nil when none is stored.
func (s *Service) Settings(ctx context.Context, userID uuid.UUID) (*models.PilotProfile, error) {
	p, err := s.profiles.Get(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Service) settingsOrDefault(ctx context.Context, userID uuid.UUID) (*models.PilotProfile, error) {
	p, err := s.Settings(ctx, userID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return models.DefaultPilotProfile(userID), nil
	}
	if p.Disciplines == nil {
		p.Disciplines = map[models.Discipline]models.DisciplineSetting{}
	}
	return p, nil
}

// Get derives the user's full pilot profile. It never creates a stored row.
func (s *Service) Get(ctx context.Context, userID uuid.UUID) (*models.DerivedPilotProfile, error) {
	settings, err := s.settingsOrDefault(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get pilot profile: %w", err)
	}
	return s.derive(ctx, userID, settings)
}

// Update merges u into the user's stored profile and returns the derived profile.
// Acknowledging keeps an existing acknowledgement time.
func (s *Service) Update(ctx context.Context, userID uuid.UUID, u Update) (*models.DerivedPilotProfile, error) {
	if err := u.Validate(); err != nil {
		return nil, err
	}
	settings, err := s.settingsOrDefault(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get pilot profile: %w", err)
	}
	if u.Mode != nil {
		settings.Mode = *u.Mode
	}
	for d, i := range u.Intents {
		setting := settings.Disciplines[d]
		setting.Intent = i
		settings.Disciplines[d] = setting
	}
	now := s.now().UTC()
	for _, d := range u.Acknowledge {
		setting := settings.Disciplines[d]
		if setting.AcknowledgedAt == nil {
			setting.AcknowledgedAt = &now
		}
		settings.Disciplines[d] = setting
	}
	settings.Compact()
	if err := s.profiles.Upsert(ctx, settings); err != nil {
		return nil, fmt.Errorf("update pilot profile: %w", err)
	}
	return s.derive(ctx, userID, settings)
}

// Replace validates p and stores it as the user's whole profile.
func (s *Service) Replace(ctx context.Context, userID uuid.UUID, p *models.PilotProfile) error {
	stored := &models.PilotProfile{UserID: userID, Mode: p.Mode, Disciplines: map[models.Discipline]models.DisciplineSetting{}}
	if stored.Mode == "" {
		stored.Mode = models.ModeAdaptive
	}
	for d, setting := range p.Disciplines {
		stored.Disciplines[d] = setting
	}
	if err := stored.Validate(); err != nil {
		return err
	}
	stored.Compact()
	return s.profiles.Upsert(ctx, stored)
}

func (s *Service) derive(ctx context.Context, userID uuid.UUID, settings *models.PilotProfile) (*models.DerivedPilotProfile, error) {
	licences, err := s.licences.GetByUserID(ctx, userID, nil)
	if err != nil {
		return nil, fmt.Errorf("list licences: %w", err)
	}
	var ratings []*models.ClassRating
	for _, l := range licences {
		rs, err := s.ratings.GetByLicenseID(ctx, l.ID)
		if err != nil {
			return nil, fmt.Errorf("list class ratings: %w", err)
		}
		ratings = append(ratings, rs...)
	}
	fleet, err := s.aircraft.GetByUserID(ctx, userID, nil)
	if err != nil {
		return nil, fmt.Errorf("list aircraft: %w", err)
	}
	groups, err := s.evidence.GetDisciplineFlightGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	var privileges []*models.LicencePrivilege
	if s.privileges != nil {
		privileges, err = s.privileges.ListByUser(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("list licence privileges: %w", err)
		}
	}

	states := Derive(licences, ratings, privileges, fleet, groups, settings, s.now())
	return &models.DerivedPilotProfile{
		Mode:                   settings.Mode,
		Disciplines:            states,
		PendingAcknowledgement: PendingAcknowledgement(states),
	}, nil
}
