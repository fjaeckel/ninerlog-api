package postgres

import (
	"strings"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/repository"
)

// TestAppendRegistrationFilter covers the SQL fragment used by the logbook
// class filter. This is the fix for the bug where the filter was previously
// applied in-memory AFTER pagination, causing multi-page logbooks to be badly
// undercounted (a pilot with 96 SEP flights only ever saw the ~18 that happened
// to land on the first page).
func TestAppendRegistrationFilter(t *testing.T) {
	t.Run("disabled filter leaves query untouched", func(t *testing.T) {
		q, args, argNum := appendRegistrationFilter("WHERE user_id = $1", []interface{}{"u"}, 2, nil)
		if q != "WHERE user_id = $1" {
			t.Errorf("Expected query unchanged, got %q", q)
		}
		if len(args) != 1 || argNum != 2 {
			t.Errorf("Expected args/argNum unchanged, got args=%v argNum=%d", args, argNum)
		}

		opts := &repository.FlightQueryOptions{FilterByRegistrations: false, AircraftRegistrations: []string{"D-ESEP"}}
		q2, _, _ := appendRegistrationFilter("WHERE user_id = $1", []interface{}{"u"}, 2, opts)
		if q2 != "WHERE user_id = $1" {
			t.Errorf("Filter disabled but query changed: %q", q2)
		}
	})

	t.Run("empty registration set matches nothing", func(t *testing.T) {
		opts := &repository.FlightQueryOptions{FilterByRegistrations: true, AircraftRegistrations: nil}
		q, args, argNum := appendRegistrationFilter("WHERE user_id = $1", []interface{}{"u"}, 2, opts)
		if !strings.Contains(q, "AND 1=0") {
			t.Errorf("Expected impossible condition for empty set, got %q", q)
		}
		if len(args) != 1 || argNum != 2 {
			t.Errorf("Expected no new args for empty set, got args=%v argNum=%d", args, argNum)
		}
	})

	t.Run("builds parameterized IN clause", func(t *testing.T) {
		opts := &repository.FlightQueryOptions{
			FilterByRegistrations: true,
			AircraftRegistrations: []string{"D-ESEP", "D-EMPY"},
		}
		q, args, argNum := appendRegistrationFilter("WHERE user_id = $1", []interface{}{"u"}, 2, opts)
		want := "WHERE user_id = $1 AND UPPER(aircraft_reg) IN ($2, $3)"
		if q != want {
			t.Errorf("Unexpected query.\n got: %q\nwant: %q", q, want)
		}
		if len(args) != 3 || args[1] != "D-ESEP" || args[2] != "D-EMPY" {
			t.Errorf("Unexpected args: %v", args)
		}
		if argNum != 4 {
			t.Errorf("Expected argNum 4, got %d", argNum)
		}
	})
}
