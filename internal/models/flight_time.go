package models

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	FlightTimeBlock  = "block"
	FlightTimeHobbs  = "hobbs"
	FlightTimeManual = "manual"
	MaxHobbsTenths   = 99999999
)

var (
	ErrFlightTimeMethod = errors.New("flightTimeMethod must be block, hobbs or manual")
	ErrHobbsReadings    = errors.New("the Hobbs start and end must be whole tenths between 0 and 9999999.9 hours")
	ErrHobbsOrder       = errors.New("the Hobbs end must be greater than Hobbs start")
	ErrHobbsMethod      = errors.New("the Hobbs meter readings require the Hobbs flight time method and cannot be used for simulator sessions")
	ErrManualTime       = errors.New("manual flight time must be a positive duration in integer minutes")
	ErrManualMethod     = errors.New("simulator sessions cannot contain an entered flight duration")
	ErrHobbsTotal       = errors.New("totalTime must match the Hobbs meter difference")
)

// EffectiveFlightTimeMethod treats legacy records as block flights.
func EffectiveFlightTimeMethod(method string) string {
	if method == "" {
		return FlightTimeBlock
	}
	return method
}

// HobbsMinutes converts a pair of integer-tenths readings to minutes.
func HobbsMinutes(start, end *int) (int, error) {
	if start == nil || end == nil || *start < 0 || *end < 0 || *start > MaxHobbsTenths || *end > MaxHobbsTenths {
		return 0, ErrHobbsReadings
	}
	if *end <= *start {
		return 0, ErrHobbsOrder
	}
	return (*end - *start) * 6, nil
}

// ValidateFlightTimeSource checks the saved method, readings and recorded total.
func (f *Flight) ValidateFlightTimeSource() error {
	method := EffectiveFlightTimeMethod(f.FlightTimeMethod)
	if method != FlightTimeBlock && method != FlightTimeHobbs && method != FlightTimeManual {
		return ErrFlightTimeMethod
	}
	if f.IsSimulator {
		if method != FlightTimeBlock || f.HobbsStartTenths != nil || f.HobbsEndTenths != nil || f.ManualTotalTime != nil {
			return ErrHobbsMethod
		}
		return nil
	}
	if err := f.ValidateHobbsObservations(); err != nil {
		return err
	}
	if f.ManualTotalTime != nil {
		if *f.ManualTotalTime <= 0 || *f.ManualTotalTime > 2147483647 || (!f.IsPassenger && f.TotalTime != *f.ManualTotalTime) {
			return ErrManualTime
		}
		return nil
	}
	if method == FlightTimeManual {
		return ErrManualTime
	}
	if method == FlightTimeBlock {
		return nil // Preserve legacy recorded block totals, including manual adjustments.
	}
	minutes, err := HobbsMinutes(f.HobbsStartTenths, f.HobbsEndTenths)
	if err != nil {
		return err
	}
	if !f.IsPassenger && f.TotalTime != minutes {
		return ErrHobbsTotal
	}
	return nil
}

// Observations survive source changes. A partial pair is evidence, not a calculation.
func (f *Flight) ValidateHobbsObservations() error {
	for _, value := range []*int{f.HobbsStartTenths, f.HobbsEndTenths} {
		if value != nil && (*value < 0 || *value > MaxHobbsTenths) {
			return ErrHobbsReadings
		}
	}
	if f.HobbsStartTenths != nil && f.HobbsEndTenths != nil && *f.HobbsEndTenths <= *f.HobbsStartTenths {
		return ErrHobbsOrder
	}
	return nil
}

var hobbsReadingPattern = regexp.MustCompile(`^[0-9]{1,7}([.,][0-9])?$`)

// ParseHobbsReading parses decimal meter hours as integer tenths.
func ParseHobbsReading(value string) (int, error) {
	value = strings.TrimSpace(value)
	if !hobbsReadingPattern.MatchString(value) {
		return 0, ErrHobbsReadings
	}
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '.' || r == ',' })
	whole, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, ErrHobbsReadings
	}
	tenths := 0
	if len(parts) == 2 {
		tenths = int(parts[1][0] - '0')
	}
	return whole*10 + tenths, nil
}

// FormatHobbsReading formats integer tenths as decimal meter hours.
func FormatHobbsReading(value *int) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d", *value/10, *value%10)
}
