package postgres

import (
	"fmt"
	"time"
)

// updatedSinceClause builds the optional delta-sync predicate used by the list
// repositories. The comparison is strictly-after so a client can hand back the
// highest updatedAt it has already stored and not receive that record again.
//
// It returns an empty fragment and no arguments when updatedSince is nil, so
// callers can append unconditionally.
func updatedSinceClause(updatedSince *time.Time, argNum int) (string, []any) {
	if updatedSince == nil {
		return "", nil
	}
	return fmt.Sprintf(" AND updated_at > $%d", argNum), []any{*updatedSince}
}
