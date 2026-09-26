package currency

import (
	"reflect"
	"sort"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func sortedClasses(cs []models.ClassType) []models.ClassType {
	out := append([]models.ClassType{}, cs...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func testScopeService() *Service {
	r := NewRegistry()
	r.Register(NewEASAEvaluator())
	r.Register(NewFAAEvaluator())
	r.Register(NewOtherEvaluator())
	ul := NewGermanULEvaluator()
	r.RegisterMulti(ul, ul.Authorities()...)
	return NewService(r, nil, nil, nil)
}

func TestCreditScope(t *testing.T) {
	svc := testScopeService()
	lic := func(authority, typ string) *models.License {
		return &models.License{RegulatoryAuthority: authority, LicenseType: typ}
	}
	rating := func(ct models.ClassType, kind *models.ULKind) *models.ClassRating {
		return &models.ClassRating{ClassType: ct, ULKind: kind}
	}
	ulSel := func(kinds ...models.ULKind) ULSelector { return ULSelector{Kinds: kinds} }

	cases := []struct {
		name    string
		license *models.License
		rating  *models.ClassRating
		peers   []*models.ClassRating
		want    CreditScope
	}{
		{
			name:    "M2 German UL three-axis counts SEP(land) and TMG (§45(2))",
			license: lic("DULV", "UL"),
			rating:  rating(models.ClassTypeUL, ulKindPtr(models.ULKindThreeAxis)),
			want:    CreditScope{Classes: []models.ClassType{models.ClassTypeSEPLand, models.ClassTypeTMG}},
		},
		{
			name:    "S German UL weight-shift counts no other class",
			license: lic("DULV", "UL"),
			rating:  rating(models.ClassTypeUL, ulKindPtr(models.ULKindWeightShift)),
			want:    CreditScope{},
		},
		{
			name:    "German UL sailplane counts towed launches",
			license: lic("DAeC", "UL"),
			rating:  rating(models.ClassTypeUL, ulKindPtr(models.ULKindSailplane)),
			want:    CreditScope{CountsTowed: true},
		},
		{
			name:    "German UL gyroplane counts GYROPLANE",
			license: lic("DULV", "UL"),
			rating:  rating(models.ClassTypeUL, ulKindPtr(models.ULKindGyroplane)),
			want:    CreditScope{Classes: []models.ClassType{models.ClassTypeGyro}},
		},
		{
			name:    "PPL SEP(land) credits three-axis UL (FCL.035(a)(4))",
			license: lic("EASA", "PPL"),
			rating:  rating(models.ClassTypeSEPLand, nil),
			want: CreditScope{
				Classes: []models.ClassType{models.ClassTypeSEPLand},
				ULs:     []ULSelector{ulSel(models.ULKindThreeAxis)},
			},
		},
		{
			name:    "PPL SEP(land) with TMG pools both (FCL.740.A(b)(1))",
			license: lic("EASA", "PPL"),
			rating:  rating(models.ClassTypeSEPLand, nil),
			peers:   []*models.ClassRating{rating(models.ClassTypeSEPLand, nil), rating(models.ClassTypeTMG, nil)},
			want: CreditScope{
				Classes: []models.ClassType{models.ClassTypeSEPLand, models.ClassTypeTMG},
				ULs:     []ULSelector{ulSel(models.ULKindThreeAxis, models.ULKindThreeAxisMotorglider)},
			},
		},
		{
			name:    "LAPL(A) pools every aeroplane class and TMG (FCL.140.A)",
			license: lic("EASA", "LAPL(A)"),
			rating:  rating(models.ClassTypeSEPLand, nil),
			want: CreditScope{
				Classes: easaAeroplaneClasses,
				ULs:     []ULSelector{ulSel(models.ULKindThreeAxis, models.ULKindThreeAxisMotorglider)},
			},
		},
		{
			name:    "SPL glider counts TMG and UL sailplanes (SFCL.160(a))",
			license: lic("EASA", "SPL"),
			rating:  rating(models.ClassTypeGlider, nil),
			want: CreditScope{
				Classes:     []models.ClassType{models.ClassTypeGlider, models.ClassTypeTMG},
				ULs:         []ULSelector{ulSel(models.ULKindSailplane, models.ULKindThreeAxisMotorglider)},
				CountsTowed: true,
			},
		},
		{
			name:    "SPL TMG counts sailplanes (SFCL.160(b))",
			license: lic("EASA", "SPL"),
			rating:  rating(models.ClassTypeTMG, nil),
			want: CreditScope{
				Classes: []models.ClassType{models.ClassTypeTMG, models.ClassTypeGlider},
				ULs:     []ULSelector{ulSel(models.ULKindSailplane, models.ULKindThreeAxisMotorglider)},
			},
		},
		{
			name:    "GPL credits UL gyroplanes of at least 450 kg (FCL.035(a)(5))",
			license: lic("EASA", "GPL"),
			rating:  rating(models.ClassTypeGyro, nil),
			want: CreditScope{
				Classes: []models.ClassType{models.ClassTypeGyro},
				ULs:     []ULSelector{{Kinds: []models.ULKind{models.ULKindGyroplane}, MinMTOMKg: 450}},
			},
		},
		{
			name:    "IR is cross-class and lists no classes",
			license: lic("EASA", "PPL"),
			rating:  rating(models.ClassTypeIR, nil),
			want:    CreditScope{},
		},
		{
			name:    "unknown authority falls back to the rating's class",
			license: lic("CAA", "PPL"),
			rating:  rating(models.ClassTypeSEPLand, nil),
			want:    CreditScope{Classes: []models.ClassType{models.ClassTypeSEPLand}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.CreditScope(tc.license, tc.rating, tc.peers)
			if !reflect.DeepEqual(sortedClasses(tc.want.Classes), sortedClasses(got.Classes)) {
				t.Errorf("Classes = %v, want %v", got.Classes, tc.want.Classes)
			}
			if !reflect.DeepEqual(tc.want.ULs, got.ULs) {
				t.Errorf("ULs = %+v, want %+v", got.ULs, tc.want.ULs)
			}
			if got.CountsTowed != tc.want.CountsTowed {
				t.Errorf("CountsTowed = %v, want %v", got.CountsTowed, tc.want.CountsTowed)
			}
		})
	}
}

func TestCreditScopeCovers(t *testing.T) {
	str := func(s string) *string { return &s }
	mtom := func(kg int) *int { return &kg }
	scope := CreditScope{
		Classes: []models.ClassType{models.ClassTypeSEPLand},
		ULs: []ULSelector{
			{Kinds: []models.ULKind{models.ULKindThreeAxis}},
			{Kinds: []models.ULKind{models.ULKindGyroplane}, MinMTOMKg: 450},
		},
	}
	cases := []struct {
		name string
		ac   *models.Aircraft
		want bool
	}{
		{"class matched case-insensitively", &models.Aircraft{AircraftClass: str(" sep_land ")}, true},
		{"other class", &models.Aircraft{AircraftClass: str("MEP_LAND")}, false},
		{"no class", &models.Aircraft{}, false},
		{"nil aircraft", nil, false},
		{"UL of a selected kind", &models.Aircraft{AircraftClass: str("ultralight"), ULKind: ulKindPtr(models.ULKindThreeAxis)}, true},
		{"UL of another kind", &models.Aircraft{AircraftClass: str("ULTRALIGHT"), ULKind: ulKindPtr(models.ULKindWeightShift)}, false},
		{"UL of no kind", &models.Aircraft{AircraftClass: str("ULTRALIGHT")}, false},
		{"UL gyroplane at the MTOM floor", &models.Aircraft{AircraftClass: str("ULTRALIGHT"), ULKind: ulKindPtr(models.ULKindGyroplane), MTOMKg: mtom(450)}, true},
		{"UL gyroplane below the MTOM floor", &models.Aircraft{AircraftClass: str("ULTRALIGHT"), ULKind: ulKindPtr(models.ULKindGyroplane), MTOMKg: mtom(449)}, false},
		{"UL gyroplane without MTOM", &models.Aircraft{AircraftClass: str("ULTRALIGHT"), ULKind: ulKindPtr(models.ULKindGyroplane)}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scope.Covers(tc.ac); got != tc.want {
				t.Errorf("Covers = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("ULTRALIGHT in Classes does not cover ultralights", func(t *testing.T) {
		s := CreditScope{Classes: []models.ClassType{models.ClassTypeUL}}
		if s.Covers(&models.Aircraft{AircraftClass: str("ULTRALIGHT")}) {
			t.Error("Covers = true, want false")
		}
	})
}
