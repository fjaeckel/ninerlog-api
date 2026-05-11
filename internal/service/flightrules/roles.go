// Package flightrules is the single source of truth for "who/what counts
// as PIC, Dual, FI, Night, IFR, MP/SP …" across the codebase.
//
// flightcalc/ owns the *write* path (mutates Flight on save). flightrules
// owns the *read* path (pure helpers consumed by handlers, exporters, PDF
// renderers, stats and tests). The two share the role + name primitives
// below so they cannot disagree.
//
// IMPORTANT: do NOT inline any of these rules in handlers/, repository/ or
// exporter code. A grep-guard in scripts/run-all-tests.sh enforces this.
package flightrules

import "github.com/fjaeckel/ninerlog-api/internal/models"

// Role classifies the user's pilot role on a flight, derived from the crew
// composition relative to the authenticated user's display name.
type Role int

const (
	// RolePIC: user is sole/lead pilot, no instruction context.
	RolePIC Role = iota
	// RoleDualReceiving: a third-party Instructor on board is giving the
	// user instruction (Dual received).
	RoleDualReceiving
	// RoleDualGiving: the user is acting as instructor — either a Student
	// is on board, or the user themselves is listed with the Instructor
	// role (Dual given / FI).
	RoleDualGiving
)

// DetermineRole inspects the crew list to classify the user's pilot role.
//
// Precedence: a third-party Instructor (name ≠ user) makes the user a Dual
// receiver, regardless of any Student also being present (e.g. observed CFI
// check rides). A Student or self-listed Instructor makes the user a Dual
// giver. Otherwise the user is PIC.
//
// When userName is empty, any Instructor crew member is conservatively
// treated as a third party (Dual received), preserving prior behaviour for
// callers that do not yet have user context.
func DetermineRole(flight *models.Flight, userName string) Role {
	hasOtherInstructor := false
	hasSelfInstructor := false
	hasStudent := false
	for _, m := range flight.CrewMembers {
		switch m.Role {
		case models.CrewRoleInstructor:
			if userName != "" && MatchesUser(m.Name, userName) {
				hasSelfInstructor = true
			} else {
				hasOtherInstructor = true
			}
		case models.CrewRoleStudent:
			hasStudent = true
		}
	}
	if hasOtherInstructor {
		return RoleDualReceiving
	}
	if hasSelfInstructor || hasStudent {
		return RoleDualGiving
	}
	return RolePIC
}
