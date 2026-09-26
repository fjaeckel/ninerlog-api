package models

import "testing"

func strPtr(s string) *string { return &s }

func TestAircraftNormalizeULKind(t *testing.T) {
	three := ULKindThreeAxis
	bogus := ULKind("HOVERCRAFT")
	tests := []struct {
		name    string
		class   *string
		kind    *ULKind
		want    *ULKind
		wantErr bool
	}{
		{"UL keeps kind", strPtr("ULTRALIGHT"), &three, &three, false},
		{"UL class case-insensitive", strPtr(" ultralight "), &three, &three, false},
		{"UL without kind", strPtr("ULTRALIGHT"), nil, nil, false},
		{"non-UL clears kind", strPtr("SEP_LAND"), &three, nil, false},
		{"no class clears kind", nil, &three, nil, false},
		{"unknown kind", strPtr("ULTRALIGHT"), &bogus, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Aircraft{AircraftClass: tt.class, ULKind: tt.kind}
			err := a.NormalizeULKind()
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if (a.ULKind == nil) != (tt.want == nil) || (a.ULKind != nil && *a.ULKind != *tt.want) {
				t.Errorf("ULKind = %v, want %v", a.ULKind, tt.want)
			}
		})
	}
}

func TestClassRatingNormalizeULKind(t *testing.T) {
	gyro := ULKindGyroplane
	motorglider := ULKindThreeAxisMotorglider

	cr := &ClassRating{ClassType: ClassTypeUL, ULKind: &gyro}
	if err := cr.NormalizeULKind(); err != nil || cr.ULKind == nil {
		t.Errorf("UL rating: err = %v, kind = %v", err, cr.ULKind)
	}
	cr = &ClassRating{ClassType: ClassTypeSEPLand, ULKind: &gyro}
	if err := cr.NormalizeULKind(); err != nil || cr.ULKind != nil {
		t.Errorf("SEP rating: err = %v, kind = %v, want cleared", err, cr.ULKind)
	}
	cr = &ClassRating{ClassType: ClassTypeUL, ULKind: &motorglider}
	if err := cr.NormalizeULKind(); err != ErrInvalidULKind {
		t.Errorf("motorglider rating: err = %v, want ErrInvalidULKind", err)
	}
}

func TestULKindMappings(t *testing.T) {
	if ct, ok := ULKindThreeAxis.PartFCLClass(); !ok || ct != ClassTypeSEPLand {
		t.Errorf("THREE_AXIS → %v %v, want SEP_LAND", ct, ok)
	}
	if ct, ok := ULKindThreeAxisMotorglider.PartFCLClass(); !ok || ct != ClassTypeTMG {
		t.Errorf("THREE_AXIS_MOTORGLIDER → %v %v, want TMG", ct, ok)
	}
	for _, k := range []ULKind{ULKindWeightShift, ULKindGyroplane, ULKindHelicopter, ULKindPoweredParaglider, ULKindSailplane} {
		if _, ok := k.PartFCLClass(); ok {
			t.Errorf("%s should not be credited under FCL.035(a)(4)", k)
		}
	}
	if got := AircraftKindsForRating(ULKindThreeAxis); len(got) != 2 {
		t.Errorf("THREE_AXIS rating covers %v, want two kinds", got)
	}
	if got := AircraftKindsForRating(ULKindGyroplane); len(got) != 1 || got[0] != ULKindGyroplane {
		t.Errorf("GYROPLANE rating covers %v", got)
	}
}

func TestAircraftValidateMTOM(t *testing.T) {
	for _, tt := range []struct {
		mtom    *int
		wantErr bool
	}{
		{nil, false}, {intPtr(472), false}, {intPtr(0), true}, {intPtr(-5), true}, {intPtr(1000001), true},
	} {
		a := &Aircraft{Registration: "D-MGYR", Type: "MTO", Make: "AutoGyro", Model: "MTOsport", MTOMKg: tt.mtom}
		if err := a.Validate(); (err == ErrInvalidAircraftMTOM) != tt.wantErr {
			t.Errorf("MTOM %v: err = %v, wantErr %v", tt.mtom, err, tt.wantErr)
		}
	}
}

func intPtr(i int) *int { return &i }
