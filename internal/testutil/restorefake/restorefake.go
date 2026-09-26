// Package restorefake is an in-memory repository.RestoreStore for tests. A
// failed transaction restores the state it started from, like a rollback.
package restorefake

import (
	"context"
	"sync"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// State is the data the store holds for the single user under test.
type State struct {
	Aircraft    []*models.Aircraft
	Licenses    []*models.License
	Ratings     []*models.ClassRating
	Credentials []*models.Credential
	Privileges  []*models.LicencePrivilege
	Reminders   []*models.AircraftReminder
	Contacts    []*models.Contact
	Flights     []*models.Flight
	Crew        []models.FlightCrewMember
	Rules       []*models.CustomCurrencyRule
	Reports     []*models.CustomReport
	Prefs       *models.NotificationPreferences
	Baseline    *models.FlightBaseline
	Profile     *models.PilotProfile

	Strict         bool
	StrictSince    *time.Time
	ModeEvents     []models.LogbookModeEvent
	Revisions      []models.FlightRevision
	Certifications []models.LogbookCertification
}

func (s State) clone() State {
	return State{
		Aircraft:    append([]*models.Aircraft(nil), s.Aircraft...),
		Licenses:    append([]*models.License(nil), s.Licenses...),
		Ratings:     append([]*models.ClassRating(nil), s.Ratings...),
		Credentials: append([]*models.Credential(nil), s.Credentials...),
		Privileges:  append([]*models.LicencePrivilege(nil), s.Privileges...),
		Reminders:   append([]*models.AircraftReminder(nil), s.Reminders...),
		Contacts:    append([]*models.Contact(nil), s.Contacts...),
		Flights:     append([]*models.Flight(nil), s.Flights...),
		Crew:        append([]models.FlightCrewMember(nil), s.Crew...),
		Rules:       append([]*models.CustomCurrencyRule(nil), s.Rules...),
		Reports:     append([]*models.CustomReport(nil), s.Reports...),
		Prefs:       s.Prefs,
		Baseline:    s.Baseline,
		Profile:     s.Profile,

		Strict:         s.Strict,
		StrictSince:    s.StrictSince,
		ModeEvents:     append([]models.LogbookModeEvent(nil), s.ModeEvents...),
		Revisions:      append([]models.FlightRevision(nil), s.Revisions...),
		Certifications: append([]models.LogbookCertification(nil), s.Certifications...),
	}
}

// Store is the fake. Set FailAt to inject an error into the nth call (1-based,
// counted per transaction) of an operation, named as in the Op constants.
type Store struct {
	mu sync.Mutex

	State       State
	Attachments repository.RestoreAttachments
	FailAt      func(op string, n int) error

	Commits   int
	Rollbacks int

	// ChangeContext is the change context carried into the last transaction.
	ChangeContext repository.ChangeContext

	// ForeignFlightIDs are flight ids held by another user.
	ForeignFlightIDs map[uuid.UUID]bool
	// WipeCorrections are the flight ids the last wipe was told to delete as
	// corrections.
	WipeCorrections []uuid.UUID
	// TxReads counts the strict-mode and licence reads served through Repos.
	TxReads int
}

// Operation names accepted by FailAt.
const (
	OpCountContent     = "count_content"
	OpCountAttachments = "count_attachments"
	OpWipe             = "wipe"
	OpAircraft         = "insert_aircraft"
	OpLicense          = "insert_license"
	OpClassRating      = "insert_class_rating"
	OpPrivilege        = "insert_licence_privilege"
	OpReminder         = "insert_aircraft_reminder"
	OpCredential       = "insert_credential" // #nosec G101 -- an operation label, not a credential.
	OpRule             = "insert_custom_currency_rule"
	OpReport           = "insert_custom_report"
	OpContacts         = "insert_contacts"
	OpFlights          = "insert_flights"
	OpCrew             = "insert_crew_members"
	OpPrefs            = "upsert_notification_preferences"
	OpBaseline         = "upsert_flight_baseline"
	OpPilotProfile     = "upsert_pilot_profile"
	OpSetStrict        = "set_strict"
	OpModeEvent        = "import_mode_event"
	OpRevisions        = "import_revisions"
	OpCertification    = "import_certification"
)

// New creates an empty store.
func New() *Store { return &Store{} }

// InRestoreTx implements repository.RestoreStore.
func (s *Store) InRestoreTx(ctx context.Context, userID uuid.UUID, fn func(tx repository.RestoreTx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	before := s.State.clone()
	s.ChangeContext, _ = repository.ChangeContextFrom(ctx)
	tx := &tx{store: s, calls: map[string]int{}}
	if err := fn(tx); err != nil {
		s.State = before
		s.Rollbacks++
		return err
	}
	s.Commits++
	return nil
}

type tx struct {
	store *Store
	calls map[string]int
}

func (t *tx) hit(op string) error {
	t.calls[op]++
	if t.store.FailAt != nil {
		return t.store.FailAt(op, t.calls[op])
	}
	return nil
}

func (t *tx) CountContent(ctx context.Context, userID uuid.UUID) (repository.RestoreContent, error) {
	if err := t.hit(OpCountContent); err != nil {
		return repository.RestoreContent{}, err
	}
	st := t.store.State
	return repository.RestoreContent{
		Flights:             len(st.Flights) > 0,
		Aircraft:            len(st.Aircraft) > 0,
		Licenses:            len(st.Licenses) > 0,
		Credentials:         len(st.Credentials) > 0,
		Contacts:            len(st.Contacts) > 0,
		CustomCurrencyRules: len(st.Rules) > 0,
		CustomReports:       len(st.Reports) > 0,
	}, nil
}

func (t *tx) CountAttachments(ctx context.Context, userID uuid.UUID) (repository.RestoreAttachments, error) {
	if err := t.hit(OpCountAttachments); err != nil {
		return repository.RestoreAttachments{}, err
	}
	return t.store.Attachments, nil
}

func (t *tx) WipeContent(ctx context.Context, userID uuid.UUID, correctionFlightIDs []uuid.UUID) error {
	if err := t.hit(OpWipe); err != nil {
		return err
	}
	t.store.WipeCorrections = correctionFlightIDs
	st := &t.store.State
	*st = State{
		Prefs: st.Prefs, Baseline: st.Baseline, Profile: st.Profile,
		Strict: st.Strict, StrictSince: st.StrictSince,
		ModeEvents: st.ModeEvents, Revisions: st.Revisions, Certifications: st.Certifications,
	}
	return nil
}

func (t *tx) ListFlightTimings(ctx context.Context, userID uuid.UUID) ([]*models.Flight, error) {
	return append([]*models.Flight(nil), t.store.State.Flights...), nil
}

func (t *tx) FlightIDUsage(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (repository.FlightIDUsage, error) {
	usage := repository.FlightIDUsage{Taken: map[uuid.UUID]bool{}, Voided: map[uuid.UUID]bool{}}
	for _, id := range ids {
		if t.store.ForeignFlightIDs[id] {
			usage.Taken[id] = true
		}
	}
	for _, f := range t.store.State.Flights {
		usage.Taken[f.ID] = true
	}
	for _, r := range t.store.State.Revisions {
		if r.Kind == models.RevisionKindDelete {
			usage.Voided[r.FlightID] = true
		}
	}
	return usage, nil
}

func ensureID(id *uuid.UUID) {
	if *id == uuid.Nil {
		*id = uuid.New()
	}
}

func (t *tx) InsertAircraft(ctx context.Context, a *models.Aircraft) error {
	if err := t.hit(OpAircraft); err != nil {
		return err
	}
	ensureID(&a.ID)
	t.store.State.Aircraft = append(t.store.State.Aircraft, a)
	return nil
}

func (t *tx) InsertLicense(ctx context.Context, l *models.License) error {
	if err := t.hit(OpLicense); err != nil {
		return err
	}
	ensureID(&l.ID)
	t.store.State.Licenses = append(t.store.State.Licenses, l)
	return nil
}

func (t *tx) InsertClassRating(ctx context.Context, cr *models.ClassRating) error {
	if err := t.hit(OpClassRating); err != nil {
		return err
	}
	ensureID(&cr.ID)
	t.store.State.Ratings = append(t.store.State.Ratings, cr)
	return nil
}

func (t *tx) InsertLicencePrivilege(ctx context.Context, p *models.LicencePrivilege) error {
	if err := t.hit(OpPrivilege); err != nil {
		return err
	}
	ensureID(&p.ID)
	t.store.State.Privileges = append(t.store.State.Privileges, p)
	return nil
}

func (t *tx) InsertAircraftReminder(ctx context.Context, r *models.AircraftReminder) error {
	if err := t.hit(OpReminder); err != nil {
		return err
	}
	ensureID(&r.ID)
	t.store.State.Reminders = append(t.store.State.Reminders, r)
	return nil
}

func (t *tx) InsertCredential(ctx context.Context, c *models.Credential) error {
	if err := t.hit(OpCredential); err != nil {
		return err
	}
	ensureID(&c.ID)
	t.store.State.Credentials = append(t.store.State.Credentials, c)
	return nil
}

func (t *tx) InsertCustomCurrencyRule(ctx context.Context, r *models.CustomCurrencyRule) error {
	if err := t.hit(OpRule); err != nil {
		return err
	}
	ensureID(&r.ID)
	t.store.State.Rules = append(t.store.State.Rules, r)
	return nil
}

func (t *tx) InsertCustomReport(ctx context.Context, r *models.CustomReport) error {
	if err := t.hit(OpReport); err != nil {
		return err
	}
	ensureID(&r.ID)
	r.Position = len(t.store.State.Reports)
	r.CreatedAt = time.Now()
	t.store.State.Reports = append(t.store.State.Reports, r)
	return nil
}

func (t *tx) InsertContacts(ctx context.Context, contacts []*models.Contact) error {
	if err := t.hit(OpContacts); err != nil {
		return err
	}
	t.store.State.Contacts = append(t.store.State.Contacts, contacts...)
	return nil
}

func (t *tx) InsertFlights(ctx context.Context, flights []*models.Flight) error {
	if err := t.hit(OpFlights); err != nil {
		return err
	}
	t.store.State.Flights = append(t.store.State.Flights, flights...)
	return nil
}

func (t *tx) InsertCrewMembers(ctx context.Context, members []models.FlightCrewMember) error {
	if err := t.hit(OpCrew); err != nil {
		return err
	}
	t.store.State.Crew = append(t.store.State.Crew, members...)
	return nil
}

func (t *tx) UpsertNotificationPreferences(ctx context.Context, p *models.NotificationPreferences) error {
	if err := t.hit(OpPrefs); err != nil {
		return err
	}
	t.store.State.Prefs = p
	return nil
}

func (t *tx) UpsertFlightBaseline(ctx context.Context, b *models.FlightBaseline) error {
	if err := t.hit(OpBaseline); err != nil {
		return err
	}
	t.store.State.Baseline = b
	return nil
}

func (t *tx) UpsertPilotProfile(ctx context.Context, p *models.PilotProfile) error {
	if err := t.hit(OpPilotProfile); err != nil {
		return err
	}
	t.store.State.Profile = p
	return nil
}

func (t *tx) SetStrict(ctx context.Context, userID uuid.UUID, strict bool, trigger string, licenseID *uuid.UUID) (bool, error) {
	if err := t.hit(OpSetStrict); err != nil {
		return false, err
	}
	st := &t.store.State
	if st.Strict == strict {
		return false, nil
	}
	now := time.Now()
	st.Strict = strict
	if strict {
		st.StrictSince = &now
	}
	st.ModeEvents = append(st.ModeEvents, models.LogbookModeEvent{
		ID: uuid.New(), UserID: userID, Strict: strict, Trigger: trigger, LicenseID: licenseID, CreatedAt: now,
	})
	return true, nil
}

func (t *tx) RestoreStrictSince(ctx context.Context, userID uuid.UUID, since time.Time) error {
	if t.store.State.Strict {
		t.store.State.StrictSince = &since
	}
	return nil
}

func (t *tx) ImportModeEvent(ctx context.Context, e *models.LogbookModeEvent) (bool, error) {
	if err := t.hit(OpModeEvent); err != nil {
		return false, err
	}
	for _, existing := range t.store.State.ModeEvents {
		if existing.Strict == e.Strict && existing.Trigger == e.Trigger && existing.CreatedAt.Equal(e.CreatedAt) {
			return false, nil
		}
	}
	ensureID(&e.ID)
	t.store.State.ModeEvents = append(t.store.State.ModeEvents, *e)
	return true, nil
}

func (t *tx) ImportRevisions(ctx context.Context, revs []models.FlightRevision) (int, error) {
	if err := t.hit(OpRevisions); err != nil {
		return 0, err
	}
	stored := 0
next:
	for _, rev := range revs {
		for _, existing := range t.store.State.Revisions {
			if existing.FlightID == rev.FlightID && existing.RevisionNo == rev.RevisionNo {
				continue next
			}
		}
		ensureID(&rev.ID)
		t.store.State.Revisions = append(t.store.State.Revisions, rev)
		stored++
	}
	return stored, nil
}

func (t *tx) ImportCertification(ctx context.Context, c *models.LogbookCertification) (bool, error) {
	if err := t.hit(OpCertification); err != nil {
		return false, err
	}
	for _, existing := range t.store.State.Certifications {
		if existing.CertifiedThrough.Equal(c.CertifiedThrough) && existing.CertifiedAt.Equal(c.CertifiedAt) {
			return false, nil
		}
	}
	ensureID(&c.ID)
	t.store.State.Certifications = append(t.store.State.Certifications, *c)
	return true, nil
}

// Repos implements repository.RestoreTx. The repositories read the state the
// transaction holds and count each read in Store.TxReads.
func (t *tx) Repos() repository.TxRepos { return &repos{store: t.store} }

type repos struct{ store *Store }

func (r *repos) Contacts() repository.ContactRepository { return nil }

func (r *repos) Logbook() repository.LogbookRepository { return &logbookRepo{store: r.store} }

func (r *repos) Licenses() repository.LicenseRepository { return &licenseRepo{store: r.store} }

type logbookRepo struct {
	repository.LogbookRepository
	store *Store
}

func (r *logbookRepo) GetMode(context.Context, uuid.UUID) (*models.LogbookMode, error) {
	r.store.TxReads++
	return &models.LogbookMode{Strict: r.store.State.Strict, Since: r.store.State.StrictSince}, nil
}

func (r *logbookRepo) LatestCertifiedThrough(context.Context, uuid.UUID) (*time.Time, error) {
	r.store.TxReads++
	var latest *time.Time
	for _, c := range r.store.State.Certifications {
		through := c.CertifiedThrough
		if latest == nil || through.After(*latest) {
			latest = &through
		}
	}
	return latest, nil
}

type licenseRepo struct {
	repository.LicenseRepository
	store *Store
}

func (r *licenseRepo) GetByUserID(context.Context, uuid.UUID, *time.Time) ([]*models.License, error) {
	r.store.TxReads++
	return append([]*models.License(nil), r.store.State.Licenses...), nil
}
