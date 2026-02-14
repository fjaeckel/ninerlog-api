package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/repository"
	"github.com/google/uuid"
)

type ClassRatingRepository struct {
	db *sql.DB
}

func NewClassRatingRepository(db *sql.DB) *ClassRatingRepository {
	return &ClassRatingRepository{db: db}
}

func (r *ClassRatingRepository) Create(ctx context.Context, cr *models.ClassRating) error {
	query := `
		INSERT INTO class_ratings (id, license_id, class_type, issue_date, expiry_date, notes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	cr.ID = uuid.New()
	now := time.Now()
	cr.CreatedAt = now
	cr.UpdatedAt = now
	_, err := r.db.ExecContext(ctx, query,
		cr.ID, cr.LicenseID, cr.ClassType, cr.IssueDate, cr.ExpiryDate, cr.Notes, cr.CreatedAt, cr.UpdatedAt,
	)
	return err
}

func (r *ClassRatingRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.ClassRating, error) {
	query := `SELECT id, license_id, class_type, issue_date, expiry_date, notes, created_at, updated_at FROM class_ratings WHERE id = $1`
	cr := &models.ClassRating{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&cr.ID, &cr.LicenseID, &cr.ClassType, &cr.IssueDate, &cr.ExpiryDate, &cr.Notes, &cr.CreatedAt, &cr.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return cr, nil
}

func (r *ClassRatingRepository) GetByLicenseID(ctx context.Context, licenseID uuid.UUID) ([]*models.ClassRating, error) {
	query := `SELECT id, license_id, class_type, issue_date, expiry_date, notes, created_at, updated_at FROM class_ratings WHERE license_id = $1 ORDER BY class_type`
	rows, err := r.db.QueryContext(ctx, query, licenseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ratings []*models.ClassRating
	for rows.Next() {
		cr := &models.ClassRating{}
		if err := rows.Scan(&cr.ID, &cr.LicenseID, &cr.ClassType, &cr.IssueDate, &cr.ExpiryDate, &cr.Notes, &cr.CreatedAt, &cr.UpdatedAt); err != nil {
			return nil, err
		}
		ratings = append(ratings, cr)
	}
	return ratings, rows.Err()
}

func (r *ClassRatingRepository) Update(ctx context.Context, cr *models.ClassRating) error {
	query := `UPDATE class_ratings SET issue_date = $1, expiry_date = $2, notes = $3, updated_at = $4 WHERE id = $5`
	cr.UpdatedAt = time.Now()
	result, err := r.db.ExecContext(ctx, query, cr.IssueDate, cr.ExpiryDate, cr.Notes, cr.UpdatedAt, cr.ID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *ClassRatingRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM class_ratings WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return repository.ErrNotFound
	}
	return nil
}
