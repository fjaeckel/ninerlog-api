package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

var (
	// ErrAircraftReminderNotFound is returned when a reminder does not exist,
	// belongs to another user, or is not on the named aircraft.
	ErrAircraftReminderNotFound = errors.New("aircraft reminder not found")
	// ErrInvalidReminderQuery is returned for an out-of-range list filter.
	ErrInvalidReminderQuery = errors.New("invalid aircraft reminder query")
)

// MaxReminderDueWithinDays bounds the dueWithinDays list filter.
const MaxReminderDueWithinDays = 3650

// AircraftReminderInput carries the fields of a new reminder.
type AircraftReminderInput struct {
	Kind           models.AircraftReminderKind
	Label          *string
	DueDate        time.Time
	IntervalMonths *int
	LastDoneOn     *time.Time
	Notes          *string
}

// AircraftReminderPatch carries a partial update. A nil pointer leaves the
// field unchanged; a Clear flag sets a nullable field to NULL.
type AircraftReminderPatch struct {
	Kind                *models.AircraftReminderKind
	Label               *string
	ClearLabel          bool
	DueDate             *time.Time
	IntervalMonths      *int
	ClearIntervalMonths bool
	LastDoneOn          *time.Time
	ClearLastDoneOn     bool
	Notes               *string
	ClearNotes          bool
}

// AircraftReminderService owns aircraft reminder rules and ownership checks.
type AircraftReminderService struct {
	repo         repository.AircraftReminderRepository
	aircraftRepo repository.AircraftRepository
	now          func() time.Time
}

// NewAircraftReminderService returns a reminder service.
func NewAircraftReminderService(repo repository.AircraftReminderRepository, aircraftRepo repository.AircraftRepository) *AircraftReminderService {
	return &AircraftReminderService{repo: repo, aircraftRepo: aircraftRepo, now: time.Now}
}

// SetClock replaces the time source.
func (s *AircraftReminderService) SetClock(now func() time.Time) {
	s.now = now
}

// Today returns the current UTC calendar date.
func (s *AircraftReminderService) Today() time.Time {
	return models.DateOnly(s.now().UTC())
}

// ownedAircraft returns the aircraft or ErrAircraftNotFound when it is
// missing or belongs to another user.
func (s *AircraftReminderService) ownedAircraft(ctx context.Context, aircraftID, userID uuid.UUID) (*models.Aircraft, error) {
	ac, err := s.aircraftRepo.GetByID(ctx, aircraftID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrAircraftNotFound
		}
		return nil, fmt.Errorf("get aircraft: %w", err)
	}
	if ac.UserID != userID {
		return nil, ErrAircraftNotFound
	}
	return ac, nil
}

// ownedReminder returns the reminder when the aircraft and the reminder both
// belong to the user and the reminder is on that aircraft.
func (s *AircraftReminderService) ownedReminder(ctx context.Context, aircraftID, reminderID, userID uuid.UUID) (*models.AircraftReminder, error) {
	if _, err := s.ownedAircraft(ctx, aircraftID, userID); err != nil {
		return nil, err
	}
	rem, err := s.repo.GetByID(ctx, reminderID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrAircraftReminderNotFound
		}
		return nil, fmt.Errorf("get aircraft reminder: %w", err)
	}
	if rem.UserID != userID || rem.AircraftID != aircraftID {
		return nil, ErrAircraftReminderNotFound
	}
	return rem, nil
}

// List returns the aircraft's reminders by due date.
func (s *AircraftReminderService) List(ctx context.Context, userID, aircraftID uuid.UUID) ([]*models.AircraftReminder, error) {
	if _, err := s.ownedAircraft(ctx, aircraftID, userID); err != nil {
		return nil, err
	}
	return s.repo.ListByAircraft(ctx, aircraftID)
}

// ListAll returns the user's reminders across aircraft by due date. A
// non-nil dueWithinDays keeps reminders due within that many days of today,
// overdue ones included.
func (s *AircraftReminderService) ListAll(ctx context.Context, userID uuid.UUID, dueWithinDays *int) ([]*models.AircraftReminder, error) {
	var bound *time.Time
	if dueWithinDays != nil {
		if *dueWithinDays < 0 || *dueWithinDays > MaxReminderDueWithinDays {
			return nil, fmt.Errorf("%w: dueWithinDays must be between 0 and %d", ErrInvalidReminderQuery, MaxReminderDueWithinDays)
		}
		b := s.Today().AddDate(0, 0, *dueWithinDays)
		bound = &b
	}
	return s.repo.ListByUser(ctx, userID, bound)
}

// Create validates and stores a reminder on one of the user's aircraft.
func (s *AircraftReminderService) Create(ctx context.Context, userID, aircraftID uuid.UUID, in AircraftReminderInput) (*models.AircraftReminder, error) {
	ac, err := s.ownedAircraft(ctx, aircraftID, userID)
	if err != nil {
		return nil, err
	}
	rem := &models.AircraftReminder{
		UserID:               userID,
		AircraftID:           aircraftID,
		AircraftRegistration: ac.Registration,
		Kind:                 in.Kind,
		Label:                in.Label,
		DueDate:              models.DateOnly(in.DueDate),
		IntervalMonths:       in.IntervalMonths,
		LastDoneOn:           dateOnlyPtr(in.LastDoneOn),
		Notes:                in.Notes,
	}
	if err := rem.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, rem); err != nil {
		return nil, fmt.Errorf("create aircraft reminder: %w", err)
	}
	return rem, nil
}

// Update applies a partial update and revalidates the result.
func (s *AircraftReminderService) Update(ctx context.Context, userID, aircraftID, reminderID uuid.UUID, p AircraftReminderPatch) (*models.AircraftReminder, error) {
	rem, err := s.ownedReminder(ctx, aircraftID, reminderID, userID)
	if err != nil {
		return nil, err
	}
	if p.Kind != nil {
		rem.Kind = *p.Kind
	}
	switch {
	case p.ClearLabel:
		rem.Label = nil
	case p.Label != nil:
		rem.Label = p.Label
	}
	if p.DueDate != nil {
		rem.DueDate = models.DateOnly(*p.DueDate)
	}
	switch {
	case p.ClearIntervalMonths:
		rem.IntervalMonths = nil
	case p.IntervalMonths != nil:
		rem.IntervalMonths = p.IntervalMonths
	}
	switch {
	case p.ClearLastDoneOn:
		rem.LastDoneOn = nil
	case p.LastDoneOn != nil:
		rem.LastDoneOn = dateOnlyPtr(p.LastDoneOn)
	}
	switch {
	case p.ClearNotes:
		rem.Notes = nil
	case p.Notes != nil:
		rem.Notes = p.Notes
	}
	if err := rem.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, rem); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrAircraftReminderNotFound
		}
		return nil, fmt.Errorf("update aircraft reminder: %w", err)
	}
	return rem, nil
}

// Complete records the item as done on doneOn (today when nil). With an
// interval the due date becomes doneOn plus intervalMonths, clamped to the
// month end; without one the due date is left unchanged.
func (s *AircraftReminderService) Complete(ctx context.Context, userID, aircraftID, reminderID uuid.UUID, doneOn *time.Time) (*models.AircraftReminder, error) {
	rem, err := s.ownedReminder(ctx, aircraftID, reminderID, userID)
	if err != nil {
		return nil, err
	}
	done := s.Today()
	if doneOn != nil {
		done = models.DateOnly(*doneOn)
	}
	rem.LastDoneOn = &done
	if rem.IntervalMonths != nil {
		rem.DueDate = models.AddMonthsClamped(done, *rem.IntervalMonths)
	}
	if err := s.repo.Update(ctx, rem); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrAircraftReminderNotFound
		}
		return nil, fmt.Errorf("complete aircraft reminder: %w", err)
	}
	return rem, nil
}

// Delete removes one of the user's reminders.
func (s *AircraftReminderService) Delete(ctx context.Context, userID, aircraftID, reminderID uuid.UUID) error {
	if _, err := s.ownedReminder(ctx, aircraftID, reminderID, userID); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, reminderID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrAircraftReminderNotFound
		}
		return fmt.Errorf("delete aircraft reminder: %w", err)
	}
	return nil
}

func dateOnlyPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	d := models.DateOnly(*t)
	return &d
}
