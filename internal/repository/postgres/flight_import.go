package postgres

import (
	"context"
	"database/sql"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type flightImportRepository struct {
	db *sql.DB
}

// NewFlightImportRepository creates the import-history repository.
func NewFlightImportRepository(db *sql.DB) repository.FlightImportRepository {
	return &flightImportRepository{db: db}
}

func (r *flightImportRepository) Create(ctx context.Context, rec *models.FlightImport) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO flight_imports (id, user_id, file_name, import_format, import_status,
			total_rows, imported_count, skipped_count, error_count, duplicate_count,
			imported_flight_ids, errors, column_mappings)
		VALUES ($1, $2, $3, $4::import_format, $5::import_status, $6, $7, $8, $9, $10, $11, $12, $13)
	`, rec.ID, rec.UserID, rec.FileName, rec.Format, rec.Status,
		rec.TotalRows, rec.ImportedCount, rec.SkippedCount, rec.ErrorCount, rec.DuplicateCount,
		rec.ImportedFlightIDs, rec.Errors, rec.ColumnMappings,
	)
	return err
}

func (r *flightImportRepository) CountByUserID(ctx context.Context, userID uuid.UUID) (int, error) {
	var total int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM flight_imports WHERE user_id = $1", userID,
	).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}

func (r *flightImportRepository) ListByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*models.FlightImport, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, file_name, import_format, import_status,
			total_rows, imported_count, skipped_count, error_count, duplicate_count,
			imported_flight_ids, errors, created_at
		FROM flight_imports WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*models.FlightImport
	for rows.Next() {
		rec, err := scanFlightImport(rows)
		if err != nil {
			continue
		}
		results = append(results, rec)
	}
	return results, rows.Err()
}

func (r *flightImportRepository) GetByIDForUser(ctx context.Context, id, userID uuid.UUID) (*models.FlightImport, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, file_name, import_format, import_status,
			total_rows, imported_count, skipped_count, error_count, duplicate_count,
			imported_flight_ids, errors, created_at
		FROM flight_imports WHERE id = $1 AND user_id = $2
	`, id, userID)

	rec := &models.FlightImport{}
	err := row.Scan(
		&rec.ID, &rec.UserID, &rec.FileName, &rec.Format, &rec.Status,
		&rec.TotalRows, &rec.ImportedCount, &rec.SkippedCount, &rec.ErrorCount, &rec.DuplicateCount,
		&rec.ImportedFlightIDs, &rec.Errors, &rec.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return rec, nil
}

func (r *flightImportRepository) DeleteByUserID(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM flight_imports WHERE user_id = $1", userID)
	return err
}

func scanFlightImport(rows *sql.Rows) (*models.FlightImport, error) {
	rec := &models.FlightImport{}
	err := rows.Scan(
		&rec.ID, &rec.UserID, &rec.FileName, &rec.Format, &rec.Status,
		&rec.TotalRows, &rec.ImportedCount, &rec.SkippedCount, &rec.ErrorCount, &rec.DuplicateCount,
		&rec.ImportedFlightIDs, &rec.Errors, &rec.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return rec, nil
}
