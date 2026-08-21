package models

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func sessionBase() *Flight {
	fstd := "FNPT II"
	return &Flight{
		UserID:              uuid.New(),
		Date:                time.Now(),
		AircraftType:        "PA34",
		IsSimulator:         true,
		FSTDType:            &fstd,
		SimulatedFlightTime: 120,
	}
}

func flightBase() *Flight {
	return &Flight{
		UserID:       uuid.New(),
		Date:         time.Now(),
		AircraftReg:  "D-EABC",
		AircraftType: "C172",
		TotalTime:    120,
		IsPIC:        true,
		PICTime:      120,
	}
}

func TestIsValidSimulatorSession(t *testing.T) {
	empty := ""
	cases := []struct {
		name   string
		mutate func(*Flight)
		want   bool
	}{
		{"session without registration is valid", func(f *Flight) {}, true},
		{"session needs an FSTD type", func(f *Flight) { f.FSTDType = nil }, false},
		{"session rejects an empty FSTD type", func(f *Flight) { f.FSTDType = &empty }, false},
		{"session needs a positive duration", func(f *Flight) { f.SimulatedFlightTime = 0 }, false},
		{"session still needs an aircraft type", func(f *Flight) { f.AircraftType = "" }, false},
		{"session does not need block time", func(f *Flight) { f.TotalTime = 0 }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := sessionBase()
			tc.mutate(f)
			if got := f.IsValid(); got != tc.want {
				t.Errorf("IsValid() = %v, want %v", got, tc.want)
			}
		})
	}
}

// A session must never carry flight time — AMC1 FCL.050 records session time
// separately and it is never summed with flight time.
func TestValidateSessionRejectsFlightTime(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Flight)
		want   error
	}{
		{"total time", func(f *Flight) { f.TotalTime = 120 }, ErrFSTDFlightTime},
		{"pic time", func(f *Flight) { f.PICTime = 120 }, ErrFSTDFlightTime},
		{"dual time", func(f *Flight) { f.DualTime = 120 }, ErrFSTDFlightTime},
		{"co-pilot time", func(f *Flight) { f.SICTime = 120 }, ErrFSTDFlightTime},
		{"instructor time", func(f *Flight) { f.DualGivenTime = 120 }, ErrFSTDFlightTime},
		{"multi-pilot time", func(f *Flight) { f.MultiPilotTime = 120 }, ErrFSTDFlightTime},
		{"cross-country time", func(f *Flight) { f.CrossCountryTime = 120 }, ErrFSTDFlightTime},
		{"night time", func(f *Flight) { f.NightTime = 30 }, ErrFSTDFlightTime},
		{"ifr time", func(f *Flight) { f.IFRTime = 30 }, ErrFSTDFlightTime},
		{"missing duration", func(f *Flight) { f.SimulatedFlightTime = 0 }, ErrFSTDSessionTime},
		{"missing type", func(f *Flight) { f.FSTDType = nil }, ErrFSTDTypeRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := sessionBase()
			tc.mutate(f)
			if err := f.ValidateTimeDistribution(); !errors.Is(err, tc.want) {
				t.Errorf("ValidateTimeDistribution() = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestValidateSessionAcceptsInstrumentWork(t *testing.T) {
	f := sessionBase()
	f.SimulatedInstrumentTime = 90
	f.Holds = 2
	f.ApproachesCount = 3
	if err := f.ValidateTimeDistribution(); err != nil {
		t.Errorf("ValidateTimeDistribution() = %v, want nil", err)
	}

	f.SimulatedInstrumentTime = 200
	if err := f.ValidateTimeDistribution(); !errors.Is(err, ErrInvalidIFRTime) {
		t.Errorf("instrument time above session duration = %v, want ErrInvalidIFRTime", err)
	}
}

// Total time is block time; the pilot function columns decompose it.
func TestValidateFunctionTimeDecomposesTotal(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Flight)
		want   error
	}{
		{"pic alone", func(f *Flight) { f.PICTime = 120 }, nil},
		{"co-pilot alone counts fully", func(f *Flight) {
			f.IsPIC, f.PICTime, f.SICTime = false, 0, 120
		}, nil},
		{"pic split with co-pilot", func(f *Flight) { f.PICTime, f.SICTime = 60, 60 }, nil},
		{"dual received alone", func(f *Flight) {
			f.IsPIC, f.IsDual, f.PICTime, f.DualTime = false, true, 0, 120
		}, nil},
		// An instructor logs the hour as PIC and as instructor time; the
		// two overlay rather than adding up.
		{"instructor time overlays pic", func(f *Flight) { f.DualGivenTime = 120 }, nil},
		// An FI instructing a qualified pilot who acts as PIC logs
		// instructor time with no PIC time of their own.
		{"instructor without pic time", func(f *Flight) { f.PICTime, f.DualGivenTime = 0, 120 }, nil},
		{"pic plus co-pilot over total", func(f *Flight) { f.SICTime = 60 }, ErrInvalidFunctionTime},
		{"pic plus dual over total", func(f *Flight) { f.DualTime = 60 }, ErrInvalidFunctionTime},
		{"instructor over total", func(f *Flight) { f.DualGivenTime = 180 }, ErrInvalidDualGivenTime},
		{"night over total", func(f *Flight) { f.NightTime = 180 }, ErrInvalidNightTime},
		{"ifr over total", func(f *Flight) { f.IFRTime = 180 }, ErrInvalidIFRTime},
		{"negative co-pilot time", func(f *Flight) { f.SICTime = -1 }, ErrNegativeTime},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := flightBase()
			tc.mutate(f)
			err := f.ValidateTimeDistribution()
			if tc.want == nil {
				if err != nil {
					t.Errorf("ValidateTimeDistribution() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("ValidateTimeDistribution() = %v, want %v", err, tc.want)
			}
		})
	}
}
