package models

import (
	"errors"
	"strings"
	"testing"
)

func clk(s string) *string { return &s }

func TestFlightClocksTotalMinutes(t *testing.T) {
	tests := []struct {
		name       string
		clocks     FlightClocks
		wantMin    int
		wantSource FlightTimeSource
		wantErr    error
		wantMsg    string
	}{
		{
			name:       "A2 block times unchanged: block only",
			clocks:     FlightClocks{OffBlock: clk("08:00"), OnBlock: clk("09:30")},
			wantMin:    90,
			wantSource: FlightTimeSourceBlock,
		},
		{
			name:       "L3/K3 take-off and landing only",
			clocks:     FlightClocks{Takeoff: clk("10:05:00"), Landing: clk("10:47:00")},
			wantMin:    42,
			wantSource: FlightTimeSourceAirborne,
		},
		{
			name:       "both pairs: block wins",
			clocks:     FlightClocks{OffBlock: clk("08:00"), OnBlock: clk("10:30"), Takeoff: clk("08:10"), Landing: clk("10:20")},
			wantMin:    150,
			wantSource: FlightTimeSourceBlock,
		},
		{
			name:       "block pair with a lone take-off half",
			clocks:     FlightClocks{OffBlock: clk("08:00"), OnBlock: clk("09:00"), Takeoff: clk("08:10")},
			wantMin:    60,
			wantSource: FlightTimeSourceBlock,
		},
		{
			name:       "take-off/landing pair with a lone off-block half",
			clocks:     FlightClocks{OffBlock: clk("08:00"), Takeoff: clk("08:10"), Landing: clk("09:10")},
			wantMin:    60,
			wantSource: FlightTimeSourceAirborne,
		},
		{
			name:       "take-off/landing across midnight",
			clocks:     FlightClocks{Takeoff: clk("23:30"), Landing: clk("00:45")},
			wantMin:    75,
			wantSource: FlightTimeSourceAirborne,
		},
		{
			name:       "block across midnight",
			clocks:     FlightClocks{OffBlock: clk("22:00:00"), OnBlock: clk("02:00:00")},
			wantMin:    240,
			wantSource: FlightTimeSourceBlock,
		},
		{
			name:       "blank strings count as absent",
			clocks:     FlightClocks{OffBlock: clk(" "), OnBlock: clk(""), Takeoff: clk("12:00"), Landing: clk("12:30")},
			wantMin:    30,
			wantSource: FlightTimeSourceAirborne,
		},
		{
			name:    "no times",
			clocks:  FlightClocks{},
			wantErr: ErrFlightTimesMissing,
		},
		{
			name:    "lone off-block",
			clocks:  FlightClocks{OffBlock: clk("08:00")},
			wantErr: ErrFlightTimePairIncomplete,
			wantMsg: "offBlockTime requires onBlockTime",
		},
		{
			name:    "lone on-block",
			clocks:  FlightClocks{OnBlock: clk("09:00")},
			wantErr: ErrFlightTimePairIncomplete,
			wantMsg: "onBlockTime requires offBlockTime",
		},
		{
			name:    "lone take-off",
			clocks:  FlightClocks{Takeoff: clk("08:00")},
			wantErr: ErrFlightTimePairIncomplete,
			wantMsg: "departureTime requires arrivalTime",
		},
		{
			name:    "lone landing",
			clocks:  FlightClocks{Landing: clk("09:00")},
			wantErr: ErrFlightTimePairIncomplete,
			wantMsg: "arrivalTime requires departureTime",
		},
		{
			name:    "off-block with landing is two halves",
			clocks:  FlightClocks{OffBlock: clk("08:00"), Landing: clk("09:00")},
			wantErr: ErrFlightTimePairIncomplete,
		},
		{
			name:       "unparseable take-off",
			clocks:     FlightClocks{Takeoff: clk("nope"), Landing: clk("09:00")},
			wantErr:    ErrInvalidTimeOfDay,
			wantSource: FlightTimeSourceAirborne,
		},
		{
			name:       "identical take-off and landing",
			clocks:     FlightClocks{Takeoff: clk("09:00"), Landing: clk("09:00:00")},
			wantErr:    ErrInvalidTimeOfDay,
			wantSource: FlightTimeSourceAirborne,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, source, err := tt.clocks.TotalMinutes()
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
					t.Errorf("message %q does not contain %q", err.Error(), tt.wantMsg)
				}
				if source != tt.wantSource {
					t.Errorf("source = %q, want %q", source, tt.wantSource)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantMin || source != tt.wantSource {
				t.Errorf("got %d (%s), want %d (%s)", got, source, tt.wantMin, tt.wantSource)
			}
		})
	}
}

func TestFlightClocksClassificationAndLogbook(t *testing.T) {
	tests := []struct {
		name                 string
		clocks               FlightClocks
		wantTakeoff, wantLdg string
		wantDep, wantArr     string
	}{
		{
			name:        "block pair classifies and prints",
			clocks:      FlightClocks{OffBlock: clk("08:00:00"), OnBlock: clk("09:00:00"), Takeoff: clk("08:10:00"), Landing: clk("08:50:00")},
			wantTakeoff: "08:00:00", wantLdg: "09:00:00",
			wantDep: "08:00:00", wantArr: "09:00:00",
		},
		{
			name:        "take-off/landing only",
			clocks:      FlightClocks{Takeoff: clk("08:10:00"), Landing: clk("08:50:00")},
			wantTakeoff: "08:10:00", wantLdg: "08:50:00",
			wantDep: "08:10:00", wantArr: "08:50:00",
		},
		{
			name:        "lone off-block beside a take-off/landing pair prints the pair",
			clocks:      FlightClocks{OffBlock: clk("08:00:00"), Takeoff: clk("08:10:00"), Landing: clk("08:50:00")},
			wantTakeoff: "08:00:00", wantLdg: "08:50:00",
			wantDep: "08:10:00", wantArr: "08:50:00",
		},
		{
			name: "nothing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.clocks.TakeoffClock(); got != tt.wantTakeoff {
				t.Errorf("TakeoffClock = %q, want %q", got, tt.wantTakeoff)
			}
			if got := tt.clocks.LandingClock(); got != tt.wantLdg {
				t.Errorf("LandingClock = %q, want %q", got, tt.wantLdg)
			}
			dep, arr := tt.clocks.LogbookClocks()
			if dep != tt.wantDep || arr != tt.wantArr {
				t.Errorf("LogbookClocks = %q/%q, want %q/%q", dep, arr, tt.wantDep, tt.wantArr)
			}
		})
	}
}
