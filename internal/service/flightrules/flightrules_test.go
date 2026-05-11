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

	cases := []struct {
		name string
		f    *models.Flight
		want string
	}{
		{"nil", nil, "SELF"},
		{"empty", &models.Flight{}, "SELF"},
		{"pic set", &models.Flight{PICName: s("CPT Doe")}, "CPT Doe"},
		{"instructor only", &models.Flight{InstructorName: s("CFI Mueller")}, "CFI Mueller"},
		{"pic blank, instructor set", &models.Flight{PICName: s("  "), InstructorName: s("CFI Mueller")}, "CFI Mueller"},
		{"both set, pic wins", &models.Flight{PICName: s("CPT Doe"), InstructorName: s("CFI Mueller")}, "CPT Doe"},
	}
	for _, tc := range cases {
		if got := DisplayPICName(tc.f); got != tc.want {
			t.Errorf("%s: DisplayPICName = %q, want %q", tc.name, got, tc.want)
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
