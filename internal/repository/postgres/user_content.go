package postgres

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type userContentRepository struct {
	db *sql.DB
}

// NewUserContentRepository creates the repository behind the "delete all my
// data" account wipe.
func NewUserContentRepository(db *sql.DB) repository.UserContentRepository {
	return &userContentRepository{db: db}
}

func (r *userContentRepository) DeleteAllContent(ctx context.Context, userID uuid.UUID) error {
	// Delete in dependency order to respect FK constraints:
	// 1. flight_crew_members (depends on flights, contacts)
	// 2. flight_imports (depends on users)
	// 3. flights (depends on users)
	// 4. class_ratings (depends on licenses)
	// 5. licenses (depends on users)
	// 6. aircraft (depends on users)
	// 7. contacts (depends on users)
	// 8. credentials (depends on users)
	// 9. notification_log (depends on users)
	queries := []string{
		`DELETE FROM flight_crew_members WHERE flight_id IN (SELECT id FROM flights WHERE user_id = $1)`,
		`DELETE FROM flight_imports WHERE user_id = $1`,
		`DELETE FROM flights WHERE user_id = $1`,
		`DELETE FROM class_ratings WHERE license_id IN (SELECT id FROM licenses WHERE user_id = $1)`,
		`DELETE FROM licenses WHERE user_id = $1`,
		`DELETE FROM aircraft WHERE user_id = $1`,
		`DELETE FROM contacts WHERE user_id = $1`,
		`DELETE FROM credentials WHERE user_id = $1`,
		`DELETE FROM notification_log WHERE user_id = $1`,
	}

	// One transaction, and errors are surfaced: a mid-sequence failure must
	// not leave the account half-deleted — flights gone, licenses still
	// present — while the caller is told the wipe succeeded. For a
	// destructive, irreversible operation that is the worst possible failure
	// mode.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // no-op once committed

	for _, q := range queries {
		if _, err := tx.ExecContext(ctx, q, userID); err != nil {
			slog.Error("bulk delete failed, rolling back", "user_id", userID, "error", err)
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		slog.Error("bulk delete commit failed", "user_id", userID, "error", err)
		return err
	}
	return nil
}
