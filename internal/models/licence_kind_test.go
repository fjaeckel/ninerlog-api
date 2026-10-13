package models

import "testing"

func TestClassifyLicence(t *testing.T) {
	tests := []struct {
		name        string
		licenceType string
		authority   string
		want        LicenceKind
	}{
		{"LAPL", "LAPL", "EASA", LicenceKindLAPLA},
		{"LAPL(A)", "LAPL(A)", "EASA", LicenceKindLAPLA},
		{"lapl(a) lower case", "lapl(a)", "easa", LicenceKindLAPLA},
		{"padded LAPL(A)", " LAPL(A) ", "EASA", LicenceKindLAPLA},
		{"Lapl mixed case", "Lapl", "", LicenceKindLAPLA},
		{"SPL", "SPL", "EASA", LicenceKindSPL},
		{"spl lower case", "spl", "LBA", LicenceKindSPL},
		{"LAPL(S)", "LAPL(S)", "EASA", LicenceKindLAPLS},
		{"Lapl(s) mixed case", "Lapl(s)", "", LicenceKindLAPLS},
		{"GPL", "GPL", "EASA", LicenceKindGPL},
		{"gpl padded", " gpl ", "", LicenceKindGPL},
		{"UL", "UL", "LBA", LicenceKindUL},
		{"UL with suffix", "UL Dreiachs", "", LicenceKindUL},
		{"UL- prefix", "UL-Lizenz", "", LicenceKindUL},
		{"Ultralight", "Ultralight Pilot", "", LicenceKindUL},
		{"Ultraleicht", "Luftfahrerschein Ultraleicht", "", LicenceKindUL},
		{"ul lower case", "ul", "", LicenceKindUL},
		{"DULV any type", "Luftsportgeräteführer", "DULV", LicenceKindUL},
		{"DULV Sportpilotenlizenz", "SPL", "DULV", LicenceKindUL},
		{"DAeC any type", "PPL", "DAeC", LicenceKindUL},
		{"ULM is not UL", "ULM", "", LicenceKindUnknown},
		{"PPL", "PPL", "EASA", LicenceKindPPLA},
		{"PPL(A)", "PPL(A)", "LBA", LicenceKindPPLA},
		{"CPL", "CPL", "EASA", LicenceKindCPLA},
		{"CPL(A)", "cpl(a)", "EASA", LicenceKindCPLA},
		{"ATPL", "ATPL", "EASA", LicenceKindATPLA},
		{"ATPL(A)", "ATPL(A)", "EASA", LicenceKindATPLA},
		{"MPL", "MPL", "EASA", LicenceKindMPL},
		{"PPL(H)", "PPL(H)", "EASA", LicenceKindHelicopter},
		{"LAPL(H)", "LAPL(H)", "EASA", LicenceKindHelicopter},
		{"ATPL(H)", "atpl(h)", "EASA", LicenceKindHelicopter},
		{"FAA Sport", "Sport", "FAA", LicenceKindFAASport},
		{"FAA Recreational", "Recreational", "FAA", LicenceKindFAARecreational},
		{"FAA Private", "Private", "FAA", LicenceKindFAAPrivate},
		{"FAA Commercial", "Commercial", "FAA", LicenceKindFAACommercial},
		{"FAA ATP", "ATP", "FAA", LicenceKindFAAATP},
		{"FAA Glider", "Glider", "FAA", LicenceKindFAAGlider},
		{"IR", "IR", "EASA", LicenceKindIR},
		{"FI(S)", "FI(S)", "EASA", LicenceKindInstructor},
		{"FI", "FI", "EASA", LicenceKindInstructor},
		{"CRI(A)", "CRI(A)", "EASA", LicenceKindInstructor},
		{"FI(H) is instructor", "FI(H)", "EASA", LicenceKindInstructor},
		{"FE", "FE", "EASA", LicenceKindInstructor},
		{"Examiner", "Examiner", "EASA", LicenceKindInstructor},
		{"CFI", "CFI", "FAA", LicenceKindInstructor},
		{"FIX is unknown", "FIX", "EASA", LicenceKindUnknown},
		{"empty", "", "", LicenceKindUnknown},
		{"free text", "Segelflugschein alt", "LBA", LicenceKindUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyLicence(tt.licenceType, tt.authority); got != tt.want {
				t.Errorf("ClassifyLicence(%q, %q) = %q, want %q", tt.licenceType, tt.authority, got, tt.want)
			}
		})
	}
}

func TestLicenceKindPredicates(t *testing.T) {
	tests := []struct {
		kind                                           LicenceKind
		easaSailplane, sailplane, aeroplane, multiCrew bool
	}{
		{LicenceKindSPL, true, true, false, false},
		{LicenceKindLAPLS, true, true, false, false},
		{LicenceKindFAAGlider, false, true, false, false},
		{LicenceKindPPLA, false, false, true, false},
		{LicenceKindLAPLA, false, false, true, false},
		{LicenceKindCPLA, false, false, true, false},
		{LicenceKindATPLA, false, false, true, true},
		{LicenceKindMPL, false, false, true, true},
		{LicenceKindFAASport, false, false, true, false},
		{LicenceKindFAAATP, false, false, true, false},
		{LicenceKindUL, false, false, false, false},
		{LicenceKindHelicopter, false, false, false, false},
		{LicenceKindUnknown, false, false, false, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			got := []bool{tt.kind.IsEASASailplane(), tt.kind.IsSailplane(), tt.kind.IsAeroplane(), tt.kind.IsMultiCrew()}
			want := []bool{tt.easaSailplane, tt.sailplane, tt.aeroplane, tt.multiCrew}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("predicate %d of %q = %v, want %v", i, tt.kind, got[i], want[i])
				}
			}
		})
	}
}
