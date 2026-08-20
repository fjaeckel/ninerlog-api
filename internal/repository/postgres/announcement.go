package postgres

import (
	"context"
	"database/sql"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
	"time"
)

type announcementRepository struct {
	db *sql.DB
}

// NewAnnouncementRepository creates a system announcement repository.
func NewAnnouncementRepository(db *sql.DB) repository.AnnouncementRepository {
	return &announcementRepository{db: db}
}

func (r *announcementRepository) ListActive(ctx context.Context, now time.Time) ([]*models.SystemAnnouncement, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, message, severity, expires_at, created_at
		FROM system_announcements
		WHERE expires_at IS NULL OR expires_at > $1
		ORDER BY created_at DESC
	`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var announcements []*models.SystemAnnouncement
	for rows.Next() {
		a := &models.SystemAnnouncement{}
		if err := rows.Scan(&a.ID, &a.Message, &a.Severity, &a.ExpiresAt, &a.CreatedAt); err != nil {
			continue
		}
		announcements = append(announcements, a)
	}
	return announcements, rows.Err()
}

func (r *announcementRepository) Create(ctx context.Context, a *models.SystemAnnouncement) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO system_announcements (id, message, severity, expires_at, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, a.ID, a.Message, a.Severity, a.ExpiresAt, a.CreatedBy, a.CreatedAt)
	return err
}

func (r *announcementRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM system_announcements WHERE id = $1", id)
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
