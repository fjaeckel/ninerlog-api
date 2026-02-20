package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type notificationRepository struct {
	db *sql.DB
}

func NewNotificationRepository(db *sql.DB) repository.NotificationRepository {
	return &notificationRepository{db: db}
}

func (r *notificationRepository) GetPreferences(ctx context.Context, userID uuid.UUID) (*models.NotificationPreferences, error) {
	query := `
		SELECT id, user_id, email_enabled, currency_warnings, credential_warnings, warning_days, created_at, updated_at
		FROM notification_preferences WHERE user_id = $1
	`
	p := &models.NotificationPreferences{}
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&p.ID, &p.UserID, &p.EmailEnabled, &p.CurrencyWarnings, &p.CredentialWarnings,
		&p.WarningDays, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		// Return defaults if no record exists
		return &models.NotificationPreferences{
			UserID:             userID,
			EmailEnabled:       true,
			CurrencyWarnings:   true,
			CredentialWarnings: true,
			WarningDays:        pq.Int64Array{30, 14, 7},
		}, nil
	}
	return p, err
}

func (r *notificationRepository) UpsertPreferences(ctx context.Context, prefs *models.NotificationPreferences) error {
	query := `
		INSERT INTO notification_preferences (user_id, email_enabled, currency_warnings, credential_warnings, warning_days)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id) DO UPDATE SET
			email_enabled = EXCLUDED.email_enabled,
			currency_warnings = EXCLUDED.currency_warnings,
			credential_warnings = EXCLUDED.credential_warnings,
			warning_days = EXCLUDED.warning_days,
			updated_at = NOW()
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, query,
		prefs.UserID, prefs.EmailEnabled, prefs.CurrencyWarnings,
		prefs.CredentialWarnings, prefs.WarningDays,
	).Scan(&prefs.ID, &prefs.CreatedAt, &prefs.UpdatedAt)
}

func (r *notificationRepository) LogNotification(ctx context.Context, log *models.NotificationLog) error {
	query := `
		INSERT INTO notification_log (user_id, notification_type, reference_id, reference_type, days_before_expiry)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, sent_at
	`
	return r.db.QueryRowContext(ctx, query,
		log.UserID, log.NotificationType, log.ReferenceID, log.ReferenceType, log.DaysBeforeExpiry,
	).Scan(&log.ID, &log.SentAt)
}

func (r *notificationRepository) HasBeenSent(ctx context.Context, userID uuid.UUID, notificationType string, referenceID uuid.UUID, daysBeforeExpiry int) (bool, error) {
	query := `
		SELECT COUNT(*) > 0 FROM notification_log
		WHERE user_id = $1 AND notification_type = $2 AND reference_id = $3 AND days_before_expiry = $4
	`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, userID, notificationType, referenceID, daysBeforeExpiry).Scan(&exists)
	return exists, err
}

func (r *notificationRepository) GetAllUsersWithPreferences(ctx context.Context) ([]*models.NotificationPreferences, error) {
	query := `
		SELECT np.id, np.user_id, np.email_enabled, np.currency_warnings, np.credential_warnings,
		       np.warning_days, np.created_at, np.updated_at
		FROM notification_preferences np
		JOIN users u ON np.user_id = u.id
		WHERE np.email_enabled = true
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prefs []*models.NotificationPreferences
	for rows.Next() {
		p := &models.NotificationPreferences{}
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.EmailEnabled, &p.CurrencyWarnings, &p.CredentialWarnings,
			&p.WarningDays, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		prefs = append(prefs, p)
	}

	// Also include users without preferences (defaults)
	defaultQuery := `
		SELECT u.id FROM users u
		LEFT JOIN notification_preferences np ON u.id = np.user_id
		WHERE np.id IS NULL
	`
	defaultRows, err := r.db.QueryContext(ctx, defaultQuery)
	if err != nil {
		return prefs, nil // non-fatal
	}
	defer defaultRows.Close()

	for defaultRows.Next() {
		var userID uuid.UUID
		if err := defaultRows.Scan(&userID); err != nil {
			continue
		}
		prefs = append(prefs, &models.NotificationPreferences{
			UserID:             userID,
			EmailEnabled:       true,
			CurrencyWarnings:   true,
			CredentialWarnings: true,
			WarningDays:        pq.Int64Array{30, 14, 7},
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		})
	}

	return prefs, nil
}
