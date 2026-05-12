package flightrules

import "strings"

// LegacyCrewMember is a name+role pair returned by InferLegacyCrew. Callers
// map it to their own crew type (generated.FlightCrewMemberInput,
// models.FlightCrewMember, …). Role values match the strings in
// models.CrewRole ("PIC", "SIC", "Instructor", "Student", "Passenger").
type LegacyCrewMember struct {
	Name string
	Role string
}

// LegacyCrewInput describes the flat set of legacy/spreadsheet columns we
// historically used to encode crew before the CrewMembers array existed:
// up to six "personN" names, an InstructorName, and the user-declared
// DualReceived / DualGiven totals.
//
// ForeFlight optionally encodes a role tag inside each Person cell
// ("Name;Role;email"). When that explicit role is available the caller
// SHOULD pass it via the matching PersonNRole field so positional
// inference does not overwrite the source-of-truth tag (e.g. a flight
// where the instructor is Person2 and Person1 is the student).
type LegacyCrewInput struct {
	Person1, Person2, Person3, Person4, Person5, Person6                 string
	Person1Role, Person2Role, Person3Role, Person4Role, Person5Role, Person6Role string
	InstructorName                                                       string
	HasDualReceived                                                      bool // any positive DualReceived value
	HasDualGiven                                                         bool // any positive DualGiven value
}

// NormalizeLegacyRole maps a free-form role string (case-insensitive) to
// one of our canonical CrewRole values. Unknown / empty strings return ""
// to signal "fall back to positional inference".
//
// ForeFlight's role vocabulary in the structured Person cell includes
// PIC, SIC, Student, Instructor, FlightAttendant, Observer, Engineer and
// Other; the latter four are flattened to Passenger.
func NormalizeLegacyRole(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "pic":
		return "PIC"
	case "sic":
		return "SIC"
	case "instructor", "cfi", "cfii":
		return "Instructor"
	case "student":
		return "Student"
	case "passenger", "pax",
		"flightattendant", "flight attendant",
		"observer", "engineer", "other":
		return "Passenger"
	}
	return ""
}

// InferLegacyCrew is the single source of truth that converts the legacy
// flat crew columns (used by ForeFlight / Garmin / older spreadsheet CSVs)
// into a CrewMembers array. Both the CSV importer and any future legacy
// re-importer MUST use this — do not re-derive crew roles by hand.
//
// Rules (preserved exactly from the historical importer):
//   - Training flight  ≡ InstructorName set OR HasDualReceived.
//   - Instructor-giving ≡ HasDualGiven (and not a training flight).
//   - Person1 is treated as Instructor on a training flight, Student when
//     the user is giving instruction, else PIC. If InstructorName is set
//     and differs from Person1, both are emitted (Person1 keeps a PIC
//     role, InstructorName is added as Instructor).
//   - Person2 is Student on a training flight, Passenger otherwise.
//   - Persons 3-6 are always Passenger.
//   - InstructorName with no Person1 is emitted as Instructor on its own.
//
// When the caller supplies an explicit role for a person via the
// matching PersonNRole field (see NormalizeLegacyRole for the accepted
// vocabulary), that role wins over the positional rules for that
// person. Other persons still follow the positional rules so a partial
// set of tags works (e.g. only Person1 tagged "Student", Person2 left
// untagged still becomes the Passenger / Student per the positional
// rule).
func InferLegacyCrew(in LegacyCrewInput) []LegacyCrewMember {
	persons := [6]string{in.Person1, in.Person2, in.Person3, in.Person4, in.Person5, in.Person6}
	roles := [6]string{
		NormalizeLegacyRole(in.Person1Role),
		NormalizeLegacyRole(in.Person2Role),
		NormalizeLegacyRole(in.Person3Role),
		NormalizeLegacyRole(in.Person4Role),
		NormalizeLegacyRole(in.Person5Role),
		NormalizeLegacyRole(in.Person6Role),
	}
	instructor := strings.TrimSpace(in.InstructorName)
	isTraining := instructor != "" || in.HasDualReceived
	isGiving := in.HasDualGiven

	var out []LegacyCrewMember

	// Person1
	if p1 := strings.TrimSpace(persons[0]); p1 != "" {
		switch {
		case roles[0] != "":
			// Explicit role tag from the source file wins. If an
			// InstructorName is also supplied and refers to a
			// different person, emit them separately.
			if isTraining && instructor != "" && !strings.EqualFold(p1, instructor) {
				out = append(out, LegacyCrewMember{Name: instructor, Role: "Instructor"})
			}
			out = append(out, LegacyCrewMember{Name: p1, Role: roles[0]})
		case isTraining && instructor != "" && !strings.EqualFold(p1, instructor):
			// Different instructor named separately: emit both.
			out = append(out,
				LegacyCrewMember{Name: instructor, Role: "Instructor"},
				LegacyCrewMember{Name: p1, Role: "PIC"},
			)
		case isTraining:
			out = append(out, LegacyCrewMember{Name: p1, Role: "Instructor"})
		case isGiving:
			out = append(out, LegacyCrewMember{Name: p1, Role: "Student"})
		default:
			out = append(out, LegacyCrewMember{Name: p1, Role: "PIC"})
		}
	} else if instructor != "" {
		out = append(out, LegacyCrewMember{Name: instructor, Role: "Instructor"})
	}

	// Person2
	if p2 := strings.TrimSpace(persons[1]); p2 != "" {
		role := roles[1]
		if role == "" {
			role = "Passenger"
			if isTraining {
				role = "Student"
			}
		}
		out = append(out, LegacyCrewMember{Name: p2, Role: role})
	}

	// Persons 3-6: explicit tag if present, else Passenger.
	for i, name := range persons[2:] {
		if n := strings.TrimSpace(name); n != "" {
			role := roles[i+2]
			if role == "" {
				role = "Passenger"
			}
			out = append(out, LegacyCrewMember{Name: n, Role: role})
		}
	}

	return out
}
