package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type flightFileRepository struct {
	db *sql.DB
}

// NewFlightFileRepository returns the PostgreSQL FlightFileRepository.
func NewFlightFileRepository(db *sql.DB) repository.FlightFileRepository {
	return &flightFileRepository{db: db}
}

// flightFileColumns is the metadata projection, without content.
const flightFileColumns = `id, user_id, flight_id, kind, filename, size_bytes, sha256, created_at`

func scanFlightFile(scan func(dest ...any) error, withContent bool) (*models.FlightFile, error) {
	f := &models.FlightFile{}
	dest := []any{&f.ID, &f.UserID, &f.FlightID, &f.Kind, &f.Filename, &f.SizeBytes, &f.SHA256, &f.CreatedAt}
	if withContent {
		dest = append(dest, &f.Content)
	}
	if err := scan(dest...); err != nil {
		return nil, err
	}
	return f, nil
}

func (r *flightFileRepository) Create(ctx context.Context, file *models.FlightFile, maxPerFlight int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var lockedID uuid.UUID
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM flights WHERE id = $1 AND user_id = $2 FOR UPDATE`,
		file.FlightID, file.UserID).Scan(&lockedID)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrNotFound
	}
	if err != nil {
		return err
	}

	var count int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM flight_files WHERE flight_id = $1`, file.FlightID).Scan(&count); err != nil {
		return err
	}
	if count >= maxPerFlight {
		return repository.ErrFlightFileLimit
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO flight_files (user_id, flight_id, kind, filename, content, size_bytes, sha256)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`,
		file.UserID, file.FlightID, file.Kind, file.Filename, file.Content, file.SizeBytes, file.SHA256,
	).Scan(&file.ID, &file.CreatedAt)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return repository.ErrDuplicate
		}
		return err
	}
	return tx.Commit()
}

func (r *flightFileRepository) list(ctx context.Context, withContent bool, query string, args ...any) ([]*models.FlightFile, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := make([]*models.FlightFile, 0)
	for rows.Next() {
		f, err := scanFlightFile(rows.Scan, withContent)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

func (r *flightFileRepository) ListByFlight(ctx context.Context, userID, flightID uuid.UUID) ([]*models.FlightFile, error) {
	return r.list(ctx, false, `SELECT `+flightFileColumns+` FROM flight_files
		WHERE user_id = $1 AND flight_id = $2
		ORDER BY created_at ASC, id ASC`, userID, flightID)
}

func (r *flightFileRepository) GetWithContent(ctx context.Context, userID, flightID, fileID uuid.UUID) (*models.FlightFile, error) {
	f, err := scanFlightFile(r.db.QueryRowContext(ctx, `SELECT `+flightFileColumns+`, content FROM flight_files
		WHERE id = $1 AND user_id = $2 AND flight_id = $3`, fileID, userID, flightID).Scan, true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	return f, err
}

func (r *flightFileRepository) Delete(ctx context.Context, userID, flightID, fileID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM flight_files WHERE id = $1 AND user_id = $2 AND flight_id = $3`, fileID, userID, flightID)
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

func (r *flightFileRepository) ListBySHA256(ctx context.Context, userID uuid.UUID, sha256 string) ([]*models.FlightFile, error) {
	return r.list(ctx, false, `SELECT `+flightFileColumns+` FROM flight_files
		WHERE user_id = $1 AND sha256 = $2
		ORDER BY created_at ASC, id ASC`, userID, sha256)
}

func (r *flightFileRepository) ListByUserWithContent(ctx context.Context, userID uuid.UUID) ([]*models.FlightFile, error) {
	return r.list(ctx, true, `SELECT `+flightFileColumns+`, content FROM flight_files
		WHERE user_id = $1
		ORDER BY flight_id ASC, created_at ASC, id ASC`, userID)
}
