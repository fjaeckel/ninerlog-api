package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type logbookRepository struct {
	db *sql.DB
	tx *sql.Tx
}

// NewLogbookRepository creates the strict logbook mode repository.
func NewLogbookRepository(db *sql.DB) repository.LogbookRepository {
	return &logbookRepository{db: db}
}

func (r *logbookRepository) GetMode(ctx context.Context, userID uuid.UUID) (*models.LogbookMode, error) {
	mode := &models.LogbookMode{}
	err := r.db.QueryRowContext(ctx,
		`SELECT strict_logbook, strict_logbook_since FROM users WHERE id = $1`, userID,
	).Scan(&mode.Strict, &mode.Since)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return mode, nil
}

// writer returns the transaction the repository is bound to, else the pool.
func (r *logbookRepository) writer() queryExecer {
	if r.tx != nil {
		return r.tx
	}
	return r.db
}

func (r *logbookRepository) SetStrict(ctx context.Context, userID uuid.UUID, strict bool, trigger string, licenseID *uuid.UUID) (bool, error) {
	if r.tx != nil {
		return setStrict(ctx, r.tx, userID, strict, trigger, licenseID)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	switched, err := setStrict(ctx, tx, userID, strict, trigger, licenseID)
	if err != nil || !switched {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// setStrict switches strict mode and stores the event in tx.
func setStrict(ctx context.Context, tx *sql.Tx, userID uuid.UUID, strict bool, trigger string, licenseID *uuid.UUID) (bool, error) {
	var current bool
	err := tx.QueryRowContext(ctx,
		`SELECT strict_logbook FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return false, repository.ErrNotFound
	}
	if err != nil {
		return false, err
	}
	if current == strict {
		return false, nil
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE users
		    SET strict_logbook = $2,
		        strict_logbook_since = CASE WHEN $2 THEN now() ELSE strict_logbook_since END
		  WHERE id = $1`, userID, strict); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO logbook_mode_events (user_id, strict, trigger, license_id) VALUES ($1, $2, $3, $4)`,
		userID, strict, trigger, licenseID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *logbookRepository) RestoreStrictSince(ctx context.Context, userID uuid.UUID, since time.Time) error {
	_, err := r.writer().ExecContext(ctx,
		`UPDATE users SET strict_logbook_since = $2 WHERE id = $1 AND strict_logbook`, userID, since)
	return err
}

func (r *logbookRepository) ListModeEvents(ctx context.Context, userID uuid.UUID) ([]models.LogbookModeEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, strict, trigger, license_id, created_at
		   FROM logbook_mode_events WHERE user_id = $1 ORDER BY created_at, id`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	events := []models.LogbookModeEvent{}
	for rows.Next() {
		var e models.LogbookModeEvent
		if err := rows.Scan(&e.ID, &e.UserID, &e.Strict, &e.Trigger, &e.LicenseID, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (r *logbookRepository) ImportModeEvent(ctx context.Context, e *models.LogbookModeEvent) (bool, error) {
	res, err := r.writer().ExecContext(ctx,
		`INSERT INTO logbook_mode_events (user_id, strict, trigger, license_id, created_at)
		 SELECT $1, $2, $3, $5, $4
		  WHERE NOT EXISTS (
		        SELECT 1 FROM logbook_mode_events
		         WHERE user_id = $1 AND strict = $2 AND trigger = $3 AND created_at = $4)`,
		e.UserID, e.Strict, e.Trigger, e.CreatedAt, e.LicenseID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *logbookRepository) CreateCertification(ctx context.Context, c *models.LogbookCertification) error {
	return r.db.QueryRowContext(ctx,
		`INSERT INTO logbook_certifications (user_id, certified_through) VALUES ($1, $2)
		 RETURNING id, certified_at`, c.UserID, c.CertifiedThrough,
	).Scan(&c.ID, &c.CertifiedAt)
}

func (r *logbookRepository) ImportCertification(ctx context.Context, c *models.LogbookCertification) (bool, error) {
	res, err := r.writer().ExecContext(ctx,
		`INSERT INTO logbook_certifications (user_id, certified_through, certified_at)
		 SELECT $1, $2, $3
		  WHERE NOT EXISTS (
		        SELECT 1 FROM logbook_certifications
		         WHERE user_id = $1 AND certified_through = $2 AND certified_at = $3)`,
		c.UserID, c.CertifiedThrough, c.CertifiedAt)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *logbookRepository) ListCertifications(ctx context.Context, userID uuid.UUID) ([]models.LogbookCertification, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, certified_through, certified_at
		   FROM logbook_certifications WHERE user_id = $1
		  ORDER BY certified_through DESC, certified_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []models.LogbookCertification{}
	for rows.Next() {
		var c models.LogbookCertification
		if err := rows.Scan(&c.ID, &c.UserID, &c.CertifiedThrough, &c.CertifiedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *logbookRepository) LatestCertifiedThrough(ctx context.Context, userID uuid.UUID) (*time.Time, error) {
	var latest sql.NullTime
	if err := r.db.QueryRowContext(ctx,
		`SELECT MAX(certified_through) FROM logbook_certifications WHERE user_id = $1`, userID,
	).Scan(&latest); err != nil {
		return nil, err
	}
	if !latest.Valid {
		return nil, nil
	}
	return &latest.Time, nil
}

const flightRevisionColumns = `id, user_id, flight_id, revision_no, kind, snapshot, crew_snapshot,
	source, reason, is_correction, changed_at`

func scanFlightRevisions(rows *sql.Rows) ([]models.FlightRevision, error) {
	defer func() { _ = rows.Close() }()
	out := []models.FlightRevision{}
	for rows.Next() {
		var rev models.FlightRevision
		if err := rows.Scan(&rev.ID, &rev.UserID, &rev.FlightID, &rev.RevisionNo, &rev.Kind,
			&rev.Snapshot, &rev.CrewSnapshot, &rev.Source, &rev.Reason, &rev.IsCorrection, &rev.ChangedAt); err != nil {
			return nil, err
		}
		out = append(out, rev)
	}
	return out, rows.Err()
}

func (r *logbookRepository) CurrentFlightState(ctx context.Context, userID, flightID uuid.UUID) (json.RawMessage, json.RawMessage, error) {
	var snapshot, crew []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT to_jsonb(f), flight_revision_crew(f.id) FROM flights f WHERE f.id = $1 AND f.user_id = $2`,
		flightID, userID,
	).Scan(&snapshot, &crew)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	return snapshot, crew, nil
}

func (r *logbookRepository) CurrentFlightStates(ctx context.Context, userID uuid.UUID, flightIDs []uuid.UUID) (map[uuid.UUID]models.FlightState, error) {
	out := make(map[uuid.UUID]models.FlightState, len(flightIDs))
	if len(flightIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT f.id, to_jsonb(f), flight_revision_crew(f.id) FROM flights f
		  WHERE f.user_id = $1 AND f.id = ANY($2)`, userID, pq.Array(flightIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id uuid.UUID
		var snapshot, crew []byte
		if err := rows.Scan(&id, &snapshot, &crew); err != nil {
			return nil, err
		}
		out[id] = models.FlightState{Snapshot: snapshot, Crew: crew}
	}
	return out, rows.Err()
}

func (r *logbookRepository) ListRevisions(ctx context.Context, userID, flightID uuid.UUID) ([]models.FlightRevision, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+flightRevisionColumns+` FROM flight_revisions
		  WHERE user_id = $1 AND flight_id = $2 ORDER BY revision_no`, userID, flightID)
	if err != nil {
		return nil, err
	}
	return scanFlightRevisions(rows)
}

func (r *logbookRepository) ListUserRevisions(ctx context.Context, userID uuid.UUID) ([]models.FlightRevision, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+flightRevisionColumns+` FROM flight_revisions
		  WHERE user_id = $1 ORDER BY flight_id, revision_no`, userID)
	if err != nil {
		return nil, err
	}
	return scanFlightRevisions(rows)
}

func (r *logbookRepository) ListVoided(ctx context.Context, userID uuid.UUID) ([]models.FlightRevision, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+flightRevisionColumns+` FROM flight_revisions
		  WHERE user_id = $1 AND kind = 'delete' ORDER BY changed_at DESC, id`, userID)
	if err != nil {
		return nil, err
	}
	return scanFlightRevisions(rows)
}

func (r *logbookRepository) CountVoided(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM flight_revisions WHERE user_id = $1 AND kind = 'delete'`, userID).Scan(&n)
	return n, err
}

func (r *logbookRepository) ImportRevisions(ctx context.Context, revs []models.FlightRevision) (int, error) {
	const perRow = 10
	stored := 0
	for start := 0; start < len(revs); start += revisionInsertChunk {
		chunk := revs[start:min(start+revisionInsertChunk, len(revs))]
		args := make([]any, 0, len(chunk)*perRow)
		for _, rev := range chunk {
			args = append(args, rev.UserID, rev.FlightID, rev.RevisionNo, rev.Kind, []byte(rev.Snapshot),
				[]byte(rev.CrewSnapshot), rev.Source, rev.Reason, rev.IsCorrection, rev.ChangedAt)
		}
		res, err := r.writer().ExecContext(ctx,
			`INSERT INTO flight_revisions
			     (user_id, flight_id, revision_no, kind, snapshot, crew_snapshot, source, reason, is_correction, changed_at)
			 VALUES `+valueRows(len(chunk), perRow)+`
			 ON CONFLICT (flight_id, revision_no) DO NOTHING`, args...)
		if err != nil {
			return stored, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return stored, err
		}
		stored += int(n)
	}
	return stored, nil
}
