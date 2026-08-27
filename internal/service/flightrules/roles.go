// Package flightrules is the single source of truth for "who/what counts
// as PIC, Dual, FI, Night, IFR, MP/SP …" across the codebase.
//
// flightcalc/ owns the *write* path (mutates Flight on save). flightrules
// owns the *read* path (pure helpers consumed by handlers, exporters, PDF
// renderers, stats and tests). The two share the role + name primitives
// below.
//
// IMPORTANT: do NOT inline any of these rules in handlers/, repository/ or
// exporter code. A grep-guard in scripts/run-all-tests.sh enforces this.
package flightrules

import "github.com/fjaeckel/ninerlog-api/internal/models"

// AircraftFacts carries the fleet facts the rules need about the aircraft a
// flight was flown in. A nil *AircraftFacts means the registration has no
// fleet entry and nothing is known about it.
type AircraftFacts struct {
	Registration string
	IsMultiPilot bool
}

// IsMultiPilotAircraft reports whether the aircraft is certificated for a
// minimum crew of two pilots. An unknown aircraft is not.
func (a *AircraftFacts) IsMultiPilotAircraft() bool {
	return a != nil && a.IsMultiPilot
}

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
	// RolePassenger: another person is PIC and the operation does not carry
	// a second pilot, so the user is carried rather than crewed and logs no
	// pilot function time.
	RolePassenger
)

// MayLogCoPilotTime reports whether the operation permits the user to log
// co-pilot time.
//
// Co-pilot time requires a seat that the operation actually calls for:
//
//   - a multi-pilot aircraft — certificated for a minimum crew of two
//     (EASA FCL.010; 14 CFR §61.51(f)(1) "aircraft type certificated for
//     more than one pilot");
//   - a required safety pilot during simulated instrument flight
//     (14 CFR §91.109(b), loggable under §61.51(f)(2)) — the user is listed
//     with the SafetyPilot role;
//   - the user declaring the co-pilot seat themselves, by listing their own
//     SIC crew entry or by entering a co-pilot time directly
//     (SICTimeOverride). Two-pilot operations mandated by an operations
//     manual rather than by the type certificate (EASA FCL.010;
//     §61.51(f)(2), §135.99(c)) are recorded this way.
//
// A single-pilot aircraft with a third-party PIC and no such declaration
// carries no co-pilot seat, whoever else is on board.
//
// A derived SICTime is not a declaration: only SICTimeOverride distinguishes
// a co-pilot time the pilot entered from one a previous derivation wrote, so
// re-deriving can still correct a row it filled in itself.
func MayLogCoPilotTime(flight *models.Flight, userName string, aircraft *AircraftFacts) bool {
	return mayLogCoPilotTime(flight, classifyCrew(flight, userName), aircraft)
}

func mayLogCoPilotTime(flight *models.Flight, crew crewComposition, aircraft *AircraftFacts) bool {
	if aircraft.IsMultiPilotAircraft() || crew.selfSafetyPilot || crew.selfSIC {
		return true
	}
	if HasDeclaredFunctionTime(flight) {
		return true
	}
	return flight.SICTimeOverride && flight.SICTime > 0
}

// HasDeclaredFunctionTime reports whether the pilot declared any PICUS, SPIC
// or cruise relief minutes on the flight. Each names a pilot-flying (or
// relief) seat the operation provides, so it declares the seat the same way
// an entered co-pilot time does.
func HasDeclaredFunctionTime(f *models.Flight) bool {
	return f.PICUSTime > 0 || f.SPICTime > 0 || f.ReliefTime > 0
}

// crewComposition is the crew list reduced to the facts the rules read.
type crewComposition struct {
	otherInstructor bool
	selfInstructor  bool
	otherExaminer   bool
	student         bool
	otherPIC        bool
	selfPIC         bool
	selfSIC         bool
	selfSafetyPilot bool
	empty           bool
}

// classifyCrew reduces a flight's crew list relative to the user's display
// name. When userName is empty, any Instructor, Examiner or PIC crew member
// is treated as a third party.
func classifyCrew(flight *models.Flight, userName string) crewComposition {
	c := crewComposition{empty: len(flight.CrewMembers) == 0}
	isSelf := func(name string) bool {
		return userName != "" && MatchesUser(name, userName)
	}
	for _, m := range flight.CrewMembers {
		switch m.Role {
		case models.CrewRoleInstructor:
			if isSelf(m.Name) {
				c.selfInstructor = true
			} else {
				c.otherInstructor = true
			}
		case models.CrewRoleExaminer:
			if !isSelf(m.Name) {
				c.otherExaminer = true
			}
		case models.CrewRoleStudent:
			c.student = true
		case models.CrewRolePIC:
			if isSelf(m.Name) {
				c.selfPIC = true
			} else {
				c.otherPIC = true
			}
		case models.CrewRoleSIC:
			if isSelf(m.Name) {
				c.selfSIC = true
			}
		case models.CrewRoleSafetyPilot:
			if isSelf(m.Name) {
				c.selfSafetyPilot = true
			}
		}
	}
	return c
}

// DetermineRole inspects the crew list to classify the user's pilot role.
// aircraft carries the fleet facts for the registration flown, or nil when
// the registration has no fleet entry.
//
// Precedence: a third-party Instructor or Examiner (name ≠ user) makes the
// user a Dual receiver, regardless of any Student also being present
// (NfL 2021-2-602 §4.2.2 no. 4; EASA AMC1 FCL.050). A Student or self-listed
// Instructor makes the user a Dual giver. A third-party PIC or a self-listed
// SIC makes the user the co-pilot (SIC) — but only where the operation
// permits co-pilot time (MayLogCoPilotTime); otherwise the user is carried
// as a passenger. A self-listed Examiner leaves the user as PIC; a
// self-listed PIC crew entry keeps the user as PIC and wins over a
// simultaneous third-party PIC entry. With no crew list at all, a declared
// SICTime makes the user the co-pilot. Otherwise the user is PIC.
func DetermineRole(flight *models.Flight, userName string, aircraft *AircraftFacts) Role {
	crew := classifyCrew(flight, userName)

	if crew.otherInstructor || crew.otherExaminer {
		return RoleDualReceiving
	}
	if crew.selfInstructor || crew.student {
		return RoleDualGiving
	}
	if (crew.otherPIC || crew.selfSIC) && !crew.selfPIC {
		if mayLogCoPilotTime(flight, crew, aircraft) {
			return RoleSIC
		}
		return RolePassenger
	}
	// A self-listed safety pilot occupies a required crew seat even when no
	// PIC crew entry names the pilot flying (14 CFR §91.109(b)).
	if crew.selfSafetyPilot && !crew.selfPIC {
		return RoleSIC
	}
	// Legacy/imported rows carry no crew list but declare co-pilot time; a
	// declared PICUS/SPIC/relief time likewise places the user in a
	// supervised or relief seat rather than as PIC.
	if crew.empty && ((flight.SICTimeOverride && flight.SICTime > 0) || HasDeclaredFunctionTime(flight)) {
		return RoleSIC
	}
	return RolePIC
}

// IsMultiPilotOperation reports whether the flight was flown with a
// two-pilot crew, filling the EASA AMC1 FCL.050 multi-pilot column (Col 10).
//
// It requires a multi-pilot aircraft: the column records time flown in
// aeroplanes certificated for a minimum crew of two (FCL.010). A safety
// pilot in a single-pilot aeroplane is a required crew member under
// 14 CFR §91.109(b) and logs co-pilot time, but the aeroplane remains
// single-pilot and the time is not multi-pilot time.
func IsMultiPilotOperation(flight *models.Flight, role Role, aircraft *AircraftFacts) bool {
	if !aircraft.IsMultiPilotAircraft() {
		return false
	}
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
