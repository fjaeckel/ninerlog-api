package postgres

import (
	"context"
	"database/sql"

	"github.com/fjaeckel/ninerlog-api/internal/repository"
)

// applyChangeContext sets the transaction-local change settings read by the
// flight revision triggers. It is a no-op when ctx carries no change context.
func applyChangeContext(ctx context.Context, tx *sql.Tx) error {
	cc, ok := repository.ChangeContextFrom(ctx)
	if !ok {
		return nil
	}
	reason := ""
	if cc.Reason != nil {
		reason = *cc.Reason
	}
	correction := "false"
	if cc.IsCorrection {
		correction = "true"
	}
	_, err := tx.ExecContext(ctx,
		`SELECT set_config('ninerlog.change_source', $1, true),
		        set_config('ninerlog.change_reason', $2, true),
		        set_config('ninerlog.change_correction', $3, true)`,
		cc.Source, reason, correction)
	return err
}

// beginChangeTx starts a transaction with the change context applied.
func beginChangeTx(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if err := applyChangeContext(ctx, tx); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}
