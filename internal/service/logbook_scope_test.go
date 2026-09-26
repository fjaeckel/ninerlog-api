package service_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/google/uuid"
)

type logbookFixture struct {
	scope    *service.LogbookScope
	licenses *mockCRLicenseRepo
	ratings  *mockClassRatingRepo
	aircraft *mockAircraftRepo
	userID   uuid.UUID
}

func newLogbookFixture() *logbookFixture {
	registry := currency.NewRegistry()
	registry.Register(currency.NewEASAEvaluator())
	registry.Register(currency.NewFAAEvaluator())
	registry.Register(currency.NewOtherEvaluator())
	ul := currency.NewGermanULEvaluator()
	registry.RegisterMulti(ul, ul.Authorities()...)

	f := &logbookFixture{
		licenses: newMockCRLicenseRepo(),
		ratings:  newMockClassRatingRepo(),
		aircraft: newMockAircraftRepo(),
		userID:   uuid.New(),
	}
	f.scope = service.NewLogbookScope(
		service.NewLicenseService(f.licenses),
		service.NewClassRatingService(f.ratings, f.licenses),
		service.NewAircraftService(f.aircraft),
		currency.NewService(registry, nil, nil, nil),
	)
	return f
}

func (f *logbookFixture) license(authority, typ string, ratings ...*models.ClassRating) uuid.UUID {
	lic := &models.License{UserID: f.userID, RegulatoryAuthority: authority, LicenseType: typ}
	_ = f.licenses.Create(context.Background(), lic)
	for _, cr := range ratings {
		cr.LicenseID = lic.ID
		_ = f.ratings.Create(context.Background(), cr)
	}
	return lic.ID
}

func (f *logbookFixture) addAircraft(reg, class string, kind *models.ULKind) {
	ac := &models.Aircraft{UserID: f.userID, Registration: reg, Type: "T", Make: "M", Model: "M", ULKind: kind}
	if class != "" {
		ac.AircraftClass = &class
	}
	_ = f.aircraft.Create(context.Background(), ac)
}

func kindPtr(k models.ULKind) *models.ULKind { return &k }

func rated(ct models.ClassType, kind *models.ULKind) *models.ClassRating {
	return &models.ClassRating{ClassType: ct, ULKind: kind}
}

func flightOn(reg string, launch string) *models.Flight {
	f := &models.Flight{ID: uuid.New(), AircraftReg: reg}
	if launch != "" {
		f.LaunchMethod = &launch
	}
	return f
}

// seedPersonaFleet registers Mehmet's and Sabine's aircraft plus a glider and a TMG.
func (f *logbookFixture) seedPersonaFleet() {
	f.addAircraft("D-MXYZ", "ULTRALIGHT", kindPtr(models.ULKindThreeAxis))
	f.addAircraft("D-MTRK", "ultralight", kindPtr(models.ULKindWeightShift))
	f.addAircraft("D-MPPG", "ULTRALIGHT", kindPtr(models.ULKindPoweredParaglider))
	f.addAircraft("D-MNOK", "ULTRALIGHT", nil)
	f.addAircraft("D-EABC", " sep_land ", nil)
	f.addAircraft("D-KTMG", "TMG", nil)
	f.addAircraft("D-1234", "GLIDER", nil)
	f.addAircraft("D-GMEP", "MEP_LAND", nil)
	f.addAircraft("D-NONE", "", nil)
}

func TestLogbookScopeClassify(t *testing.T) {
	type flightCase struct {
		reg, launch string
		want        service.LogbookMembership
	}
	cases := []struct {
		name      string
		authority string
		licType   string
		ratings   func() []*models.ClassRating
		flights   []flightCase
	}{
		{
			name:      "M2 German UL three-axis: C42 native, SEP and TMG credited, towed excluded",
			authority: "DULV", licType: "UL",
			ratings: func() []*models.ClassRating {
				return []*models.ClassRating{rated(models.ClassTypeUL, kindPtr(models.ULKindThreeAxis))}
			},
			flights: []flightCase{
				{"D-MXYZ", "", service.LogbookNative},
				{"d-mxyz", "", service.LogbookNative},
				{"D-EABC", "", service.LogbookCredited},
				{"D-KTMG", "self-launch", service.LogbookCredited},
				{"D-KTMG", "winch", service.LogbookExcluded},
				{"D-EABC", "aerotow", service.LogbookExcluded},
				{"D-1234", "winch", service.LogbookExcluded},
				{"D-MTRK", "", service.LogbookExcluded},
				{"D-MNOK", "", service.LogbookExcluded},
				{"D-GMEP", "", service.LogbookExcluded},
				{"D-UNKNOWN", "", service.LogbookExcluded},
			},
		},
		{
			name:      "S1 Sabine trike and powered paraglider: three-axis and kindless UL excluded",
			authority: "DULV", licType: "UL",
			ratings: func() []*models.ClassRating {
				return []*models.ClassRating{
					rated(models.ClassTypeUL, kindPtr(models.ULKindWeightShift)),
					rated(models.ClassTypeUL, kindPtr(models.ULKindPoweredParaglider)),
				}
			},
			flights: []flightCase{
				{"D-MTRK", "", service.LogbookNative},
				{"D-MPPG", "", service.LogbookNative},
				{"D-MXYZ", "", service.LogbookExcluded},
				{"D-MNOK", "", service.LogbookExcluded},
				{"D-EABC", "", service.LogbookExcluded},
			},
		},
		{
			name:      "UL rating without kind keeps every ultralight",
			authority: "DULV", licType: "UL",
			ratings: func() []*models.ClassRating {
				return []*models.ClassRating{rated(models.ClassTypeUL, nil)}
			},
			flights: []flightCase{
				{"D-MXYZ", "", service.LogbookNative},
				{"D-MTRK", "", service.LogbookNative},
				{"D-MNOK", "", service.LogbookNative},
			},
		},
		{
			name:      "M2 PPL SEP(land): class matched case-insensitively, three-axis UL credited",
			authority: "EASA", licType: "PPL",
			ratings: func() []*models.ClassRating {
				return []*models.ClassRating{rated(models.ClassTypeSEPLand, nil)}
			},
			flights: []flightCase{
				{"D-EABC", "", service.LogbookNative},
				{"D-MXYZ", "", service.LogbookCredited},
				{"D-MXYZ", "winch", service.LogbookExcluded},
				{"D-MTRK", "", service.LogbookExcluded},
				{"D-KTMG", "", service.LogbookExcluded},
				{"D-GMEP", "", service.LogbookExcluded},
				{"D-NONE", "", service.LogbookExcluded},
			},
		},
		{
			name:      "N2 guard: towed glider launch never credited to LAPL(A)",
			authority: "EASA", licType: "LAPL(A)",
			ratings: func() []*models.ClassRating {
				return []*models.ClassRating{rated(models.ClassTypeSEPLand, nil)}
			},
			flights: []flightCase{
				{"D-EABC", "", service.LogbookNative},
				{"D-GMEP", "", service.LogbookCredited},
				{"D-KTMG", "", service.LogbookCredited},
				{"D-KTMG", "aerotow", service.LogbookExcluded},
				{"D-1234", "winch", service.LogbookExcluded},
				{"D-1234", "", service.LogbookExcluded},
			},
		},
		{
			name:      "SPL glider: TMG credited, towed glider native",
			authority: "EASA", licType: "SPL",
			ratings: func() []*models.ClassRating {
				return []*models.ClassRating{rated(models.ClassTypeGlider, nil)}
			},
			flights: []flightCase{
				{"D-1234", "winch", service.LogbookNative},
				{"D-KTMG", "", service.LogbookCredited},
				{"D-EABC", "", service.LogbookExcluded},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newLogbookFixture()
			fx.seedPersonaFleet()
			licID := fx.license(tc.authority, tc.licType, tc.ratings()...)
			lb, err := fx.scope.Resolve(context.Background(), fx.userID, licID)
			if err != nil || lb == nil {
				t.Fatalf("Resolve = (%v, %v)", lb, err)
			}
			for _, fc := range tc.flights {
				if got := lb.Classify(flightOn(fc.reg, fc.launch)); got != fc.want {
					t.Errorf("Classify(%s, launch %q) = %v, want %v", fc.reg, fc.launch, got, fc.want)
				}
			}
		})
	}
}

func TestLogbookScopeApply(t *testing.T) {
	t.Run("M2 three-axis: SEP and TMG listed as untowed-only", func(t *testing.T) {
		fx := newLogbookFixture()
		fx.seedPersonaFleet()
		licID := fx.license("DULV", "UL", rated(models.ClassTypeUL, kindPtr(models.ULKindThreeAxis)))
		opts := &repository.FlightQueryOptions{}
		if err := fx.scope.Apply(context.Background(), fx.userID, licID, opts); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		if !opts.FilterByRegistrations {
			t.Fatal("FilterByRegistrations = false")
		}
		assertRegs(t, "AircraftRegistrations", opts.AircraftRegistrations, "D-MXYZ")
		assertRegs(t, "UntowedAircraftRegistrations", opts.UntowedAircraftRegistrations, "D-EABC", "D-KTMG")
	})

	t.Run("SPL glider: TMG credited with towed launches", func(t *testing.T) {
		fx := newLogbookFixture()
		fx.seedPersonaFleet()
		licID := fx.license("EASA", "SPL", rated(models.ClassTypeGlider, nil))
		opts := &repository.FlightQueryOptions{}
		if err := fx.scope.Apply(context.Background(), fx.userID, licID, opts); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		assertRegs(t, "AircraftRegistrations", opts.AircraftRegistrations, "D-1234", "D-KTMG")
		assertRegs(t, "UntowedAircraftRegistrations", opts.UntowedAircraftRegistrations)
	})

	t.Run("licence without ratings leaves opts unfiltered", func(t *testing.T) {
		fx := newLogbookFixture()
		licID := fx.license("EASA", "PPL")
		opts := &repository.FlightQueryOptions{}
		if err := fx.scope.Apply(context.Background(), fx.userID, licID, opts); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		if opts.FilterByRegistrations {
			t.Error("FilterByRegistrations = true, want false")
		}
	})

	t.Run("another user's licence is refused", func(t *testing.T) {
		fx := newLogbookFixture()
		licID := fx.license("EASA", "PPL", rated(models.ClassTypeSEPLand, nil))
		err := fx.scope.Apply(context.Background(), uuid.New(), licID, &repository.FlightQueryOptions{})
		if !errors.Is(err, service.ErrUnauthorizedAccess) {
			t.Errorf("Apply = %v, want ErrUnauthorizedAccess", err)
		}
	})

	t.Run("unknown licence is not found", func(t *testing.T) {
		fx := newLogbookFixture()
		err := fx.scope.Apply(context.Background(), fx.userID, uuid.New(), &repository.FlightQueryOptions{})
		if !errors.Is(err, service.ErrLicenseNotFound) {
			t.Errorf("Apply = %v, want ErrLicenseNotFound", err)
		}
	})
}

func assertRegs(t *testing.T, field string, got []string, want ...string) {
	t.Helper()
	g := append([]string{}, got...)
	sort.Strings(g)
	sort.Strings(want)
	if len(g) != len(want) {
		t.Errorf("%s = %v, want %v", field, g, want)
		return
	}
	for i := range g {
		if g[i] != want[i] {
			t.Errorf("%s = %v, want %v", field, g, want)
			return
		}
	}
}
