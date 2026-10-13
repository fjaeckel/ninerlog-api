package currency

import "testing"

func TestLicenceClassifierWrappers(t *testing.T) {
	tests := []struct {
		licenseType                    string
		lapla, sailplane, gpl, ulTyped bool
	}{
		{"LAPL", true, false, false, false},
		{"LAPL(A)", true, false, false, false},
		{" lapl(a) ", true, false, false, false},
		{"SPL", false, true, false, false},
		{"LAPL(S)", false, true, false, false},
		{"spl", false, true, false, false},
		{"GPL", false, false, true, false},
		{" gpl", false, false, true, false},
		{"UL", false, false, false, true},
		{"ul", false, false, false, true},
		{"UL Dreiachs", false, false, false, true},
		{"UL-Lizenz", false, false, false, true},
		{"Ultralight", false, false, false, true},
		{"Luftfahrerschein Ultraleicht", false, false, false, true},
		{"ULM", false, false, false, false},
		{"PPL", false, false, false, false},
		{"LAPL(H)", false, false, false, false},
		{"Luftsportgeräteführer", false, false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.licenseType, func(t *testing.T) {
			if got := isEASALAPLA(tt.licenseType); got != tt.lapla {
				t.Errorf("isEASALAPLA(%q) = %v, want %v", tt.licenseType, got, tt.lapla)
			}
			if got := isEASASailplane(tt.licenseType); got != tt.sailplane {
				t.Errorf("isEASASailplane(%q) = %v, want %v", tt.licenseType, got, tt.sailplane)
			}
			if got := isGPL(tt.licenseType); got != tt.gpl {
				t.Errorf("isGPL(%q) = %v, want %v", tt.licenseType, got, tt.gpl)
			}
			if got := isULLicenceType(tt.licenseType); got != tt.ulTyped {
				t.Errorf("isULLicenceType(%q) = %v, want %v", tt.licenseType, got, tt.ulTyped)
			}
		})
	}
}
