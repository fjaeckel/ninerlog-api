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

// NewUserContentRepository creates the account-content wipe repository.
func NewUserContentRepository(db *sql.DB) repository.UserContentRepository {
	return &userContentRepository{db: db}
}

const (
	wipeCrewQuery    = `DELETE FROM flight_crew_members WHERE flight_id IN (SELECT id FROM flights WHERE user_id = $1)`
	wipeImportsQuery = `DELETE FROM flight_imports WHERE user_id = $1`
	wipeFlightsQuery = `DELETE FROM flights WHERE user_id = $1`
)

// logbookWipeQueries delete a user's logbook content in dependency order:
// crew members, import history, flights, class ratings, licences, aircraft,
// contacts, credentials.
var logbookWipeQueries = []string{
	wipeCrewQuery,
	wipeImportsQuery,
	wipeFlightsQuery,
	`DELETE FROM class_ratings WHERE license_id IN (SELECT id FROM licenses WHERE user_id = $1)`,
	`DELETE FROM licenses WHERE user_id = $1`,
	`DELETE FROM aircraft WHERE user_id = $1`,
	`DELETE FROM contacts WHERE user_id = $1`,
	`DELETE FROM credentials WHERE user_id = $1`,
}

func (r *userContentRepository) DeleteAllContent(ctx context.Context, userID uuid.UUID) error {
	queries := append(append([]string{}, logbookWipeQueries...),
		`DELETE FROM notification_log WHERE user_id = $1`)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // no-op once committed

	ctx = withUserEditSource(ctx)
	if err := applyChangeContext(ctx, tx); err != nil {
		return err
	}
	if _, err := (&restoreTx{tx: tx}).deleteClassifiedCorrections(ctx, userID); err != nil {
		return err
	}

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

func (r *userContentRepository) DeleteFlightsAndHistory(ctx context.Context, userID uuid.UUID) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }() // no-op once committed

	ctx = withUserEditSource(ctx)
	if err := applyChangeContext(ctx, tx); err != nil {
		return 0, err
	}
	flagged, err := (&restoreTx{tx: tx}).deleteClassifiedCorrections(ctx, userID)
	if err != nil {
		return 0, err
	}

	for _, q := range []string{wipeCrewQuery, wipeImportsQuery} {
		if _, err := tx.ExecContext(ctx, q, userID); err != nil {
			slog.Error("flight delete failed, rolling back", "user_id", userID, "error", err)
			return 0, err
		}
	}
	res, err := tx.ExecContext(ctx, wipeFlightsQuery, userID)
	if err != nil {
		slog.Error("flight delete failed, rolling back", "user_id", userID, "error", err)
		return 0, err
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		slog.Error("flight delete commit failed", "user_id", userID, "error", err)
		return 0, err
	}
	return deleted + flagged, nil
}

// withUserEditSource marks the writes of a bulk delete as user edits.
func withUserEditSource(ctx context.Context) context.Context {
	return repository.WithChangeSource(ctx, repository.ChangeSourceUserEdit)
}
