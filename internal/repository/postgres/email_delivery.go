package postgres

import (
	"context"
	"database/sql"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type emailDeliveryRepository struct {
	db *sql.DB
}

// NewEmailDeliveryRepository creates a new email delivery repository.
func NewEmailDeliveryRepository(db *sql.DB) repository.EmailDeliveryRepository {
	return &emailDeliveryRepository{db: db}
}

// normalizeEmail lower-cases and trims an address so the suppression list keys
// on the same form the users table stores.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (r *emailDeliveryRepository) RecordEvent(ctx context.Context, event *models.EmailDeliveryEvent) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	const query = `
		INSERT INTO email_delivery_events (id, user_id, recipient, email_type, status, smtp_code, detail)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at
	`
	return r.db.QueryRowContext(ctx, query,
		event.ID,
		event.UserID,
		normalizeEmail(event.Recipient),
		event.EmailType,
		event.Status,
		event.SMTPCode,
		event.Detail,
	).Scan(&event.CreatedAt)
}

func (r *emailDeliveryRepository) ListEvents(ctx context.Context, recipient string, limit int) ([]*models.EmailDeliveryEvent, error) {
	// An empty recipient means "no filter". Expressed in SQL rather than by
	// building two query strings: $1 IS NULL short-circuits the comparison.
	const query = `
		SELECT id, user_id, recipient, email_type, status, smtp_code, detail, created_at
		FROM email_delivery_events
		WHERE $1::text IS NULL OR recipient = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	var filter *string
	if recipient != "" {
		normalized := normalizeEmail(recipient)
		filter = &normalized
	}

	rows, err := r.db.QueryContext(ctx, query, filter, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	events := []*models.EmailDeliveryEvent{}
	for rows.Next() {
		e := &models.EmailDeliveryEvent{}
		if err := rows.Scan(&e.ID, &e.UserID, &e.Recipient, &e.EmailType, &e.Status,
			&e.SMTPCode, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (r *emailDeliveryRepository) IsSuppressed(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM email_suppressions WHERE email = $1)`,
		normalizeEmail(email),
	).Scan(&exists)
	return exists, err
}

func (r *emailDeliveryRepository) Suppress(ctx context.Context, email, reason string, smtpCode *int, detail string) error {
	// A repeat bounce refreshes the reason and bumps the count but keeps
	// first_bounced_at, so an operator can see how long the address has been
	// dead.
	const query = `
		INSERT INTO email_suppressions (email, reason, smtp_code, detail)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email) DO UPDATE SET
			reason = EXCLUDED.reason,
			smtp_code = EXCLUDED.smtp_code,
			detail = EXCLUDED.detail,
			last_bounced_at = NOW(),
			bounce_count = email_suppressions.bounce_count + 1
	`
	_, err := r.db.ExecContext(ctx, query, normalizeEmail(email), reason, smtpCode, detail)
	return err
}

func (r *emailDeliveryRepository) Unsuppress(ctx context.Context, email string) error {
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM email_suppressions WHERE email = $1`, normalizeEmail(email))
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

func (r *emailDeliveryRepository) ListSuppressions(ctx context.Context, limit int) ([]*models.EmailSuppression, error) {
	const query = `
		SELECT email, reason, smtp_code, detail, first_bounced_at, last_bounced_at, bounce_count
		FROM email_suppressions
		ORDER BY last_bounced_at DESC
		LIMIT $1
	`
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	suppressions := []*models.EmailSuppression{}
	for rows.Next() {
		s := &models.EmailSuppression{}
		if err := rows.Scan(&s.Email, &s.Reason, &s.SMTPCode, &s.Detail,
			&s.FirstBouncedAt, &s.LastBouncedAt, &s.BounceCount); err != nil {
			return nil, err
		}
		suppressions = append(suppressions, s)
	}
	return suppressions, rows.Err()
}

func (r *emailDeliveryRepository) CountSuppressions(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM email_suppressions`).Scan(&count)
	return count, err
}

func (r *emailDeliveryRepository) UserIDForEmail(ctx context.Context, email string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM users WHERE email = $1`, normalizeEmail(email)).Scan(&id)
	if err == sql.ErrNoRows {
		return uuid.Nil, repository.ErrNotFound
	}
	return id, err
}
