package models

import (
	"testing"
)

func TestClassFromRegistration(t *testing.T) {
	tests := []struct {
		name   string
		reg    string
		want   ClassType
		wantOK bool
	}{
		{"L2 Lena's ASK 21 D-1234 is a glider", "D-1234", ClassTypeGlider, true},
		{"glider without hyphen", "d1234", ClassTypeGlider, true},
		{"glider with space", "D 5678", ClassTypeGlider, true},
		{"M3 Mehmet's C42 D-MXYZ is an ultralight", "D-MXYZ", ClassTypeUL, true},
		{"ultralight lower case without hyphen", "dmtrk", ClassTypeUL, true},
		{"D-K is ambiguous SLG or TMG", "D-KFSV", "", false},
		{"D-E single engine is not guessed", "D-EABC", "", false},
		{"five digits is not a German registration", "D-12345", "", false},
		{"three digits is not a German glider", "D-123", "", false},
		{"Swiss glider-like mark is not German", "HB-1234", "", false},
		{"US registration", "N12345", "", false},
		{"Angola D2 is not Germany", "D2-ABC", "", false},
		{"empty", "", "", false},
		{"simulator placeholder", "SIM", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ClassFromRegistration(tt.reg)
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("ClassFromRegistration(%q) = (%q, %v), want (%q, %v)", tt.reg, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestInferImportedAircraftClass(t *testing.T) {
	tests := []struct {
		name        string
		sourceClass ClassType
		reg         string
		towed       bool
		want        *string
	}{
		{"source class wins over registration", ClassTypeTMG, "D-1234", false, strp("TMG")},
		{"source class wins over towed launch", ClassTypeSEPLand, "D-EABC", true, strp("SEP_LAND")},
		{"registration glider", "", "D-1234", false, strp("GLIDER")},
		{"registration ultralight", "", "D-MXYZ", false, strp("ULTRALIGHT")},
		{"registration ultralight is not overridden by a tow", "", "D-MXYZ", true, strp("ULTRALIGHT")},
		{"D-K with towed launch is a glider", "", "D-KFSV", true, strp("GLIDER")},
		{"D-K without launch stays unset", "", "D-KFSV", false, nil},
		{"foreign glider with towed launch", "", "HB-1234", true, strp("GLIDER")},
		{"powered aircraft without launch stays unset", "", "D-EABC", false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InferImportedAircraftClass(tt.sourceClass, tt.reg, tt.towed)
			if derefOr(got, "<nil>") != derefOr(tt.want, "<nil>") {
				t.Errorf("InferImportedAircraftClass = %s, want %s", derefOr(got, "<nil>"), derefOr(tt.want, "<nil>"))
			}
		})
	}
}
