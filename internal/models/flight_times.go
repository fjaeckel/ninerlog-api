package models

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// Flight time-of-day errors.
var (
	ErrFlightTimesMissing       = errors.New("offBlockTime and onBlockTime, or departureTime and arrivalTime, are required for a flight")
	ErrFlightTimePairIncomplete = errors.New("incomplete time pair")
	ErrInvalidTimeOfDay         = errors.New("invalid time of day")
)

// FlightTimeSource names the pair of clock times a flight's total time spans.
type FlightTimeSource string

const (
	FlightTimeSourceNone     FlightTimeSource = ""
	FlightTimeSourceBlock    FlightTimeSource = "block"
	FlightTimeSourceAirborne FlightTimeSource = "airborne"
)

// FlightClocks holds the four clock times of a flight as HH:MM or HH:MM:SS
// strings; nil or blank means absent.
type FlightClocks struct {
	OffBlock *string
	OnBlock  *string
	Takeoff  *string
	Landing  *string
}

// ClocksOf returns the clock times stored on f.
func ClocksOf(f *Flight) FlightClocks {
	return FlightClocks{
		OffBlock: f.OffBlockTime,
		OnBlock:  f.OnBlockTime,
		Takeoff:  f.DepartureTime,
		Landing:  f.ArrivalTime,
	}
}

func clockValue(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

// Pair returns the start and end of the span that bounds the flight: off-block
// to on-block when both are set, otherwise take-off to landing when both are
// set. ok is false when neither pair is complete.
func (c FlightClocks) Pair() (start, end string, source FlightTimeSource, ok bool) {
	if off, on := clockValue(c.OffBlock), clockValue(c.OnBlock); off != "" && on != "" {
		return off, on, FlightTimeSourceBlock, true
	}
	if to, ldg := clockValue(c.Takeoff), clockValue(c.Landing); to != "" && ldg != "" {
		return to, ldg, FlightTimeSourceAirborne, true
	}
	return "", "", FlightTimeSourceNone, false
}

// TakeoffClock returns the clock time that classifies the take-off as day or
// night: off-block when set, otherwise take-off.
func (c FlightClocks) TakeoffClock() string {
	if off := clockValue(c.OffBlock); off != "" {
		return off
	}
	return clockValue(c.Takeoff)
}

// LandingClock returns the clock time that classifies the landing as day or
// night: on-block when set, otherwise landing.
func (c FlightClocks) LandingClock() string {
	if on := clockValue(c.OnBlock); on != "" {
		return on
	}
	return clockValue(c.Landing)
}

// LogbookClocks returns the departure and arrival times a logbook row prints:
// the complete pair from Pair, otherwise TakeoffClock and LandingClock.
func (c FlightClocks) LogbookClocks() (departure, arrival string) {
	if start, end, _, ok := c.Pair(); ok {
		return start, end
	}
	return c.TakeoffClock(), c.LandingClock()
}

// Validate checks that at least one complete pair is present and that no
// lone half of a pair stands without a complete pair beside it. The returned
// error wraps ErrFlightTimesMissing or ErrFlightTimePairIncomplete.
func (c FlightClocks) Validate() error {
	if _, _, _, ok := c.Pair(); ok {
		return nil
	}
	halves := []struct {
		have, missing string
		present       bool
	}{
		{"offBlockTime", "onBlockTime", clockValue(c.OffBlock) != ""},
		{"onBlockTime", "offBlockTime", clockValue(c.OnBlock) != ""},
		{"departureTime", "arrivalTime", clockValue(c.Takeoff) != ""},
		{"arrivalTime", "departureTime", clockValue(c.Landing) != ""},
	}
	for _, h := range halves {
		if h.present {
			return fmt.Errorf("%w: %s requires %s; a flight needs offBlockTime and onBlockTime, or departureTime and arrivalTime",
				ErrFlightTimePairIncomplete, h.have, h.missing)
		}
	}
	return ErrFlightTimesMissing
}

// TotalMinutes validates the clocks and returns the span of Pair in minutes.
func (c FlightClocks) TotalMinutes() (int, FlightTimeSource, error) {
	if err := c.Validate(); err != nil {
		return 0, FlightTimeSourceNone, err
	}
	start, end, source, _ := c.Pair()
	m, err := ClockSpanMinutes(start, end)
	if err != nil {
		return 0, source, err
	}
	return m, source, nil
}

// ParseClock parses an HH:MM:SS or HH:MM time of day.
func ParseClock(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse("15:04:05", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("15:04", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("%w: %q", ErrInvalidTimeOfDay, s)
}

// ClockSpanMinutes returns the minutes from start to end, rounded. An end
// earlier than start is on the following day. Identical times are an error.
func ClockSpanMinutes(start, end string) (int, error) {
	startT, err := ParseClock(start)
	if err != nil {
		return 0, err
	}
	endT, err := ParseClock(end)
	if err != nil {
		return 0, err
	}
	d := endT.Sub(startT)
	if d < 0 {
		d += 24 * time.Hour
	}
	if d == 0 {
		return 0, fmt.Errorf("%w: start and end times cannot be identical", ErrInvalidTimeOfDay)
	}
	return int(math.Round(d.Minutes())), nil
}
