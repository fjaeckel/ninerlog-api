package currency

import (
	"context"
	"slices"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

func faaPPL() *models.License {
	return &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PRIVATE"}
}

func faaRating(lic *models.License, ct models.ClassType) *models.ClassRating {
	return &models.ClassRating{ID: uuid.New(), LicenseID: lic.ID, ClassType: ct}
}

func TestFAAClasses(t *testing.T) {
	cases := []struct {
		ct   models.ClassType
		want []models.ClassType
	}{
		{models.ClassTypeSEPLand, []models.ClassType{models.ClassTypeSEPLand, models.ClassTypeSETLand, models.AircraftClassSEJLand}},
		{models.ClassTypeSETLand, []models.ClassType{models.ClassTypeSEPLand, models.ClassTypeSETLand, models.AircraftClassSEJLand}},
		{models.ClassTypeSEPSea, []models.ClassType{models.ClassTypeSEPSea, models.ClassTypeSETSea}},
		{models.ClassTypeMEPLand, []models.ClassType{models.ClassTypeMEPLand, models.AircraftClassMETLand, models.AircraftClassMEJLand}},
		{models.ClassTypeMEPSea, []models.ClassType{models.ClassTypeMEPSea, models.AircraftClassMETSea}},
		{models.ClassTypeTMG, []models.ClassType{models.ClassTypeTMG}},
		{models.ClassTypeGlider, []models.ClassType{models.ClassTypeGlider}},
		{models.ClassTypeGyro, []models.ClassType{models.ClassTypeGyro}},
	}
	for _, tc := range cases {
		if got := faaClasses(tc.ct); !slices.Equal(got, tc.want) {
			t.Errorf("faaClasses(%s) = %v, want %v", tc.ct, got, tc.want)
		}
	}
}

// #303: an AMES holder flying a Twin Otter on floats is current.
func TestFAA_Passenger_MEPSeaCountsTurbopropOnFloats(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.AircraftClassMETSea] = &Progress{Landings: 4, DayLandings: 1, NightLandings: 3, Flights: 4}
	lic := faaPPL()

	result := NewFAAEvaluator().Evaluate(context.Background(), faaRating(lic, models.ClassTypeMEPSea), lic, dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current", result.Status)
	}
	want := []models.ClassType{models.ClassTypeMEPSea, models.AircraftClassMETSea}
	if !slices.Equal(result.CountedClasses, want) {
		t.Errorf("CountedClasses = %v, want %v", result.CountedClasses, want)
	}
}

func TestFAA_Passenger_SEPLandPoolsPistonTurbopropAndJet(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{Landings: 1, DayLandings: 1, Flights: 1}
	dp.progressByClass[models.ClassTypeSETLand] = &Progress{Landings: 1, DayLandings: 1, Flights: 1}
	dp.progressByClass[models.AircraftClassSEJLand] = &Progress{Landings: 1, DayLandings: 1, Flights: 1}
	lic := faaPPL()

	result := NewFAAEvaluator().Evaluate(context.Background(), faaRating(lic, models.ClassTypeSEPLand), lic, dp)
	if result.Progress == nil || result.Progress.Landings != 3 {
		t.Fatalf("Progress = %+v, want 3 landings", result.Progress)
	}
	if result.Status == StatusExpired {
		t.Errorf("Status = %s, want day current", result.Status)
	}
}

func TestFAA_Passenger_PoolsKeepEnginesAndSurfaceApart(t *testing.T) {
	cases := []struct {
		name    string
		rating  models.ClassType
		flownOn models.ClassType
	}{
		{"single-engine rating, twin turboprop", models.ClassTypeSEPLand, models.AircraftClassMETLand},
		{"single-engine sea rating, twin turboprop on floats", models.ClassTypeSEPSea, models.AircraftClassMETSea},
		{"multi-engine sea rating, twin turboprop on land", models.ClassTypeMEPSea, models.AircraftClassMETLand},
		{"multi-engine land rating, single-engine jet", models.ClassTypeMEPLand, models.AircraftClassSEJLand},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dp := newMockFlightDataProvider()
			dp.progressByClass[tc.flownOn] = &Progress{Landings: 5, DayLandings: 2, NightLandings: 3, Flights: 5}
			lic := faaPPL()
			result := NewFAAEvaluator().Evaluate(context.Background(), faaRating(lic, tc.rating), lic, dp)
			if result.Status != StatusExpired {
				t.Errorf("Status = %s, want expired", result.Status)
			}
		})
	}
}

func TestFAA_PassengerCurrency_MergesLandingDaysAcrossPool(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.landingDays = map[models.ClassType][]LandingDay{
		models.ClassTypeMEPLand:     {{Date: day(10), NightLandings: 1}},
		models.AircraftClassMETLand: {{Date: day(10), NightLandings: 1}, {Date: day(40), DayLandings: 2}},
		models.AircraftClassMEJLand: {{Date: day(20), NightLandings: 1}},
	}

	result := NewFAAEvaluator().EvaluatePassengerCurrency(context.Background(), models.ClassTypeMEPLand, faaPPL(), nil, dp)
	if result.DayLandings != 5 || result.NightLandings != 3 {
		t.Fatalf("landings = %d day+night / %d night, want 5 / 3", result.DayLandings, result.NightLandings)
	}
	if result.DayStatus != StatusCurrent || result.NightStatus != StatusCurrent {
		t.Fatalf("statuses = %s/%s, want current/current", result.DayStatus, result.NightStatus)
	}
	if want := dayStr(20 - paxWindowDays); result.NightExpiresOn == nil || *result.NightExpiresOn != want {
		t.Errorf("NightExpiresOn = %v, want %s", result.NightExpiresOn, want)
	}
}

func TestLandingDaysByClasses_SumsSameDateNewestFirst(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.landingDays = map[models.ClassType][]LandingDay{
		models.ClassTypeSEPLand: {{Date: day(3), DayLandings: 1}, {Date: day(9), NightLandings: 2}},
		models.ClassTypeSETLand: {{Date: day(3), DayLandings: 2, NightLandings: 1}, {Date: day(1), DayLandings: 1}},
	}
	got, err := landingDaysByClasses(context.Background(), dp, uuid.New(),
		[]models.ClassType{models.ClassTypeSEPLand, models.ClassTypeSETLand, models.AircraftClassSEJLand}, false, false, day(30))
	if err != nil {
		t.Fatal(err)
	}
	want := []LandingDay{
		{Date: day(1), DayLandings: 1},
		{Date: day(3), DayLandings: 3, NightLandings: 1},
		{Date: day(9), NightLandings: 2},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestEASA_MEPDoesNotCountTurbineClasses(t *testing.T) {
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.AircraftClassMETLand] = &Progress{Flights: 20, Landings: 20, InstructorMinutes: 120}
	lic := &models.License{ID: uuid.New(), UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL(A)"}
	rating := faaRating(lic, models.ClassTypeMEPLand)

	result := NewEASAEvaluator().Evaluate(context.Background(), rating, lic, dp)
	if len(result.CountedClasses) != 0 {
		t.Errorf("CountedClasses = %v, want none", result.CountedClasses)
	}
	if slices.Contains(dp.lastClasses, models.AircraftClassMETLand) {
		t.Errorf("EASA MEP queried %v, want MEP_LAND only", dp.lastClasses)
	}
	if result.Progress != nil && result.Progress.Flights != 0 {
		t.Errorf("Progress.Flights = %d, want 0", result.Progress.Flights)
	}
}
