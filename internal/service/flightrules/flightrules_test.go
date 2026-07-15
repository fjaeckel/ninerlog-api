package flightrules

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func TestNormalizeName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"Amelia Earhart", "amelia earhart"},
		{"  Amelia Earhart  ", "amelia earhart"},
		{"Amelia  Earhart", "amelia earhart"},      // collapse internal whitespace
		{"Amelia\u00a0Earhart", "amelia earhart"}, // U+00A0 nbsp
		{"AMELIA\tEARHART\n", "amelia earhart"},
	}
	for _, tc := range cases {
		if got := NormalizeName(tc.in); got != tc.want {
			t.Errorf("NormalizeName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMatchesUser(t *testing.T) {
	cases := []struct {
		candidate, user string
		want            bool
	}{
		{"Amelia Earhart", "Amelia Earhart", true},
		{"amelia earhart", "AMELIA EARHART", true},
		{"Amelia  Earhart", "Amelia Earhart", true},
		{"Earhart, Amelia", "Amelia Earhart", true},
		{"Amelia Earhart", "Earhart, Amelia", true},
		{"Amelia M. Earhart", "Amelia Earhart", false},
		{"", "Amelia Earhart", false},
		{"Amelia Earhart", "", false},
	}
	for _, tc := range cases {
		if got := MatchesUser(tc.candidate, tc.user); got != tc.want {
			t.Errorf("MatchesUser(%q, %q) = %v, want %v", tc.candidate, tc.user, got, tc.want)
		}
	}
}

func TestDisplayPICName(t *testing.T) {
	s := func(v string) *string { return &v }
	crewInstr := func(name string) []models.FlightCrewMember {
		return []models.FlightCrewMember{{Name: name, Role: models.CrewRoleInstructor}}
	}
	crewExam := func(name string) []models.FlightCrewMember {
		return []models.FlightCrewMember{{Name: name, Role: models.CrewRoleExaminer}}
	}

	cases := []struct {
		name     string
		f        *models.Flight
		userName string
		want     string
	}{
		{"nil", nil, "Amelia Earhart", "SELF"},
		{"empty", &models.Flight{}, "Amelia Earhart", "SELF"},
		{"pic set", &models.Flight{PICName: s("CPT Doe")}, "Amelia Earhart", "CPT Doe"},
		{"instructor only", &models.Flight{InstructorName: s("CFI Mueller")}, "Amelia Earhart", "CFI Mueller"},
		{"pic blank, instructor set", &models.Flight{PICName: s("  "), InstructorName: s("CFI Mueller")}, "Amelia Earhart", "CFI Mueller"},
		{"both set, pic wins", &models.Flight{PICName: s("CPT Doe"), InstructorName: s("CFI Mueller")}, "Amelia Earhart", "CPT Doe"},
		// Modern data shape: legacy InstructorName is nil, instructor only in CrewMembers.
		{"crew instructor only", &models.Flight{CrewMembers: crewInstr("CFI Mueller")}, "Amelia Earhart", "CFI Mueller"},
		{"crew instructor reversed name", &models.Flight{CrewMembers: crewInstr("Mueller, CFI")}, "Amelia Earhart", "Mueller, CFI"},
		{"crew instructor is self → SELF", &models.Flight{CrewMembers: crewInstr("Earhart, Amelia")}, "Amelia Earhart", "SELF"},
		{"crew instructor self + empty userName → falls through (treated as third party)", &models.Flight{CrewMembers: crewInstr("Amelia Earhart")}, "", "Amelia Earhart"},
		{"pic set wins over crew", &models.Flight{PICName: s("CPT Doe"), CrewMembers: crewInstr("CFI Mueller")}, "Amelia Earhart", "CPT Doe"},
		{"legacy instructor wins over crew", &models.Flight{InstructorName: s("Legacy Instructor"), CrewMembers: crewInstr("Crew Instructor")}, "Amelia Earhart", "Legacy Instructor"},
		// Stale "Self" must NOT mask the actual instructor — regardless of
		// IsDual (legacy data may have mismatched flags).
		{"dual + stale Self + crew instructor → instructor", &models.Flight{PICName: s("Self"), IsDual: true, CrewMembers: crewInstr("CFI Mueller")}, "Amelia Earhart", "CFI Mueller"},
		{"dual + stale SELF + legacy instructor → instructor", &models.Flight{PICName: s("SELF"), IsDual: true, InstructorName: s("Legacy I")}, "Amelia Earhart", "Legacy I"},
		{"dual + stale Self + no instructor → keep Self", &models.Flight{PICName: s("Self"), IsDual: true}, "Amelia Earhart", "Self"},
		{"non-dual + Self + crew instructor → instructor (legacy flag mismatch)", &models.Flight{PICName: s("Self"), IsDual: false, CrewMembers: crewInstr("CFI Mueller")}, "Amelia Earhart", "CFI Mueller"},
		{"non-dual + Self + no instructor stays Self", &models.Flight{PICName: s("Self"), IsDual: false}, "Amelia Earhart", "Self"},
		// Exam flights: the examiner is PIC of record (GH #98).
		{"crew examiner only", &models.Flight{CrewMembers: crewExam("DPE Prüfer")}, "Amelia Earhart", "DPE Prüfer"},
		{"crew examiner is self → SELF", &models.Flight{CrewMembers: crewExam("Earhart, Amelia")}, "Amelia Earhart", "SELF"},
		{"dual + stale Self + crew examiner → examiner", &models.Flight{PICName: s("Self"), IsDual: true, CrewMembers: crewExam("DPE Prüfer")}, "Amelia Earhart", "DPE Prüfer"},
		{"instructor wins over examiner", &models.Flight{CrewMembers: append(crewExam("DPE Prüfer"), crewInstr("CFI Mueller")...)}, "Amelia Earhart", "CFI Mueller"},
	}
	for _, tc := range cases {
		if got := DisplayPICName(tc.f, tc.userName); got != tc.want {
			t.Errorf("%s: DisplayPICName = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestResolvePICNameForSave_CrewFallback(t *testing.T) {
	crewInstr := func(name string) []models.FlightCrewMember {
		return []models.FlightCrewMember{{Name: name, Role: models.CrewRoleInstructor}}
	}
	s := func(v string) *string { return &v }

	cases := []struct {
		name     string
		f        *models.Flight
		userName string
		want     *string
	}{
		{"explicit PICName wins", &models.Flight{PICName: s("CPT Doe"), IsDual: true, CrewMembers: crewInstr("CFI M")}, "Amelia Earhart", s("CPT Doe")},
		{"PIC + not dual → Self", &models.Flight{IsPIC: true}, "Amelia Earhart", s("Self")},
		{"Dual + legacy InstructorName", &models.Flight{IsDual: true, InstructorName: s("Legacy I")}, "Amelia Earhart", s("Legacy I")},
		{"Dual + crew instructor (third party)", &models.Flight{IsDual: true, CrewMembers: crewInstr("CFI Mueller")}, "Amelia Earhart", s("CFI Mueller")},
		{"Dual + crew instructor reversed", &models.Flight{IsDual: true, CrewMembers: crewInstr("Mueller, CFI")}, "Amelia Earhart", s("Mueller, CFI")},
		{"Dual + crew instructor is self → nil (rendered as SELF)", &models.Flight{IsDual: true, CrewMembers: crewInstr("Earhart, Amelia")}, "Amelia Earhart", nil},
		{"Dual + no instructor anywhere → nil", &models.Flight{IsDual: true}, "Amelia Earhart", nil},
		// Stale "Self" must be replaced by the real instructor when known,
		// regardless of IsDual (legacy data may have mismatched flags).
		{"Dual + stale Self + crew instructor → instructor", &models.Flight{PICName: s("Self"), IsDual: true, CrewMembers: crewInstr("CFI Mueller")}, "Amelia Earhart", s("CFI Mueller")},
		{"Dual + stale SELF + legacy instructor → instructor", &models.Flight{PICName: s("SELF"), IsDual: true, InstructorName: s("Legacy I")}, "Amelia Earhart", s("Legacy I")},
		{"Dual + stale Self + no instructor → preserve Self", &models.Flight{PICName: s("Self"), IsDual: true}, "Amelia Earhart", s("Self")},
		{"non-Dual + Self + crew instructor → instructor (legacy flag mismatch)", &models.Flight{PICName: s("Self"), IsDual: false, CrewMembers: crewInstr("CFI Mueller")}, "Amelia Earhart", s("CFI Mueller")},
		{"non-Dual + Self + no instructor stays Self", &models.Flight{PICName: s("Self"), IsPIC: true}, "Amelia Earhart", s("Self")},
		// Exam flights: the examiner is PIC of record (GH #98).
		{"Dual + crew examiner (third party)", &models.Flight{IsDual: true, CrewMembers: []models.FlightCrewMember{{Name: "DPE Prüfer", Role: models.CrewRoleExaminer}}}, "Amelia Earhart", s("DPE Prüfer")},
		{"Dual + stale Self + crew examiner → examiner", &models.Flight{PICName: s("Self"), IsDual: true, CrewMembers: []models.FlightCrewMember{{Name: "DPE Prüfer", Role: models.CrewRoleExaminer}}}, "Amelia Earhart", s("DPE Prüfer")},
	}
	for _, tc := range cases {
		got := ResolvePICNameForSave(tc.f, tc.userName)
		switch {
		case got == nil && tc.want == nil:
			// ok
		case got == nil || tc.want == nil:
			t.Errorf("%s: ResolvePICNameForSave = %v, want %v", tc.name, got, tc.want)
		case *got != *tc.want:
			t.Errorf("%s: ResolvePICNameForSave = %q, want %q", tc.name, *got, *tc.want)
		}
	}
}

func TestPilotingCategoryFor(t *testing.T) {
	cases := []struct {
		name    string
		f       *models.Flight
		acClass string
		want    PilotingCategory
	}{
		{"multi-pilot wins", &models.Flight{MultiPilotTime: 60, TotalTime: 60}, "SEP", CategoryMP},
		{"SEP → SP-SE", &models.Flight{TotalTime: 60}, "SEP", CategorySPSE},
		{"MEP → SP-ME", &models.Flight{TotalTime: 60}, "MEP", CategorySPME},
		{"SET → SP-ME", &models.Flight{TotalTime: 60}, "SET", CategorySPME},
		{"MET → SP-ME", &models.Flight{TotalTime: 60}, "MET", CategorySPME},
		{"unknown → SP-SE", &models.Flight{TotalTime: 60}, "", CategorySPSE},
	}
	for _, tc := range cases {
		if got := PilotingCategoryFor(tc.f, tc.acClass); got != tc.want {
			t.Errorf("%s: PilotingCategoryFor = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestEffectiveIFRTime(t *testing.T) {
	cases := []struct {
		name string
		f    *models.Flight
		want int
	}{
		{"nil", nil, 0},
		{"explicit", &models.Flight{IFRTime: 30}, 30},
		{"derived sum", &models.Flight{ActualInstrumentTime: 10, SimulatedInstrumentTime: 20, TotalTime: 60}, 30},
		{"capped at total", &models.Flight{ActualInstrumentTime: 100, SimulatedInstrumentTime: 100, TotalTime: 60}, 60},
		{"all zero", &models.Flight{TotalTime: 60}, 0},
		{"explicit beats derived", &models.Flight{IFRTime: 15, ActualInstrumentTime: 30, TotalTime: 60}, 15},
	}
	for _, tc := range cases {
		if got := EffectiveIFRTime(tc.f); got != tc.want {
			t.Errorf("%s: EffectiveIFRTime = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestCombinedRemarks(t *testing.T) {
	s := func(v string) *string { return &v }
	f := &models.Flight{
		Remarks:      s("R1"),
		Endorsements: s("E1"),
		IsIPC:        true,
	}
	got := CombinedRemarks(f, FlagIPC, FlagFlightReview)
	want := "R1 | E1 [IPC]"
	if got != want {
		t.Errorf("CombinedRemarks = %q, want %q", got, want)
	}

	if got := CombinedRemarks(nil); got != "" {
		t.Errorf("nil flight remarks = %q, want empty", got)
	}
}

func TestDetermineRole(t *testing.T) {
	f := &models.Flight{
		CrewMembers: []models.FlightCrewMember{
			{Name: "Earhart, Amelia", Role: models.CrewRoleInstructor},
		},
	}
	if got := DetermineRole(f, "Amelia Earhart"); got != RoleDualGiving {
		t.Errorf("self-instructor via reversed name = %v, want RoleDualGiving", got)
	}

	f2 := &models.Flight{
		CrewMembers: []models.FlightCrewMember{
			{Name: "CFI Mueller", Role: models.CrewRoleInstructor},
		},
	}
	if got := DetermineRole(f2, "Amelia Earhart"); got != RoleDualReceiving {
		t.Errorf("third-party instructor = %v, want RoleDualReceiving", got)
	}
}

// Regression: GH issue #98 — a check ride with a third-party Examiner on
// board must be Dual received, not PIC (NfL 2021-2-602 §4.2.2 no. 4: the
// examiner in a pilot seat is PIC of record, and there is only one PIC).
func TestDetermineRole_Examiner(t *testing.T) {
	f := &models.Flight{
		CrewMembers: []models.FlightCrewMember{
			{Name: "DPE Prüfer", Role: models.CrewRoleExaminer},
		},
	}
	if got := DetermineRole(f, "Amelia Earhart"); got != RoleDualReceiving {
		t.Errorf("third-party examiner = %v, want RoleDualReceiving", got)
	}

	// Empty userName: conservative fallback treats the examiner as third party.
	if got := DetermineRole(f, ""); got != RoleDualReceiving {
		t.Errorf("examiner with empty userName = %v, want RoleDualReceiving", got)
	}

	// The user acting as examiner logs PIC time themselves.
	self := &models.Flight{
		CrewMembers: []models.FlightCrewMember{
			{Name: "Earhart, Amelia", Role: models.CrewRoleExaminer},
		},
	}
	if got := DetermineRole(self, "Amelia Earhart"); got != RolePIC {
		t.Errorf("self-examiner = %v, want RolePIC", got)
	}
}

func TestInferLegacyCrew(t *testing.T) {
	// Training flight: Person1 is the named instructor.
	got := InferLegacyCrew(LegacyCrewInput{Person1: "CFI Mueller", HasDualReceived: true})
	if len(got) != 1 || got[0].Role != "Instructor" || got[0].Name != "CFI Mueller" {
		t.Errorf("training: %+v", got)
	}

	// Instructor-giving: Person1 is the student.
	got = InferLegacyCrew(LegacyCrewInput{Person1: "Stu Dent", HasDualGiven: true})
	if len(got) != 1 || got[0].Role != "Student" {
		t.Errorf("giving: %+v", got)
	}

	// Separate instructor + Person1 PIC.
	got = InferLegacyCrew(LegacyCrewInput{Person1: "Capt Doe", InstructorName: "CFI Mueller", HasDualReceived: true})
	if len(got) != 2 || got[0].Name != "CFI Mueller" || got[0].Role != "Instructor" || got[1].Name != "Capt Doe" || got[1].Role != "PIC" {
		t.Errorf("separate: %+v", got)
	}

	// Plain PIC + passengers.
	got = InferLegacyCrew(LegacyCrewInput{Person1: "Me", Person2: "Pax A", Person3: "Pax B"})
	if len(got) != 3 || got[0].Role != "PIC" || got[1].Role != "Passenger" || got[2].Role != "Passenger" {
		t.Errorf("plain: %+v", got)
	}
}

func TestInferLegacyCrew_ExplicitRoles(t *testing.T) {
	// ForeFlight scenario: the instructor is the SECOND person, the first
	// is the student. The role tags on the Person cells must win over the
	// positional "Person1 = Instructor on training flight" rule.
	got := InferLegacyCrew(LegacyCrewInput{
		Person1:         "Stu Dent",
		Person1Role:     "Student",
		Person2:         "CFI Mueller",
		Person2Role:     "Instructor",
		HasDualReceived: true,
	})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(got), got)
	}
	if got[0].Name != "Stu Dent" || got[0].Role != "Student" {
		t.Errorf("Person1: %+v, want Stu Dent/Student", got[0])
	}
	if got[1].Name != "CFI Mueller" || got[1].Role != "Instructor" {
		t.Errorf("Person2: %+v, want CFI Mueller/Instructor", got[1])
	}

	// Explicit Passenger tag on Person3 (the positional default), explicit
	// SIC on Person2, explicit PIC on Person1 — all roles must round-trip.
	got = InferLegacyCrew(LegacyCrewInput{
		Person1:     "Captain",
		Person1Role: "PIC",
		Person2:     "First Officer",
		Person2Role: "SIC",
		Person3:     "Jumpseater",
		Person3Role: "Observer",
	})
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %+v", len(got), got)
	}
	if got[0].Role != "PIC" || got[1].Role != "SIC" || got[2].Role != "Passenger" {
		t.Errorf("roles = %s/%s/%s, want PIC/SIC/Passenger", got[0].Role, got[1].Role, got[2].Role)
	}

	// Unknown role tag falls back to positional inference (Person1 on a
	// training flight → Instructor).
	got = InferLegacyCrew(LegacyCrewInput{
		Person1:         "CFI Mueller",
		Person1Role:     "WeirdTag",
		HasDualReceived: true,
	})
	if len(got) != 1 || got[0].Role != "Instructor" {
		t.Errorf("unknown tag fallback: %+v", got)
	}
}

func TestNormalizeLegacyRole(t *testing.T) {
	cases := map[string]string{
		"PIC":              "PIC",
		"pic":              "PIC",
		"  Instructor  ":   "Instructor",
		"CFI":              "Instructor",
		"student":          "Student",
		"SIC":              "SIC",
		"Passenger":        "Passenger",
		"FlightAttendant":  "Passenger",
		"Flight Attendant": "Passenger",
		"Observer":         "Passenger",
		"Engineer":         "Passenger",
		"Other":            "Passenger",
		"":                 "",
		"NotARole":         "",
	}
	for in, want := range cases {
		if got := NormalizeLegacyRole(in); got != want {
			t.Errorf("NormalizeLegacyRole(%q) = %q, want %q", in, got, want)
		}
	}
}
