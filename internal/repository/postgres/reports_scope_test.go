package postgres

import (
	"testing"

	"github.com/google/uuid"
)

func TestReportScopeWithLimit(t *testing.T) {
	userID := uuid.New()

	t.Run("positive limit binds a placeholder", func(t *testing.T) {
		args, ph := newReportScope(userID, 12).withLimit(25)
		if ph != "$3" {
			t.Errorf("placeholder = %q, want $3", ph)
		}
		if len(args) != 3 || args[2] != 25 {
			t.Errorf("args = %v, want [user 12 25]", args)
		}
	})

	t.Run("all-time scope numbers the limit after the user", func(t *testing.T) {
		args, ph := newReportScope(userID, 0).withLimit(5)
		if ph != "$2" || len(args) != 2 {
			t.Errorf("got %q with %d args, want $2 with 2", ph, len(args))
		}
	})

	for _, limit := range []int{0, -1} {
		args, ph := newReportScope(userID, 12).withLimit(limit)
		if ph != "ALL" {
			t.Errorf("withLimit(%d) placeholder = %q, want ALL", limit, ph)
		}
		if len(args) != 2 {
			t.Errorf("withLimit(%d) bound %d args, want only the scope's 2", limit, len(args))
		}
	}
}
