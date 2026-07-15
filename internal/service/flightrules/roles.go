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
	// user instruction, or a third-party Examiner is conducting a check
	// ride (Dual received).
	RoleDualReceiving
	// RoleDualGiving: the user is acting as instructor — either a Student
	// is on board, or the user themselves is listed with the Instructor
	// role (Dual given / FI).
	RoleDualGiving
	// RoleSIC: the user is co-pilot on a multi-pilot operation — another
	// person is listed with the PIC role, or the user themselves is listed
	// with the SIC role. Per AMC1 FCL.050 only the designated PIC logs PIC
	// time; the co-pilot logs co-pilot (SIC) time, even if both pilots are
	// qualified PICs (FOCA GM/INFO "Logging of Flight Time" §2.3.3).
	RoleSIC
)

// DetermineRole inspects the crew list to classify the user's pilot role.
//
// Precedence: a third-party Instructor or Examiner (name ≠ user) makes the
// user a Dual receiver, regardless of any Student also being present (e.g.
// observed CFI check rides). A Student or self-listed Instructor makes the
// user a Dual giver. A third-party PIC or a self-listed SIC makes the user
// the co-pilot (SIC) of a multi-pilot operation. Otherwise the user is PIC.
//
// A third-party Examiner counts as Dual received because there can only be
// one PIC per flight and the examiner occupying a pilot seat is PIC of
// record (NfL 2021-2-602 §4.2.2 no. 4; EASA AMC1 FCL.050). A self-listed
// Examiner leaves the user as PIC — an examiner logs their exam flights as
// PIC time.
//
// A third-party PIC counts as SIC for the same "only one PIC per flight"
// reason: if someone else was designated PIC, the user occupying the other
// pilot seat logs co-pilot time (AMC1 FCL.050; FOCA GM/INFO §2.3.3), even
// when both pilots hold PIC qualifications. A self-listed PIC crew entry
// keeps the user as PIC and wins over a simultaneous third-party PIC entry
// (conflicting data — trust the user's explicit self-declaration).
//
// When userName is empty, any Instructor, Examiner or PIC crew member is
// conservatively treated as a third party, preserving prior behaviour for
// callers that do not yet have user context.
func DetermineRole(flight *models.Flight, userName string) Role {
	hasOtherInstructor := false
	hasSelfInstructor := false
	hasOtherExaminer := false
	hasStudent := false
	hasOtherPIC := false
	hasSelfPIC := false
	hasSelfSIC := false
	for _, m := range flight.CrewMembers {
		switch m.Role {
		case models.CrewRoleInstructor:
			if userName != "" && MatchesUser(m.Name, userName) {
				hasSelfInstructor = true
			} else {
				hasOtherInstructor = true
			}
		case models.CrewRoleExaminer:
			if userName == "" || !MatchesUser(m.Name, userName) {
				hasOtherExaminer = true
			}
		case models.CrewRoleStudent:
			hasStudent = true
		case models.CrewRolePIC:
			if userName != "" && MatchesUser(m.Name, userName) {
				hasSelfPIC = true
			} else {
				hasOtherPIC = true
			}
		case models.CrewRoleSIC:
			if userName != "" && MatchesUser(m.Name, userName) {
				hasSelfSIC = true
			}
		}
	}
	if hasOtherInstructor || hasOtherExaminer {
		return RoleDualReceiving
	}
	if hasSelfInstructor || hasStudent {
		return RoleDualGiving
	}
	if (hasOtherPIC || hasSelfSIC) && !hasSelfPIC {
		return RoleSIC
	}
	return RolePIC
}

// IsMultiPilotOperation reports whether the crew composition indicates an
// operation flown with a two-pilot crew (multi-crew cooperation): the user
// is the co-pilot (RoleSIC), or a crew member holds the SIC role (the user
// is PIC with a co-pilot). Per AMC1 FCL.050 both pilots then log the full
// flight time in the multi-pilot column.
func IsMultiPilotOperation(flight *models.Flight, role Role) bool {
	if role == RoleSIC {
		return true
	}
	for _, m := range flight.CrewMembers {
		if m.Role == models.CrewRoleSIC {
			return true
		}
	}
	return false
}
