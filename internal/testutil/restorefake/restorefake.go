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
	Contacts    []*models.Contact
	Flights     []*models.Flight
	Crew        []models.FlightCrewMember
	Rules       []*models.CustomCurrencyRule
	Reports     []*models.CustomReport
	Prefs       *models.NotificationPreferences
	Baseline    *models.FlightBaseline
}

func (s State) clone() State {
	return State{
		Aircraft:    append([]*models.Aircraft(nil), s.Aircraft...),
		Licenses:    append([]*models.License(nil), s.Licenses...),
		Ratings:     append([]*models.ClassRating(nil), s.Ratings...),
		Credentials: append([]*models.Credential(nil), s.Credentials...),
		Contacts:    append([]*models.Contact(nil), s.Contacts...),
		Flights:     append([]*models.Flight(nil), s.Flights...),
		Crew:        append([]models.FlightCrewMember(nil), s.Crew...),
		Rules:       append([]*models.CustomCurrencyRule(nil), s.Rules...),
		Reports:     append([]*models.CustomReport(nil), s.Reports...),
		Prefs:       s.Prefs,
		Baseline:    s.Baseline,
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
}

// Operation names accepted by FailAt.
const (
	OpCountContent     = "count_content"
	OpCountAttachments = "count_attachments"
	OpWipe             = "wipe"
	OpAircraft         = "insert_aircraft"
	OpLicense          = "insert_license"
	OpClassRating      = "insert_class_rating"
	OpCredential       = "insert_credential" // #nosec G101 -- an operation label, not a credential.
	OpRule             = "insert_custom_currency_rule"
	OpReport           = "insert_custom_report"
	OpContacts         = "insert_contacts"
	OpFlights          = "insert_flights"
	OpCrew             = "insert_crew_members"
	OpPrefs            = "upsert_notification_preferences"
	OpBaseline         = "upsert_flight_baseline"
)

// New creates an empty store.
func New() *Store { return &Store{} }

// InRestoreTx implements repository.RestoreStore.
func (s *Store) InRestoreTx(ctx context.Context, userID uuid.UUID, fn func(tx repository.RestoreTx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	before := s.State.clone()
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

func (t *tx) WipeContent(ctx context.Context, userID uuid.UUID) error {
	if err := t.hit(OpWipe); err != nil {
		return err
	}
	st := &t.store.State
	prefs, baseline := st.Prefs, st.Baseline
	*st = State{Prefs: prefs, Baseline: baseline}
	return nil
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
