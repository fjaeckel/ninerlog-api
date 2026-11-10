package models

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func validFlight() *Flight {
	return &Flight{
		UserID:       uuid.New(),
		Date:         time.Now(),
		AircraftReg:  "D-EFGH",
		AircraftType: "C172",
		TotalTime:    90,
		IsPIC:        true,
		PICTime:      90,
		LandingsDay:  2,
	}
}

func TestFlightIsValid_Valid(t *testing.T) {
	f := validFlight()
	if !f.IsValid() {
		t.Error("IsValid() = false, want true")
	}
}

func TestFlightIsValid_NilUserID(t *testing.T) {
	f := validFlight()
	f.UserID = uuid.Nil
	if f.IsValid() {
		t.Error("IsValid() = true, want false for nil UserID")
	}
}

func TestFlightIsValid_ZeroDate(t *testing.T) {
	f := validFlight()
	f.Date = time.Time{}
	if f.IsValid() {
		t.Error("IsValid() = true, want false for zero date")
	}
}

func TestFlightIsValid_EmptyAircraftReg(t *testing.T) {
	f := validFlight()
	f.AircraftReg = ""
	if f.IsValid() {
		t.Error("IsValid() = true, want false for empty AircraftReg")
	}
}

func TestFlightIsValid_EmptyAircraftType(t *testing.T) {
	f := validFlight()
	f.AircraftType = ""
	if f.IsValid() {
		t.Error("IsValid() = true, want false for empty AircraftType")
	}
}

func TestFlightIsValid_ZeroTotalTime(t *testing.T) {
	f := validFlight()
	f.TotalTime = 0
	if f.IsValid() {
		t.Error("IsValid() = true, want false for zero TotalTime")
	}
}

func TestFlightValidateTimeDistribution_Valid(t *testing.T) {
	f := validFlight()
	if err := f.ValidateTimeDistribution(); err != nil {
		t.Errorf("ValidateTimeDistribution() = %v, want nil", err)
	}
}

func TestFlightValidateTimeDistribution_PICWhileDual(t *testing.T) {
	f := validFlight()
	f.TotalTime = 96
	f.IsPIC = true
	f.IsDual = true
	f.PICTime = 96
	f.DualTime = 96
	if err := f.ValidateTimeDistribution(); err != nil {
		t.Errorf("ValidateTimeDistribution() = %v, want nil", err)
	}
	if got := f.PICDualOverlap(); got != 96 {
		t.Errorf("PICDualOverlap() = %d, want 96", got)
	}
}

func TestFlightValidateTimeDistribution_PICWhileDualPartial(t *testing.T) {
	f := validFlight()
	f.TotalTime = 90
	f.IsPIC = true
	f.IsDual = true
	f.PICTime = 60
	f.DualTime = 90
	if err := f.ValidateTimeDistribution(); err != nil {
		t.Errorf("ValidateTimeDistribution() = %v, want nil", err)
	}
}

func TestFlightValidateTimeDistribution_PICWhileDualCountsOverlapOnce(t *testing.T) {
	f := validFlight()
	f.TotalTime = 96
	f.IsPIC = true
	f.IsDual = true
	f.PICTime = 96
	f.DualTime = 96
	f.PICUSTime = 10
	if err := f.ValidateTimeDistribution(); !errors.Is(err, ErrInvalidFunctionTime) {
		t.Errorf("ValidateTimeDistribution() = %v, want ErrInvalidFunctionTime", err)
	}
}

func TestFlightValidateTimeDistribution_PICAndDualTimeWithoutBothFlags(t *testing.T) {
	f := validFlight()
	f.IsPIC = true
	f.IsDual = false
	f.PICTime = 45
	f.DualTime = 45
	if err := f.ValidateTimeDistribution(); !errors.Is(err, ErrInvalidTimeDistribution) {
		t.Errorf("ValidateTimeDistribution() = %v, want ErrInvalidTimeDistribution", err)
	}
}

func TestFlightPICDualOverlap_NotBoth(t *testing.T) {
	f := validFlight()
	f.DualTime = 30
	if got := f.PICDualOverlap(); got != 0 {
		t.Errorf("PICDualOverlap() = %d, want 0", got)
	}
}

func TestFlightValidateTimeDistribution_NightExceedsTotal(t *testing.T) {
	f := validFlight()
	f.NightTime = f.TotalTime + 1
	if err := f.ValidateTimeDistribution(); !errors.Is(err, ErrInvalidNightTime) {
		t.Errorf("ValidateTimeDistribution() = %v, want ErrInvalidNightTime", err)
	}
}

func TestFlightValidateTimeDistribution_IFRExceedsTotal(t *testing.T) {
	f := validFlight()
	f.IFRTime = f.TotalTime + 1
	if err := f.ValidateTimeDistribution(); !errors.Is(err, ErrInvalidIFRTime) {
		t.Errorf("ValidateTimeDistribution() = %v, want ErrInvalidIFRTime", err)
	}
}

func TestFlightValidateTimeDistribution_NegativeTime(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(f *Flight)
		wantErr error
	}{
		{"negative TotalTime", func(f *Flight) { f.TotalTime = -1; f.NightTime = -1; f.IFRTime = -1 }, ErrNegativeTime},
		{"negative NightTime", func(f *Flight) { f.NightTime = -1 }, ErrNegativeTime},
		{"negative IFRTime", func(f *Flight) { f.IFRTime = -1 }, ErrNegativeTime},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := validFlight()
			tt.setup(f)
			if err := f.ValidateTimeDistribution(); !errors.Is(err, tt.wantErr) {
				t.Errorf("ValidateTimeDistribution() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestFlightValidateTimeDistribution_NegativeLandings(t *testing.T) {
	tests := []struct {
		name  string
		setup func(f *Flight)
	}{
		{"negative LandingsDay", func(f *Flight) { f.LandingsDay = -1 }},
		{"negative LandingsNight", func(f *Flight) { f.LandingsNight = -1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := validFlight()
			tt.setup(f)
			if err := f.ValidateTimeDistribution(); !errors.Is(err, ErrNegativeLandings) {
				t.Errorf("ValidateTimeDistribution() = %v, want ErrNegativeLandings", err)
			}
		})
	}
}

func TestFlightValidateTimeDistribution_Dual(t *testing.T) {
	f := validFlight()
	f.IsPIC = false
	f.IsDual = true
	f.DualTime = 90
	f.PICTime = 0
	if err := f.ValidateTimeDistribution(); err != nil {
		t.Errorf("ValidateTimeDistribution() for dual = %v, want nil", err)
	}
}

func TestFlightStatisticsDefaults(t *testing.T) {
	s := FlightStatistics{}
	if s.TotalFlights != 0 || s.TotalMinutes != 0 {
		t.Error("Default FlightStatistics should have zero values")
	}
}

func TestCurrencyDataDefaults(t *testing.T) {
	d := CurrencyData{}
	if d.Flights != 0 || d.TotalLandings != 0 || d.DayLandings != 0 || d.NightLandings != 0 {
		t.Error("Default CurrencyData should have zero values")
	}
}
