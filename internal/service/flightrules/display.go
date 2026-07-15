package flightrules

import (
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// InstructorNameFromCrew returns the name of the first CrewMember with the
// Instructor role whose name does NOT match userName (i.e. a third-party
// instructor). Returns "" if no such member exists or if `f` is nil.
//
// userName is the authenticated user's display name and may be empty; in
// that case ALL Instructor crew members are treated as third parties
// (matches the conservative behaviour of DetermineRole).
func InstructorNameFromCrew(f *models.Flight, userName string) string {
	return thirdPartyNameFromCrew(f, userName, models.CrewRoleInstructor)
}

// ExaminerNameFromCrew is the Examiner-role counterpart of
// InstructorNameFromCrew: the first third-party Examiner in the crew, or "".
func ExaminerNameFromCrew(f *models.Flight, userName string) string {
	return thirdPartyNameFromCrew(f, userName, models.CrewRoleExaminer)
}

// PICNameFromCrew is the PIC-role counterpart of InstructorNameFromCrew:
// the first third-party PIC in the crew, or "". Used when the user flies as
// co-pilot (SIC) — the other pilot is PIC of record.
func PICNameFromCrew(f *models.Flight, userName string) string {
	return thirdPartyNameFromCrew(f, userName, models.CrewRolePIC)
}

func thirdPartyNameFromCrew(f *models.Flight, userName string, role models.CrewRole) string {
	if f == nil {
		return ""
	}
	for _, m := range f.CrewMembers {
		if m.Role != role {
			continue
		}
		name := strings.TrimSpace(m.Name)
		if name == "" {
			continue
		}
		if userName != "" && MatchesUser(name, userName) {
			continue
		}
		return name
	}
	return ""
}

// isSelfPlaceholder reports whether s is the literal "Self"/"SELF" placeholder
// (case-insensitive, ignoring surrounding whitespace). When an actual
// instructor is known for the flight, this value is wrong — the instructor is
// PIC of record — and the instructor's name should override it.
func isSelfPlaceholder(s string) bool {
	return strings.EqualFold(strings.TrimSpace(s), "self")
}

// DisplayPICName returns the value to render in the "PIC Name" column of a
// logbook row, applying the canonical fallback chain:
//
//  1. flight.PICName, if set (but stale "Self" on a Dual flight is treated as
//     unset so the instructor — who is PIC of record on a Dual — wins)
//  2. flight.InstructorName, if set (legacy column; pre-crew-table flights)
//  3. first non-self Instructor in flight.CrewMembers (modern data shape:
//     the FE only writes the instructor into CrewMembers, leaving
//     InstructorName nil — the instructor *is* PIC of record on a Dual)
//  4. first non-self Examiner in flight.CrewMembers (an exam flight is Dual
//     and the examiner is PIC of record)
//  5. original PICName if present (e.g. "Self" with no instructor available)
//  6. "SELF"
//
// userName is the authenticated user's display name; pass "" if unknown.
// All exporters (CSV, PDF, future formats) MUST use this helper instead of
// re-implementing the fallback locally.
func DisplayPICName(f *models.Flight, userName string) string {
	if f == nil {
		return "SELF"
	}
	// Resolve any known instructor/examiner first — used both as the primary
	// fallback when PICName is empty AND to override a stale "Self" value
	// (the candidate is never PIC of record when an instructor or examiner
	// is on board).
	var instructor string
	if f.InstructorName != nil && strings.TrimSpace(*f.InstructorName) != "" {
		instructor = strings.TrimSpace(*f.InstructorName)
	}
	if instructor == "" {
		instructor = InstructorNameFromCrew(f, userName)
	}
	if instructor == "" {
		instructor = ExaminerNameFromCrew(f, userName)
	}
	if instructor == "" {
		// A third-party PIC crew member (user flying as co-pilot) is PIC
		// of record.
		instructor = PICNameFromCrew(f, userName)
	}

	picSet := f.PICName != nil && strings.TrimSpace(*f.PICName) != ""
	// Stale "Self" is overridden whenever an actual instructor is known,
	// regardless of IsDual (legacy data can have mismatched flags).
	staleSelf := picSet && isSelfPlaceholder(*f.PICName) && instructor != ""
	if picSet && !staleSelf {
		return *f.PICName
	}
	if instructor != "" {
		return instructor
	}
	if picSet {
		// No instructor info available — preserve the original value
		// (typically "Self") rather than losing it entirely.
		return *f.PICName
	}
	return "SELF"
}

// ResolvePICNameForSave returns the value that should be persisted into
// `flight.PICName` when a flight is saved. The write-path equivalent of
// DisplayPICName:
//
//   - existing PICName wins (user explicitly set it),
//   - if the user is PIC and no instructor is involved → "Self",
//   - if the user is Dual and an InstructorName is set → that instructor,
//   - if the user is Dual and a non-self Instructor exists in CrewMembers
//     → that instructor's name (modern data shape),
//   - if the user is Dual and a non-self Examiner exists in CrewMembers
//     → that examiner's name (exam flight: the examiner is PIC of record),
//   - otherwise nil (column stays empty; exporter will fall through to
//     DisplayPICName's "SELF" default at render time).
//
// The CRUD handler must call this once at save time so the persisted
// column is canonical.
func ResolvePICNameForSave(f *models.Flight, userName string) *string {
	if f == nil {
		return nil
	}
	picSet := f.PICName != nil && strings.TrimSpace(*f.PICName) != ""
	// Resolve any known instructor first so a stale "Self" can be cleaned
	// up regardless of the IsDual flag (legacy data may have mismatched
	// flags).
	var instructor *string
	if f.InstructorName != nil && strings.TrimSpace(*f.InstructorName) != "" {
		instructor = f.InstructorName
	} else if n := InstructorNameFromCrew(f, userName); n != "" {
		instructor = &n
	} else if n := ExaminerNameFromCrew(f, userName); n != "" {
		instructor = &n
	} else if n := PICNameFromCrew(f, userName); n != "" {
		// User flying as co-pilot: the third-party PIC is PIC of record.
		instructor = &n
	}
	// Existing PICName wins — except a stale "Self" when an actual
	// instructor is known (instructor is PIC of record on a Dual).
	staleSelf := picSet && isSelfPlaceholder(*f.PICName) && instructor != nil
	if picSet && !staleSelf {
		return f.PICName
	}
	if instructor != nil {
		return instructor
	}
	if f.IsPIC && !f.IsDual {
		s := "Self"
		return &s
	}
	if picSet {
		// Stale "Self" with no instructor info available: preserve the
		// original rather than clearing (exporter will still render it;
		// user can correct manually).
		return f.PICName
	}
	return nil
}

// PilotingCategory is the per-row bucket used for the EASA AMC1 FCL.050
// columns "SP-SE" / "SP-ME" / "Multi-Pilot".
type PilotingCategory int

const (
	// CategorySPSE: single-pilot single-engine.
	CategorySPSE PilotingCategory = iota
	// CategorySPME: single-pilot multi-engine.
	CategorySPME
	// CategoryMP: multi-pilot.
	CategoryMP
)

// PilotingCategoryFor returns the bucket for `flight`. acClass is the user's
// stored AircraftClass for this registration (e.g. "SEP", "MEP", "SET",
// "MET", "SES"), passed in by callers that have aircraft data. When acClass
// is empty (no fleet entry available) the rule defaults to SP-SE.
//
// Rule:
//   - MultiPilotTime > 0 ⇒ MP (user-declared)
//   - acClass starts with "ME" or "MET" or "SET" ⇒ SP-ME
//   - otherwise ⇒ SP-SE
func PilotingCategoryFor(f *models.Flight, acClass string) PilotingCategory {
	if f != nil && f.MultiPilotTime > 0 {
		return CategoryMP
	}
	c := strings.ToUpper(strings.TrimSpace(acClass))
	if strings.HasPrefix(c, "MEP") || strings.HasPrefix(c, "MET") || strings.HasPrefix(c, "SET") {
		return CategorySPME
	}
	return CategorySPSE
}

// RowTimes returns the minutes that should be filled into the SP-SE / SP-ME
// / MP columns for `flight` given its piloting category. Exactly one of the
// three returned values is non-zero (the others are 0).
func RowTimes(f *models.Flight, acClass string) (spSE, spME, mp int) {
	if f == nil {
		return 0, 0, 0
	}
	switch PilotingCategoryFor(f, acClass) {
	case CategoryMP:
		return 0, 0, f.MultiPilotTime
	case CategorySPME:
		return 0, f.TotalTime, 0
	default:
		return f.TotalTime, 0, 0
	}
}
