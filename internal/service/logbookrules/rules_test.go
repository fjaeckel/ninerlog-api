package logbookrules

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRuleFor(t *testing.T) {
	tests := []struct {
		name        string
		licence     Licence
		requirement Requirement
		window      Window
		basis       string
		downward    bool
	}{
		{"no country never triggers", Licence{RegulatoryAuthority: "EASA", LicenseType: "PPL"}, RequirementNone, "", "", false},
		{"blank country never triggers", Licence{IssuingCountry: "  ", LicenseType: "PPL"}, RequirementNone, "", "", false},
		{"CH any type", Licence{IssuingCountry: "CH", RegulatoryAuthority: "EASA", LicenseType: "PPL"}, RequirementRequired, WindowHours48, "FOCA Accepted Logbook Formats §2.3(7)", true},
		{"CH lower case", Licence{IssuingCountry: "ch", LicenseType: "SPL"}, RequirementRequired, WindowHours48, "FOCA Accepted Logbook Formats §2.3(7)", true},
		{"DE PPL", Licence{IssuingCountry: "DE", RegulatoryAuthority: "EASA", LicenseType: "PPL"}, RequirementRequired, WindowFlightDay, "NfL 2212-21 §4.3.2", false},
		{"DE UL", Licence{IssuingCountry: "DE", LicenseType: "UL"}, RequirementSuggested, "", "§120 LuftPersV", false},
		{"DE ultraleicht", Licence{IssuingCountry: "DE", LicenseType: "Ultraleicht-Pilot"}, RequirementSuggested, "", "§120 LuftPersV", false},
		{"AT national UL", Licence{IssuingCountry: "AT", LicenseType: "UL-Schein"}, RequirementRequired, WindowFlightDay, "ZLPV 2006 Anlage 6", false},
		{"AT hang glider", Licence{IssuingCountry: "AT", LicenseType: "Hang Glider"}, RequirementRequired, WindowFlightDay, "ZLPV 2006 Anlage 6", false},
		{"AT paraglider German", Licence{IssuingCountry: "AT", LicenseType: "Paragleiter"}, RequirementRequired, WindowFlightDay, "ZLPV 2006 Anlage 6", false},
		{"AT national glider", Licence{IssuingCountry: "AT", LicenseType: "national glider"}, RequirementRequired, WindowFlightDay, "ZLPV 2006 Anlage 6", false},
		{"AT Part-FCL", Licence{IssuingCountry: "AT", LicenseType: "PPL"}, RequirementSuggested, "", "AMC1 FCL.050 (e)(2)", false},
		{"AU", Licence{IssuingCountry: "AU", LicenseType: "PPL"}, RequirementRequired, WindowFlightDay, "CASR 61.355, RAAP 13", false},
		{"NZ", Licence{IssuingCountry: "NZ", LicenseType: "PPL"}, RequirementRequired, WindowFlightDay, "Part 61.29(e)", false},
		{"JP", Licence{IssuingCountry: "JP", LicenseType: "PPL"}, RequirementRequired, WindowFlightDay, "国空乗390号", false},
		{"UY", Licence{IssuingCountry: "UY", LicenseType: "PPL"}, RequirementRequired, WindowFlightDay, "RAU 61.51", false},
		{"DK suggested", Licence{IssuingCountry: "DK", LicenseType: "PPL"}, RequirementSuggested, "", "BL 6-04 §3.2.6", false},
		{"BR suggested", Licence{IssuingCountry: "BR", LicenseType: "PPL"}, RequirementSuggested, "", "CIV Digital", false},
		{"GB suggested", Licence{IssuingCountry: "GB", LicenseType: "PPL"}, RequirementSuggested, "", "printed extracts", false},
		{"US suggested", Licence{IssuingCountry: "US", LicenseType: "PPL"}, RequirementSuggested, "", "AC 120-78B", false},
		{"other country under EASA", Licence{IssuingCountry: "FR", RegulatoryAuthority: "easa", LicenseType: "PPL"}, RequirementSuggested, "", "AMC1 FCL.050 (e)(2)", false},
		{"other country under FAA", Licence{IssuingCountry: "CA", RegulatoryAuthority: "FAA", LicenseType: "PPL"}, RequirementNone, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RuleFor(tt.licence)
			if got.Requirement != tt.requirement || got.Window != tt.window ||
				got.Basis != tt.basis || got.DownwardOnlyOverrides != tt.downward {
				t.Errorf("RuleFor() = %+v", got)
			}
		})
	}
}

func TestLooksLikeUL(t *testing.T) {
	tests := map[string]bool{
		"UL": true, " ul ": true, "UL DE": true, "UL-Schein": true, "Ultralight": true,
		"Ultraleicht": true, "PPL": false, "ULX": false, "": false,
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			if got := LooksLikeUL(in); got != want {
				t.Errorf("LooksLikeUL(%q) = %v, want %v", in, got, want)
			}
		})
	}
}

func TestAssess(t *testing.T) {
	de := Licence{ID: uuid.New(), IssuingCountry: "DE", LicenseType: "PPL"}
	ch := Licence{ID: uuid.New(), IssuingCountry: "CH", LicenseType: "PPL"}
	us := Licence{ID: uuid.New(), IssuingCountry: "US", LicenseType: "PPL"}
	bare := Licence{ID: uuid.New(), LicenseType: "PPL"}

	tests := []struct {
		name      string
		licences  []Licence
		required  int
		suggested int
		window    Window
		downward  bool
	}{
		{"nothing required", []Licence{us, bare}, 0, 1, "", false},
		{"single required licence", []Licence{de, us}, 1, 1, WindowFlightDay, false},
		{"CH makes overrides downward only", []Licence{ch}, 1, 0, WindowHours48, true},
		{"shortest window wins", []Licence{ch, de}, 2, 0, WindowFlightDay, true},
		{"empty", nil, 0, 0, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Assess(tt.licences)
			if len(a.Required) != tt.required || len(a.Suggested) != tt.suggested ||
				a.Window != tt.window || a.OverridesDownwardOnly != tt.downward {
				t.Errorf("Assess() = %+v", a)
			}
		})
	}
}

func TestFlightDayEnd(t *testing.T) {
	day := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		lon  float64
		want time.Time
	}{
		{"greenwich", 0, time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		{"zurich rounds to +1", 8.55, time.Date(2026, 3, 31, 23, 0, 0, 0, time.UTC)},
		{"sydney rounds to +10", 151.2, time.Date(2026, 3, 31, 14, 0, 0, 0, time.UTC)},
		{"new york rounds to -5", -74, time.Date(2026, 4, 1, 5, 0, 0, 0, time.UTC)},
		{"half hour rounds away from zero", 7.5, time.Date(2026, 3, 31, 23, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FlightDayEnd(day, tt.lon); !tt.want.Equal(got) {
				t.Errorf("FlightDayEnd() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestAssessmentDeadline(t *testing.T) {
	created := time.Date(2026, 5, 10, 18, 30, 0, 0, time.UTC)
	flight := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	ch := Licence{IssuingCountry: "CH", LicenseType: "PPL"}
	de := Licence{IssuingCountry: "DE", LicenseType: "PPL"}

	tests := []struct {
		name     string
		licences []Licence
		want     time.Time
	}{
		{"manual strict uses flight day", nil, time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)},
		{"CH is 48h after creation", []Licence{ch}, created.Add(48 * time.Hour)},
		{"DE is end of flight day", []Licence{de}, time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)},
		{"earliest of both", []Licence{ch, de}, time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Assess(tt.licences).Deadline(created, flight, 0); !tt.want.Equal(got) {
				t.Errorf("Deadline() = %s, want %s", got, tt.want)
			}
		})
	}

	t.Run("late-created CH entry still closes before flight-day DE", func(t *testing.T) {
		late := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
		got := Assess([]Licence{ch, de}).Deadline(late, flight, 0)
		if want := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC); !want.Equal(got) {
			t.Errorf("Deadline() = %s, want %s", got, want)
		}
	})
}
