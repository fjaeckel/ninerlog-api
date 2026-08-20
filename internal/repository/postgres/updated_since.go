package postgres

import (
	"fmt"
	"time"
)

// updatedSinceClause builds the optional delta-sync predicate (strictly-after
// comparison). Returns an empty fragment and no arguments when updatedSince
// is nil.
func updatedSinceClause(updatedSince *time.Time, argNum int) (string, []any) {
	if updatedSince == nil {
		return "", nil
	}
	return fmt.Sprintf(" AND updated_at > $%d", argNum), []any{*updatedSince}
}
