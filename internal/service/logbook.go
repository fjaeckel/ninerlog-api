package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightcalc"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/fjaeckel/ninerlog-api/internal/service/logbookrules"
	"github.com/google/uuid"
)

var (
	// ErrStrictModeRequired is returned when an action needs strict logbook mode on.
	ErrStrictModeRequired = errors.New("strict logbook mode is off")
	// ErrCertificationInFuture is returned when a certification date is after
	// today at UTC+14, the latest timezone on Earth.
	ErrCertificationInFuture = errors.New("certification date is in the future")
	// ErrCertificationBeforeLatest is returned when a certification date is
	// earlier than the latest one.
	ErrCertificationBeforeLatest = errors.New("certification date is before the latest certification")
	// ErrCorrectionReasonRequired is returned when a strict-mode edit is a
	// correction and carries no reason.
	ErrCorrectionReasonRequired = errors.New("a correction needs a reason")
	// ErrOverrideExceedsCalculated is returned when an override raises a value
	// above its auto-calculated one under a rule that allows only reductions.
	ErrOverrideExceedsCalculated = errors.New("override exceeds the calculated value")
)

// latestUTCOffset is the UTC offset of the earliest calendar day on Earth.
const latestUTCOffset = 14 * time.Hour

// OverrideExceedsError names the flight fields whose override is too high.
type OverrideExceedsError struct {
	Fields []string
}

func (e *OverrideExceedsError) Error() string {
	return fmt.Sprintf("%v: %s", ErrOverrideExceedsCalculated, strings.Join(e.Fields, ", "))
}

func (e *OverrideExceedsError) Unwrap() error { return ErrOverrideExceedsCalculated }

// LogbookService owns strict logbook mode, the licence rule registry lookups,
// correction classification, certifications and revision history.
type LogbookService struct {
	repo        repository.LogbookRepository
	licenseRepo repository.LicenseRepository
	flightRepo  repository.FlightRepository
	sigRepo     repository.FlightSignatureRepository

	now          func() time.Time
	departureLon func(location string) (float64, bool)
}

// NewLogbookService creates the logbook service.
func NewLogbookService(
	repo repository.LogbookRepository,
	licenseRepo repository.LicenseRepository,
	flightRepo repository.FlightRepository,
	sigRepo repository.FlightSignatureRepository,
) *LogbookService {
	return &LogbookService{
		repo:         repo,
		licenseRepo:  licenseRepo,
		flightRepo:   flightRepo,
		sigRepo:      sigRepo,
		now:          time.Now,
		departureLon: airportLongitude,
	}
}

func airportLongitude(location string) (float64, bool) {
	location = strings.TrimSpace(location)
	if location == "" {
		return 0, false
	}
	ap := airports.LookupCode(location)
	if ap == nil {
		return 0, false
	}
	return ap.Longitude, true
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
	VoidedCount            int
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
	overview, err := s.repo.GetOverview(ctx, userID)
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

	view := &ModeView{
		VoidedCount:            overview.VoidedCount,
		Strict:                 overview.Strict,
		Required:               assessment.Required,
		Suggested:              assessment.Suggested,
		Events:                 events,
		LatestCertifiedThrough: overview.LatestCertifiedThrough,
	}
	if overview.Strict {
		view.Since = overview.Since
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
	if through.After(dateOnly(s.now().UTC().Add(latestUTCOffset))) {
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

// EditClassification is the verdict on a strict-mode edit or delete.
type EditClassification struct {
	Strict       bool
	IsCorrection bool
}

// StrictState is a user's strict-mode state, read once for a write.
type StrictState struct {
	Strict          bool
	Assessment      logbookrules.Assessment
	LatestCertified *time.Time
}

// LoadState reads the user's strict-mode state. For a user who is not in
// strict mode only the mode is read.
func (s *LogbookService) LoadState(ctx context.Context, userID uuid.UUID) (*StrictState, error) {
	mode, err := s.repo.GetMode(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !mode.Strict {
		return &StrictState{}, nil
	}
	assessment, err := s.assess(ctx, userID)
	if err != nil {
		return nil, err
	}
	latest, err := s.repo.LatestCertifiedThrough(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &StrictState{Strict: true, Assessment: assessment, LatestCertified: latest}, nil
}

// ClassifyEdit decides whether updating existing into updated is a
// correction. updated may be nil for a delete, which skips the override rule.
func (s *LogbookService) ClassifyEdit(ctx context.Context, userID uuid.UUID, existing, updated *models.Flight) (EditClassification, error) {
	state, err := s.LoadState(ctx, userID)
	if err != nil {
		return EditClassification{}, err
	}
	return s.ClassifyEditWith(ctx, state, existing, updated)
}

// ClassifyEditWith is ClassifyEdit against an already loaded state.
func (s *LogbookService) ClassifyEditWith(ctx context.Context, state *StrictState, existing, updated *models.Flight) (EditClassification, error) {
	if !state.Strict {
		return EditClassification{}, nil
	}
	result := EditClassification{Strict: true}
	result.IsCorrection = s.outsideWindow(state, existing, updated)

	sigs, err := s.sigRepo.ListByFlightID(ctx, existing.ID)
	if err != nil {
		return result, err
	}
	for _, sig := range sigs {
		if sig.Status == models.SignatureStatusCompleted || sig.Status == models.SignatureStatusVoided {
			result.IsCorrection = true
			break
		}
	}
	return result, nil
}

// outsideWindow applies the signature-independent correction rules: the free
// edit window has closed, the flight lies in a certified range, or an
// override was raised after the flight day. updated may be nil for a delete.
func (s *LogbookService) outsideWindow(state *StrictState, existing, updated *models.Flight) bool {
	now := s.now()
	lon := s.lonOf(existing)

	landedAt, _ := flightcalc.OnBlockInstant(existing)
	if now.After(state.Assessment.Deadline(existing.Date, landedAt, lon)) {
		return true
	}

	if latest := state.LatestCertified; latest != nil {
		earliest := dateOnly(existing.Date)
		if updated != nil && dateOnly(updated.Date).Before(earliest) {
			earliest = dateOnly(updated.Date)
		}
		if !earliest.After(dateOnly(*latest)) {
			return true
		}
	}

	return updated != nil && overridesTouched(existing, updated) &&
		now.After(logbookrules.FlightDayEnd(existing.Date, lon))
}

// DeleteClassifier classifies the deletion of many of a user's flights
// against one loaded strict-mode state.
type DeleteClassifier struct {
	s     *LogbookService
	state *StrictState
}

// NewDeleteClassifier loads the user's strict-mode state. It returns nil when
// the user is not in strict mode.
func (s *LogbookService) NewDeleteClassifier(ctx context.Context, userID uuid.UUID) (*DeleteClassifier, error) {
	state, err := s.LoadState(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !state.Strict {
		return nil, nil
	}
	return &DeleteClassifier{s: s, state: state}, nil
}

// Corrections returns the ids of the flights whose deletion is a correction:
// the free-edit window has closed, the flight lies in a certified range, or it
// has carried a completed or voided signature (SignatureID set).
func (c *DeleteClassifier) Corrections(flights []*models.Flight) []uuid.UUID {
	var ids []uuid.UUID
	for _, f := range flights {
		if f.SignatureID != nil || c.s.outsideWindow(c.state, f, nil) {
			ids = append(ids, f.ID)
		}
	}
	return ids
}

// DeleteCorrections returns the ids of the flights, among those list loads,
// whose deletion is a correction. It reads the strict-mode state through
// repos, the repositories of the transaction that deletes the flights, and
// lists nothing for a user who is not in strict mode.
func (s *LogbookService) DeleteCorrections(ctx context.Context, repos repository.TxRepos, userID uuid.UUID, list repository.FlightLister) ([]uuid.UUID, error) {
	bound := *s
	bound.repo = repos.Logbook()
	bound.licenseRepo = repos.Licenses()
	classifier, err := bound.NewDeleteClassifier(ctx, userID)
	if err != nil || classifier == nil {
		return nil, err
	}
	flights, err := list()
	if err != nil {
		return nil, err
	}
	return classifier.Corrections(flights), nil
}

func (s *LogbookService) lonOf(f *models.Flight) float64 {
	if f.DepartureICAO == nil {
		return 0
	}
	lon, _ := s.departureLon(*f.DepartureICAO)
	return lon
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

type overrideField struct {
	name  string
	flag  func(*models.Flight) *bool
	value func(*models.Flight) *int
}

var overrideFields = []overrideField{
	{"nightTime", func(f *models.Flight) *bool { return &f.NightTimeOverride }, func(f *models.Flight) *int { return &f.NightTime }},
	{"crossCountryTime", func(f *models.Flight) *bool { return &f.CrossCountryTimeOverride }, func(f *models.Flight) *int { return &f.CrossCountryTime }},
	{"takeoffsDay", func(f *models.Flight) *bool { return &f.TakeoffsDayOverride }, func(f *models.Flight) *int { return &f.TakeoffsDay }},
	{"takeoffsNight", func(f *models.Flight) *bool { return &f.TakeoffsNightOverride }, func(f *models.Flight) *int { return &f.TakeoffsNight }},
	{"landingsDay", func(f *models.Flight) *bool { return &f.LandingsDayOverride }, func(f *models.Flight) *int { return &f.LandingsDay }},
	{"landingsNight", func(f *models.Flight) *bool { return &f.LandingsNightOverride }, func(f *models.Flight) *int { return &f.LandingsNight }},
	{"sicTime", func(f *models.Flight) *bool { return &f.SICTimeOverride }, func(f *models.Flight) *int { return &f.SICTime }},
	{"multiPilotTime", func(f *models.Flight) *bool { return &f.MultiPilotTimeOverride }, func(f *models.Flight) *int { return &f.MultiPilotTime }},
}

// overrideChanged reports whether the field is overridden in updated and was
// not, or had another value, in previous (nil previous means a new flight).
func overrideChanged(f overrideField, previous, updated *models.Flight) bool {
	if !*f.flag(updated) {
		return false
	}
	if previous == nil || !*f.flag(previous) {
		return true
	}
	return *f.value(previous) != *f.value(updated)
}

func overridesTouched(previous, updated *models.Flight) bool {
	for _, f := range overrideFields {
		if overrideChanged(f, previous, updated) {
			return true
		}
	}
	return false
}

// CheckDownwardOverrides rejects overrides that exceed the auto-calculated
// value when the user is in strict mode under a rule that allows overrides
// only downward. previous is nil for a new flight; only overrides that are
// new or changed against it are checked.
func (s *LogbookService) CheckDownwardOverrides(ctx context.Context, userID uuid.UUID, previous, flight *models.Flight, userName string, facts *flightrules.AircraftFacts) error {
	if !hasOverride(flight) {
		return nil
	}
	state, err := s.LoadState(ctx, userID)
	if err != nil {
		return err
	}
	return s.CheckDownwardOverridesWith(state, previous, flight, userName, facts)
}

// CheckDownwardOverridesWith is CheckDownwardOverrides against an already
// loaded state.
func (s *LogbookService) CheckDownwardOverridesWith(state *StrictState, previous, flight *models.Flight, userName string, facts *flightrules.AircraftFacts) error {
	if !state.Strict || !state.Assessment.OverridesDownwardOnly || !hasOverride(flight) {
		return nil
	}

	calculated := *flight
	calculated.CrewMembers = append([]models.FlightCrewMember(nil), flight.CrewMembers...)
	for _, f := range overrideFields {
		*f.flag(&calculated) = false
		*f.value(&calculated) = 0
	}
	flightcalc.ApplyAutoCalculations(&calculated, userName, facts)

	var exceeded []string
	for _, f := range overrideFields {
		if overrideChanged(f, previous, flight) && *f.value(flight) > *f.value(&calculated) {
			exceeded = append(exceeded, f.name)
		}
	}
	if len(exceeded) > 0 {
		return &OverrideExceedsError{Fields: exceeded}
	}
	return nil
}

func hasOverride(f *models.Flight) bool {
	for _, o := range overrideFields {
		if *o.flag(f) {
			return true
		}
	}
	return false
}
