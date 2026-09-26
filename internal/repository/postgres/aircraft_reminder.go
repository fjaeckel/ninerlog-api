package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// AircraftReminderRepository implements repository.AircraftReminderRepository.
type AircraftReminderRepository struct {
	db *sql.DB
}

// NewAircraftReminderRepository returns a PostgreSQL aircraft reminder repository.
func NewAircraftReminderRepository(db *sql.DB) *AircraftReminderRepository {
	return &AircraftReminderRepository{db: db}
}

const aircraftReminderSelect = `
	SELECT r.id, r.user_id, r.aircraft_id, a.registration, r.kind, r.label, r.due_date,
	       r.interval_months, r.last_done_on, r.notes, r.created_at, r.updated_at
	FROM aircraft_reminders r
	JOIN aircraft a ON a.id = r.aircraft_id`

func scanAircraftReminder(row interface{ Scan(...any) error }) (*models.AircraftReminder, error) {
	r := &models.AircraftReminder{}
	var interval sql.NullInt64
	var kind string
	if err := row.Scan(&r.ID, &r.UserID, &r.AircraftID, &r.AircraftRegistration, &kind, &r.Label, &r.DueDate,
		&interval, &r.LastDoneOn, &r.Notes, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	r.Kind = models.AircraftReminderKind(kind)
	if interval.Valid {
		v := int(interval.Int64)
		r.IntervalMonths = &v
	}
	return r, nil
}

func dateParam(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format("2006-01-02")
}

// Create inserts the reminder, assigning id and timestamps.
func (r *AircraftReminderRepository) Create(ctx context.Context, rem *models.AircraftReminder) error {
	query := `
		INSERT INTO aircraft_reminders (user_id, aircraft_id, kind, label, due_date, interval_months, last_done_on, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at`
	due := rem.DueDate
	return r.db.QueryRowContext(ctx, query,
		rem.UserID, rem.AircraftID, string(rem.Kind), rem.Label, dateParam(&due),
		rem.IntervalMonths, dateParam(rem.LastDoneOn), rem.Notes,
	).Scan(&rem.ID, &rem.CreatedAt, &rem.UpdatedAt)
}

// GetByID returns the reminder or repository.ErrNotFound.
func (r *AircraftReminderRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.AircraftReminder, error) {
	rem, err := scanAircraftReminder(r.db.QueryRowContext(ctx, aircraftReminderSelect+` WHERE r.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	return rem, err
}

// ListByAircraft returns the aircraft's reminders by due date.
func (r *AircraftReminderRepository) ListByAircraft(ctx context.Context, aircraftID uuid.UUID) ([]*models.AircraftReminder, error) {
	return r.list(ctx, aircraftReminderSelect+` WHERE r.aircraft_id = $1 ORDER BY r.due_date, r.id`, aircraftID)
}

// ListByUser returns the user's reminders by due date, optionally bounded.
func (r *AircraftReminderRepository) ListByUser(ctx context.Context, userID uuid.UUID, dueOnOrBefore *time.Time) ([]*models.AircraftReminder, error) {
	return r.list(ctx, aircraftReminderSelect+`
		WHERE r.user_id = $1 AND ($2::DATE IS NULL OR r.due_date <= $2::DATE)
		ORDER BY r.due_date, a.registration, r.id`, userID, dateParam(dueOnOrBefore))
}

func (r *AircraftReminderRepository) list(ctx context.Context, query string, args ...any) ([]*models.AircraftReminder, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*models.AircraftReminder{}
	for rows.Next() {
		rem, err := scanAircraftReminder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rem)
	}
	return out, rows.Err()
}

// Update writes the mutable fields and refreshes UpdatedAt.
func (r *AircraftReminderRepository) Update(ctx context.Context, rem *models.AircraftReminder) error {
	query := `
		UPDATE aircraft_reminders
		SET kind = $1, label = $2, due_date = $3, interval_months = $4, last_done_on = $5, notes = $6
		WHERE id = $7
		RETURNING updated_at`
	due := rem.DueDate
	err := r.db.QueryRowContext(ctx, query,
		string(rem.Kind), rem.Label, dateParam(&due), rem.IntervalMonths, dateParam(rem.LastDoneOn), rem.Notes, rem.ID,
	).Scan(&rem.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrNotFound
	}
	return err
}

// Delete removes the reminder or returns repository.ErrNotFound.
func (r *AircraftReminderRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM aircraft_reminders WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

var _ repository.AircraftReminderRepository = (*AircraftReminderRepository)(nil)
