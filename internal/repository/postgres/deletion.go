package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type deletionRepository struct {
	db *sql.DB
}

// NewDeletionRepository creates a repository over trigger-written deletion
// tombstones. It is read-and-sweep only.
func NewDeletionRepository(db *sql.DB) repository.DeletionRepository {
	return &deletionRepository{db: db}
}

// deletionFilter builds the shared WHERE clause. entity is optional and
// pre-validated against the enum.
func deletionFilter(userID uuid.UUID, since time.Time, entity *models.DeletionEntityType) (string, []any) {
	where := " WHERE user_id = $1 AND deleted_at > $2"
	args := []any{userID, since}
	if entity != nil {
		where += " AND entity_type = $3"
		args = append(args, string(*entity))
	}
	return where, args
}

// ListSince returns the user's deletions after the watermark, oldest first,
// tie-broken on id.
func (r *deletionRepository) ListSince(
	ctx context.Context, userID uuid.UUID, since time.Time,
	entity *models.DeletionEntityType, limit, offset int,
) ([]*models.Deletion, error) {
	where, args := deletionFilter(userID, since, entity)
	query := `SELECT user_id, entity_type, entity_id, deleted_at FROM deletion_tombstones` +
		where +
		fmt.Sprintf(" ORDER BY deleted_at ASC, id ASC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deletions []*models.Deletion
	for rows.Next() {
		d := &models.Deletion{}
		if err := rows.Scan(&d.UserID, &d.EntityType, &d.EntityID, &d.DeletedAt); err != nil {
			return nil, err
		}
		deletions = append(deletions, d)
	}
	return deletions, rows.Err()
}

// CountSince counts the same set ListSince pages over.
func (r *deletionRepository) CountSince(
	ctx context.Context, userID uuid.UUID, since time.Time, entity *models.DeletionEntityType,
) (int, error) {
	where, args := deletionFilter(userID, since, entity)
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM deletion_tombstones`+where, args...).Scan(&count)
	return count, err
}

// DeleteExpired removes tombstones older than `before`. Returns how many were
// swept.
func (r *deletionRepository) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM deletion_tombstones WHERE deleted_at < $1`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
