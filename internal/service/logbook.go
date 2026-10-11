package service

import (
	"context"
	"errors"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service/logbookrules"
	"github.com/google/uuid"
)

var (
	// ErrStrictModeRequired is returned when an action needs strict logbook mode on.
	ErrStrictModeRequired = errors.New("strict logbook mode is off")
	// ErrCertificationInFuture is returned when a certification date is after today.
	ErrCertificationInFuture = errors.New("certification date is in the future")
	// ErrCertificationBeforeLatest is returned when a certification date is
	// earlier than the latest one.
	ErrCertificationBeforeLatest = errors.New("certification date is before the latest certification")
)

// LogbookService owns strict logbook mode, the licence rule registry lookups,
// correction classification, certifications and revision history.
type LogbookService struct {
	repo        repository.LogbookRepository
	licenseRepo repository.LicenseRepository
	flightRepo  repository.FlightRepository
	sigRepo     repository.FlightSignatureRepository

	now func() time.Time
}

// NewLogbookService creates the logbook service.
func NewLogbookService(
	repo repository.LogbookRepository,
	licenseRepo repository.LicenseRepository,
	flightRepo repository.FlightRepository,
	sigRepo repository.FlightSignatureRepository,
) *LogbookService {
	return &LogbookService{
		repo:        repo,
		licenseRepo: licenseRepo,
		flightRepo:  flightRepo,
		sigRepo:     sigRepo,
		now:         time.Now,
	}
}

// ModeView is a user's logbook mode with the licences that bear on it.
type ModeView struct {
	Strict                 bool
	Since                  *time.Time
	DraftWindow            logbookrules.Window
	OverridesDownwardOnly  bool
	LatestCertifiedThrough *time.Time
	Required               []logbookrules.Match
	Suggested              []logbookrules.Match
	Events                 []models.LogbookModeEvent
}

func (s *LogbookService) assess(ctx context.Context, userID uuid.UUID) (logbookrules.Assessment, error) {
	licences, err := s.licenseRepo.GetByUserID(ctx, userID, nil)
	if err != nil {
		return logbookrules.Assessment{}, err
	}
	rules := make([]logbookrules.Licence, 0, len(licences))
	for _, l := range licences {
		country := ""
		if l.IssuingCountry != nil {
			country = *l.IssuingCountry
		}
		rules = append(rules, logbookrules.Licence{
			ID:                  l.ID,
			RegulatoryAuthority: l.RegulatoryAuthority,
			LicenseType:         l.LicenseType,
			IssuingCountry:      country,
		})
	}
	return logbookrules.Assess(rules), nil
}

// GetMode returns the user's logbook mode.
func (s *LogbookService) GetMode(ctx context.Context, userID uuid.UUID) (*ModeView, error) {
	mode, err := s.repo.GetMode(ctx, userID)
	if err != nil {
		return nil, err
	}
	assessment, err := s.assess(ctx, userID)
	if err != nil {
		return nil, err
	}
	events, err := s.repo.ListModeEvents(ctx, userID)
	if err != nil {
		return nil, err
	}
	latest, err := s.repo.LatestCertifiedThrough(ctx, userID)
	if err != nil {
		return nil, err
	}

	view := &ModeView{
		Strict:                 mode.Strict,
		Required:               assessment.Required,
		Suggested:              assessment.Suggested,
		Events:                 events,
		LatestCertifiedThrough: latest,
	}
	if mode.Strict {
		view.Since = mode.Since
		view.DraftWindow = assessment.Window
		if view.DraftWindow == "" {
			view.DraftWindow = logbookrules.WindowFlightDay
		}
		view.OverridesDownwardOnly = assessment.OverridesDownwardOnly
	}
	return view, nil
}

// SetStrict switches strict mode by the user's own choice.
func (s *LogbookService) SetStrict(ctx context.Context, userID uuid.UUID, strict bool) (*ModeView, error) {
	if _, err := s.repo.SetStrict(ctx, userID, strict, models.LogbookTriggerManual, nil); err != nil {
		return nil, err
	}
	return s.GetMode(ctx, userID)
}

// SyncFromLicences switches strict mode on when a licence of the user requires
// it. It never switches it off. licenseID is the licence that was just saved.
func (s *LogbookService) SyncFromLicences(ctx context.Context, userID, licenseID uuid.UUID) error {
	mode, err := s.repo.GetMode(ctx, userID)
	if err != nil {
		return err
	}
	if mode.Strict {
		return nil
	}
	assessment, err := s.assess(ctx, userID)
	if err != nil {
		return err
	}
	if len(assessment.Required) == 0 {
		return nil
	}
	_, err = s.repo.SetStrict(ctx, userID, true, models.LogbookTriggerLicence, &licenseID)
	return err
}

// IsStrict reports whether the user is in strict mode.
func (s *LogbookService) IsStrict(ctx context.Context, userID uuid.UUID) (bool, error) {
	mode, err := s.repo.GetMode(ctx, userID)
	if err != nil {
		return false, err
	}
	return mode.Strict, nil
}

// ListCertifications returns the user's certifications, newest first.
func (s *LogbookService) ListCertifications(ctx context.Context, userID uuid.UUID) ([]models.LogbookCertification, error) {
	return s.repo.ListCertifications(ctx, userID)
}

// CreateCertification certifies the logbook through the given date.
func (s *LogbookService) CreateCertification(ctx context.Context, userID uuid.UUID, through time.Time) (*models.LogbookCertification, error) {
	mode, err := s.repo.GetMode(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !mode.Strict {
		return nil, ErrStrictModeRequired
	}
	through = dateOnly(through)
	if through.After(dateOnly(s.now())) {
		return nil, ErrCertificationInFuture
	}
	latest, err := s.repo.LatestCertifiedThrough(ctx, userID)
	if err != nil {
		return nil, err
	}
	if latest != nil && through.Before(dateOnly(*latest)) {
		return nil, ErrCertificationBeforeLatest
	}
	cert := &models.LogbookCertification{UserID: userID, CertifiedThrough: through}
	if err := s.repo.CreateCertification(ctx, cert); err != nil {
		return nil, err
	}
	return cert, nil
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
