package flightrules

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func TestCombinedRemarks_LaunchMethod(t *testing.T) {
	s := func(v string) *string { return &v }
	tests := []struct {
		name string
		f    *models.Flight
		want string
	}{
		{"L5 winch launch alone", &models.Flight{LaunchMethod: s("winch")}, "[Launch: winch]"},
		{"after remarks", &models.Flight{Remarks: s("Thermals"), LaunchMethod: s("aerotow")}, "Thermals [Launch: aerotow]"},
		{"after function times, before flags", &models.Flight{Remarks: s("R"), PICUSTime: 30, LaunchMethod: s("self-launch"), IsIPC: true}, "R [PICUS 0:30] [Launch: self-launch] [IPC]"},
		{"no launch method", &models.Flight{Remarks: s("R")}, "R"},
		{"empty launch method", &models.Flight{Remarks: s("R"), LaunchMethod: s("")}, "R"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CombinedRemarks(tt.f, FlagIPC); got != tt.want {
				t.Errorf("CombinedRemarks = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractLaunchRemark(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		wantMethod string
		wantRest   string
	}{
		{"marker only", "[Launch: winch]", "winch", ""},
		{"after remarks", "Thermals [Launch: aerotow]", "aerotow", "Thermals"},
		{"between annotations", "R [PICUS 0:30] [Launch: self-launch] [IPC]", "self-launch", "R [PICUS 0:30] [IPC]"},
		{"unknown method left alone", "R [Launch: catapult]", "", "R [Launch: catapult]"},
		{"no marker", "Local flight", "", "Local flight"},
		{"empty", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, rest := ExtractLaunchRemark(tt.in)
			if m != tt.wantMethod || rest != tt.wantRest {
				t.Errorf("ExtractLaunchRemark(%q) = (%q, %q), want (%q, %q)", tt.in, m, rest, tt.wantMethod, tt.wantRest)
			}
		})
	}
}

func TestLaunchRemarkRoundTrip(t *testing.T) {
	for _, m := range models.ValidLaunchMethods() {
		t.Run(m, func(t *testing.T) {
			got, _ := ExtractLaunchRemark("x " + LaunchRemark(&m))
			if got != m {
				t.Errorf("round trip of %q = %q", m, got)
			}
		})
	}
}

func TestSailplaneRemarks(t *testing.T) {
	s := func(v string) *string { return &v }
	h := func(v int) *int { return &v }
	tests := []struct {
		name string
		f    *models.Flight
		want string
	}{
		{"L5 launch method has its own column", &models.Flight{Remarks: s("Thermik"), LaunchMethod: s("winch")}, "Thermik"},
		{"outlanding marker", &models.Flight{Remarks: s("Feld bei Aalen"), IsOutlanding: true}, "Feld bei Aalen [Outlanding]"},
		{"release height", &models.Flight{LaunchMethod: s("aerotow"), ReleaseHeightM: h(600)}, "[Release 600 m]"},
		{"function time, outlanding, release", &models.Flight{SPICTime: 12, IsOutlanding: true, ReleaseHeightM: h(400)}, "[SPIC 0:12] [Outlanding] [Release 400 m]"},
		{"endorsement kept", &models.Flight{Endorsements: s("FI(S) Hans"), LaunchMethod: s("winch")}, "FI(S) Hans"},
		{"empty", &models.Flight{}, ""},
		{"nil", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SailplaneRemarks(tt.f); got != tt.want {
				t.Errorf("SailplaneRemarks = %q, want %q", got, tt.want)
			}
		})
	}
}
