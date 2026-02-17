package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/repository"
	"github.com/google/uuid"
)

type ContactRepository struct {
	db *sql.DB
}

func NewContactRepository(db *sql.DB) *ContactRepository {
	return &ContactRepository{db: db}
}

func (r *ContactRepository) Create(ctx context.Context, contact *models.Contact) error {
	query := `
		INSERT INTO contacts (id, user_id, name, email, phone, notes, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	contact.ID = uuid.New()
	now := time.Now()
	contact.CreatedAt = now
	contact.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, query,
		contact.ID, contact.UserID, contact.Name, contact.Email, contact.Phone, contact.Notes,
		contact.CreatedAt, contact.UpdatedAt,
	)
	return err
}

func (r *ContactRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Contact, error) {
	query := `SELECT id, user_id, name, email, phone, notes, created_at, updated_at FROM contacts WHERE id = $1`
	c := &models.Contact{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&c.ID, &c.UserID, &c.Name, &c.Email, &c.Phone, &c.Notes, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *ContactRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.Contact, error) {
	query := `SELECT id, user_id, name, email, phone, notes, created_at, updated_at FROM contacts WHERE user_id = $1 ORDER BY name`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []*models.Contact
	for rows.Next() {
		c := &models.Contact{}
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Email, &c.Phone, &c.Notes, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, rows.Err()
}

func (r *ContactRepository) GetByExactName(ctx context.Context, userID uuid.UUID, name string) (*models.Contact, error) {
	query := `SELECT id, user_id, name, email, phone, notes, created_at, updated_at FROM contacts WHERE user_id = $1 AND LOWER(name) = LOWER($2) LIMIT 1`
	c := &models.Contact{}
	err := r.db.QueryRowContext(ctx, query, userID, name).Scan(
		&c.ID, &c.UserID, &c.Name, &c.Email, &c.Phone, &c.Notes, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *ContactRepository) Search(ctx context.Context, userID uuid.UUID, query string, limit int) ([]*models.Contact, error) {
	sqlQuery := `
		SELECT id, user_id, name, email, phone, notes, created_at, updated_at
		FROM contacts
		WHERE user_id = $1 AND LOWER(name) LIKE LOWER($2)
		ORDER BY name
		LIMIT $3
	`
	pattern := fmt.Sprintf("%%%s%%", query)
	rows, err := r.db.QueryContext(ctx, sqlQuery, userID, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []*models.Contact
	for rows.Next() {
		c := &models.Contact{}
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Email, &c.Phone, &c.Notes, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, rows.Err()
}

func (r *ContactRepository) Update(ctx context.Context, contact *models.Contact) error {
	query := `
		UPDATE contacts SET name = $1, email = $2, phone = $3, notes = $4, updated_at = $5
		WHERE id = $6
	`
	contact.UpdatedAt = time.Now()
	result, err := r.db.ExecContext(ctx, query,
		contact.Name, contact.Email, contact.Phone, contact.Notes, contact.UpdatedAt, contact.ID,
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

func (r *ContactRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM contacts WHERE id = $1`
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
