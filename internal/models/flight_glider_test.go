package models

import (
	"errors"
	"testing"
)

func TestFlight_DeriveLaunches(t *testing.T) {
	tests := []struct {
		name         string
		flight       Flight
		wantLaunches int
		wantOverride bool
	}{
		{name: "one take-off is one launch", flight: Flight{TakeoffsDay: 1}, wantLaunches: 1},
		{name: "day and night take-offs add up", flight: Flight{TakeoffsDay: 2, TakeoffsNight: 1}, wantLaunches: 3},
		{name: "a flight without take-offs has one launch", flight: Flight{}, wantLaunches: 1},
		{name: "L1 series entry keeps the pilot's count", flight: Flight{TakeoffsDay: 1, Launches: 6, LaunchesOverride: true}, wantLaunches: 6, wantOverride: true},
		{name: "override of zero is kept", flight: Flight{TakeoffsDay: 1, Launches: 0, LaunchesOverride: true}, wantLaunches: 0, wantOverride: true},
		{name: "FSTD session has none", flight: Flight{IsSimulator: true, Launches: 4, LaunchesOverride: true}, wantLaunches: 0},
		{name: "passenger flight has none", flight: Flight{IsPassenger: true, TakeoffsDay: 1}, wantLaunches: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.flight
			f.DeriveLaunches()
			if f.Launches != tt.wantLaunches || f.LaunchesOverride != tt.wantOverride {
				t.Errorf("launches = %d (override %t), want %d (override %t)",
					f.Launches, f.LaunchesOverride, tt.wantLaunches, tt.wantOverride)
			}
		})
	}
}

func TestFlight_ValidateGliderFacts(t *testing.T) {
	height := func(m int) *int { return &m }
	tests := []struct {
		name    string
		mutate  func(f *Flight)
		wantErr error
	}{
		{name: "no glider facts", mutate: func(f *Flight) {}},
		{name: "release height 0 m", mutate: func(f *Flight) { f.ReleaseHeightM = height(0) }},
		{name: "release height 20000 m", mutate: func(f *Flight) { f.ReleaseHeightM = height(20000) }},
		{name: "release height below 0", mutate: func(f *Flight) { f.ReleaseHeightM = height(-1) }, wantErr: ErrInvalidReleaseHeight},
		{name: "release height above 20000", mutate: func(f *Flight) { f.ReleaseHeightM = height(20001) }, wantErr: ErrInvalidReleaseHeight},
		{name: "negative launches", mutate: func(f *Flight) { f.Launches = -1 }, wantErr: ErrNegativeLaunches},
		{name: "outlanding and tow flight", mutate: func(f *Flight) { f.IsOutlanding, f.IsTowFlight = true, true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := validFlight()
			tt.mutate(f)
			err := f.ValidateTimeDistribution()
			if tt.wantErr == nil && err != nil {
				t.Errorf("err = %v, want nil", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
