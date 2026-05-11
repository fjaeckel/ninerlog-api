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
type LegacyCrewInput struct {
	Person1, Person2, Person3, Person4, Person5, Person6 string
	InstructorName                                       string
	HasDualReceived                                      bool // any positive DualReceived value
	HasDualGiven                                         bool // any positive DualGiven value
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
func InferLegacyCrew(in LegacyCrewInput) []LegacyCrewMember {
	persons := [6]string{in.Person1, in.Person2, in.Person3, in.Person4, in.Person5, in.Person6}
	instructor := strings.TrimSpace(in.InstructorName)
	isTraining := instructor != "" || in.HasDualReceived
	isGiving := in.HasDualGiven

	var out []LegacyCrewMember

	// Person1
	if p1 := strings.TrimSpace(persons[0]); p1 != "" {
		switch {
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
		role := "Passenger"
		if isTraining {
			role = "Student"
		}
		out = append(out, LegacyCrewMember{Name: p2, Role: role})
	}

	// Persons 3-6: always Passenger.
	for _, name := range persons[2:] {
		if n := strings.TrimSpace(name); n != "" {
			out = append(out, LegacyCrewMember{Name: n, Role: "Passenger"})
		}
	}

	return out
}
