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
		SELECT id, user_id, email_enabled, enabled_categories, warning_days, check_hour, created_at, updated_at
		FROM notification_preferences WHERE user_id = $1
	`
	p := &models.NotificationPreferences{}
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&p.ID, &p.UserID, &p.EmailEnabled, &p.EnabledCategories,
		&p.WarningDays, &p.CheckHour, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		// Return defaults if no record exists
		return &models.NotificationPreferences{
			UserID:            userID,
			EmailEnabled:      true,
			EnabledCategories: models.AllNotificationCategories,
			WarningDays:       pq.Int64Array{30, 14, 7},
			CheckHour:         8,
		}, nil
	}
	return p, err
}

func (r *notificationRepository) UpsertPreferences(ctx context.Context, prefs *models.NotificationPreferences) error {
	query := `
		INSERT INTO notification_preferences (user_id, email_enabled, enabled_categories, warning_days, check_hour)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id) DO UPDATE SET
			email_enabled = EXCLUDED.email_enabled,
			enabled_categories = EXCLUDED.enabled_categories,
			warning_days = EXCLUDED.warning_days,
			check_hour = EXCLUDED.check_hour,
			updated_at = NOW()
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, query,
		prefs.UserID, prefs.EmailEnabled, prefs.EnabledCategories,
		prefs.WarningDays, prefs.CheckHour,
	).Scan(&prefs.ID, &prefs.CreatedAt, &prefs.UpdatedAt)
}

func (r *notificationRepository) LogNotification(ctx context.Context, log *models.NotificationLog) error {
	query := `
		INSERT INTO notification_log (user_id, notification_type, reference_id, reference_type, days_before_expiry, expiry_reference_date, subject)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, notification_type, reference_id, days_before_expiry, expiry_reference_date) DO NOTHING
		RETURNING id, sent_at
	`
	err := r.db.QueryRowContext(ctx, query,
		log.UserID, log.NotificationType, log.ReferenceID, log.ReferenceType,
		log.DaysBeforeExpiry, log.ExpiryReferenceDate, log.Subject,
	).Scan(&log.ID, &log.SentAt)
	// If ON CONFLICT DO NOTHING fires, no row is returned — treat as already sent
	if err == sql.ErrNoRows {
		return nil
	}
	return err
}

func (r *notificationRepository) HasBeenSent(ctx context.Context, userID uuid.UUID, notificationType string, referenceID uuid.UUID, daysBeforeExpiry int, expiryReferenceDate *time.Time) (bool, error) {
	query := `
		SELECT COUNT(*) > 0 FROM notification_log
		WHERE user_id = $1 AND notification_type = $2 AND reference_id = $3
		  AND days_before_expiry = $4
		  AND (($5::DATE IS NULL AND expiry_reference_date IS NULL) OR expiry_reference_date = $5)
	`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, userID, notificationType, referenceID, daysBeforeExpiry, expiryReferenceDate).Scan(&exists)
	return exists, err
}

func (r *notificationRepository) GetAllUsersWithPreferences(ctx context.Context) ([]*models.NotificationPreferences, error) {
	query := `
		SELECT np.id, np.user_id, np.email_enabled, np.enabled_categories,
		       np.warning_days, np.check_hour, np.created_at, np.updated_at
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
			&p.ID, &p.UserID, &p.EmailEnabled, &p.EnabledCategories,
			&p.WarningDays, &p.CheckHour, &p.CreatedAt, &p.UpdatedAt,
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
			UserID:            userID,
			EmailEnabled:      true,
			EnabledCategories: models.AllNotificationCategories,
			WarningDays:       pq.Int64Array{30, 14, 7},
			CheckHour:         8,
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		})
	}

	return prefs, nil
}

func (r *notificationRepository) GetNotificationHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*models.NotificationLog, int, error) {
	// Get total count
	countQuery := `SELECT COUNT(*) FROM notification_log WHERE user_id = $1`
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, userID).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get paginated results
	query := `
		SELECT id, user_id, notification_type, reference_id, reference_type,
		       days_before_expiry, expiry_reference_date, subject, sent_at
		FROM notification_log
		WHERE user_id = $1
		ORDER BY sent_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []*models.NotificationLog
	for rows.Next() {
		l := &models.NotificationLog{}
		if err := rows.Scan(
			&l.ID, &l.UserID, &l.NotificationType, &l.ReferenceID, &l.ReferenceType,
			&l.DaysBeforeExpiry, &l.ExpiryReferenceDate, &l.Subject, &l.SentAt,
		); err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}

	if logs == nil {
		logs = []*models.NotificationLog{}
	}

	return logs, total, nil
}
