package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/repository"
	"github.com/google/uuid"
)

type licenseRepository struct {
	db *sql.DB
}

// NewLicenseRepository creates a new license repository
func NewLicenseRepository(db *sql.DB) repository.LicenseRepository {
	return &licenseRepository{db: db}
}

func (r *licenseRepository) Create(ctx context.Context, license *models.License) error {
	query := `
		INSERT INTO licenses (
			user_id, regulatory_authority, license_type, license_number,
			issue_date, issuing_authority, requires_separate_logbook
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`

	return r.db.QueryRowContext(
		ctx, query,
		license.UserID,
		license.RegulatoryAuthority,
		license.LicenseType,
		license.LicenseNumber,
		license.IssueDate,
		license.IssuingAuthority,
		license.RequiresSeparateLogbook,
	).Scan(&license.ID, &license.CreatedAt, &license.UpdatedAt)
}

func (r *licenseRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.License, error) {
	query := `
		SELECT id, user_id, regulatory_authority, license_type, license_number,
		       issue_date, issuing_authority, requires_separate_logbook, created_at, updated_at
		FROM licenses
		WHERE id = $1
	`

	license := &models.License{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&license.ID,
		&license.UserID,
		&license.RegulatoryAuthority,
		&license.LicenseType,
		&license.LicenseNumber,
		&license.IssueDate,
		&license.IssuingAuthority,
		&license.RequiresSeparateLogbook,
		&license.CreatedAt,
		&license.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return license, nil
}

func (r *licenseRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.License, error) {
	query := `
		SELECT id, user_id, regulatory_authority, license_type, license_number,
		       issue_date, issuing_authority, requires_separate_logbook, created_at, updated_at
		FROM licenses
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanLicenses(rows)
}

func (r *licenseRepository) Update(ctx context.Context, license *models.License) error {
	query := `
		UPDATE licenses
		SET regulatory_authority = $1, license_type = $2, license_number = $3,
		    issuing_authority = $4, requires_separate_logbook = $5, updated_at = $6
		WHERE id = $7
	`

	result, err := r.db.ExecContext(
		ctx, query,
		license.RegulatoryAuthority,
		license.LicenseType,
		license.LicenseNumber,
		license.IssuingAuthority,
		license.RequiresSeparateLogbook,
		time.Now(),
		license.ID,
	)
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

func (r *licenseRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM licenses WHERE id = $1`

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

func (r *licenseRepository) scanLicenses(rows *sql.Rows) ([]*models.License, error) {
	var licenses []*models.License

	for rows.Next() {
		license := &models.License{}
		err := rows.Scan(
			&license.ID,
			&license.UserID,
			&license.RegulatoryAuthority,
			&license.LicenseType,
			&license.LicenseNumber,
			&license.IssueDate,
			&license.IssuingAuthority,
			&license.RequiresSeparateLogbook,
			&license.CreatedAt,
			&license.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		licenses = append(licenses, license)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return licenses, nil
}
