package flightcalc

import (
	"testing"
)

func intPtr(v int) *int { return &v }

func TestApplyAutoCalculations_OutlandingIsNotCrossCountry(t *testing.T) {
	tests := []struct {
		name       string
		outlanding bool
		xcOverride *int
		wantXC     int
		wantXCFlag bool
	}{
		{name: "planned arrival away is cross-country", wantXC: 90},
		{name: "P1 outlanding derives no cross-country time", outlanding: true, wantXC: 0},
		{name: "outlanding keeps a declared cross-country time", outlanding: true, xcOverride: intPtr(40), wantXC: 40, wantXCFlag: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := baseFlight()
			f.IsOutlanding = tt.outlanding
			if tt.xcOverride != nil {
				f.CrossCountryTime = *tt.xcOverride
				f.CrossCountryTimeOverride = true
			}
			ApplyAutoCalculations(f, "", nil)
			if f.CrossCountryTime != tt.wantXC || f.CrossCountryTimeOverride != tt.wantXCFlag {
				t.Errorf("crossCountryTime = %d (override %t), want %d (override %t)",
					f.CrossCountryTime, f.CrossCountryTimeOverride, tt.wantXC, tt.wantXCFlag)
			}
		})
	}
}

func TestApplyAutoCalculations_Launches(t *testing.T) {
	tests := []struct {
		name         string
		landings     int
		launches     *int
		wantLaunches int
		wantOverride bool
	}{
		{name: "L1 winch circuit is one launch", landings: 1, wantLaunches: 1},
		{name: "three touch-and-goes are three launches", landings: 3, wantLaunches: 3},
		{name: "L1 series entry keeps six launches", landings: 1, launches: intPtr(6), wantLaunches: 6, wantOverride: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := baseFlight()
			f.LandingsDay = 0
			f.AllLandings = tt.landings
			if tt.launches != nil {
				f.Launches = *tt.launches
				f.LaunchesOverride = true
			}
			ApplyAutoCalculations(f, "", nil)
			if f.Launches != tt.wantLaunches || f.LaunchesOverride != tt.wantOverride {
				t.Errorf("launches = %d (override %t), want %d (override %t)",
					f.Launches, f.LaunchesOverride, tt.wantLaunches, tt.wantOverride)
			}
		})
	}
}

func TestApplyAutoCalculations_SessionClearsGliderFacts(t *testing.T) {
	f := baseFlight()
	f.IsSimulator = true
	fstd := "FNPT II"
	f.FSTDType = &fstd
	f.SimulatedFlightTime = 60
	f.Launches, f.LaunchesOverride = 3, true
	f.IsOutlanding, f.IsTowFlight = true, true
	f.ReleaseHeightM = intPtr(400)
	ApplyAutoCalculations(f, "", nil)
	if f.Launches != 0 || f.LaunchesOverride || f.IsOutlanding || f.IsTowFlight || f.ReleaseHeightM != nil {
		t.Errorf("session kept glider facts: %+v", f)
	}
}
