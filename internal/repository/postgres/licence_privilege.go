package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// LicencePrivilegeRepository implements repository.LicencePrivilegeRepository.
type LicencePrivilegeRepository struct {
	db *sql.DB
}

// NewLicencePrivilegeRepository returns a PostgreSQL licence privilege repository.
func NewLicencePrivilegeRepository(db *sql.DB) *LicencePrivilegeRepository {
	return &LicencePrivilegeRepository{db: db}
}

const licencePrivilegeSelect = `
	SELECT id, user_id, license_id, kind, detail, issued_on, expires_on, notes, created_at, updated_at
	FROM licence_privileges`

const licencePrivilegeOrder = ` ORDER BY kind, detail NULLS FIRST, id`

func scanLicencePrivilege(row interface{ Scan(...any) error }) (*models.LicencePrivilege, error) {
	p := &models.LicencePrivilege{}
	var kind string
	if err := row.Scan(&p.ID, &p.UserID, &p.LicenseID, &kind, &p.Detail, &p.IssuedOn, &p.ExpiresOn,
		&p.Notes, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	p.Kind = models.LicencePrivilegeKind(kind)
	return p, nil
}

// Create inserts the privilege, assigning id and timestamps.
func (r *LicencePrivilegeRepository) Create(ctx context.Context, p *models.LicencePrivilege) error {
	query := `
		INSERT INTO licence_privileges (user_id, license_id, kind, detail, issued_on, expires_on, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`
	return r.db.QueryRowContext(ctx, query,
		p.UserID, p.LicenseID, string(p.Kind), p.Detail, dateParam(p.IssuedOn), dateParam(p.ExpiresOn), p.Notes,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

// GetByID returns the privilege or repository.ErrNotFound.
func (r *LicencePrivilegeRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.LicencePrivilege, error) {
	p, err := scanLicencePrivilege(r.db.QueryRowContext(ctx, licencePrivilegeSelect+` WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	return p, err
}

// ListByLicense returns the licence's privileges.
func (r *LicencePrivilegeRepository) ListByLicense(ctx context.Context, licenseID uuid.UUID) ([]*models.LicencePrivilege, error) {
	return r.list(ctx, licencePrivilegeSelect+` WHERE license_id = $1`+licencePrivilegeOrder, licenseID)
}

// ListByUser returns the user's privileges across licences.
func (r *LicencePrivilegeRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]*models.LicencePrivilege, error) {
	return r.list(ctx, licencePrivilegeSelect+` WHERE user_id = $1`+licencePrivilegeOrder, userID)
}

func (r *LicencePrivilegeRepository) list(ctx context.Context, query string, args ...any) ([]*models.LicencePrivilege, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*models.LicencePrivilege{}
	for rows.Next() {
		p, err := scanLicencePrivilege(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Update writes the mutable fields and refreshes UpdatedAt.
func (r *LicencePrivilegeRepository) Update(ctx context.Context, p *models.LicencePrivilege) error {
	query := `
		UPDATE licence_privileges
		SET kind = $1, detail = $2, issued_on = $3, expires_on = $4, notes = $5
		WHERE id = $6
		RETURNING updated_at`
	err := r.db.QueryRowContext(ctx, query,
		string(p.Kind), p.Detail, dateParam(p.IssuedOn), dateParam(p.ExpiresOn), p.Notes, p.ID,
	).Scan(&p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrNotFound
	}
	return err
}

// Delete removes the privilege or returns repository.ErrNotFound.
func (r *LicencePrivilegeRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM licence_privileges WHERE id = $1`, id)
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

var _ repository.LicencePrivilegeRepository = (*LicencePrivilegeRepository)(nil)
