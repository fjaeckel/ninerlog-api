package handlers

import (
	"errors"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func selectedFlightMinutes(f *models.Flight) (int, error) {
	for _, value := range []*string{f.OffBlockTime, f.OnBlockTime, f.DepartureTime, f.ArrivalTime} {
		if value != nil {
			if _, err := time.Parse("15:04:05", *value); err != nil {
				if _, err := time.Parse("15:04", *value); err != nil {
					return 0, errors.New("invalid block or takeoff/landing time format")
				}
			}
		}
	}
	switch models.EffectiveFlightTimeMethod(f.FlightTimeMethod) {
	case models.FlightTimeManual:
		if f.HobbsStartTenths != nil || f.HobbsEndTenths != nil {
			return 0, models.ErrHobbsMethod
		}
		if f.ManualTotalTime == nil || *f.ManualTotalTime <= 0 || *f.ManualTotalTime > 2147483647 {
			return 0, models.ErrManualTime
		}
		return *f.ManualTotalTime, nil
	case models.FlightTimeHobbs:
		return models.HobbsMinutes(f.HobbsStartTenths, f.HobbsEndTenths)
	case models.FlightTimeBlock:
		if f.HobbsStartTenths != nil || f.HobbsEndTenths != nil {
			return 0, models.ErrHobbsMethod
		}
		if f.OffBlockTime == nil || f.OnBlockTime == nil {
			return 0, errFlightFieldsMissing
		}
		return calculateBlockTime(*f.OffBlockTime, *f.OnBlockTime)
	default:
		return 0, models.ErrFlightTimeMethod
	}
}

func applyFlightTimeUpdate(f *models.Flight, req *generated.UpdateFlightJSONRequestBody) error {
	oldMethod := models.EffectiveFlightTimeMethod(f.FlightTimeMethod)
	f.FlightTimeMethod = oldMethod
	if req.FlightTimeMethod != nil {
		f.FlightTimeMethod = string(*req.FlightTimeMethod)
	}
	changedMethod := oldMethod != f.FlightTimeMethod
	if changedMethod && f.FlightTimeMethod != models.FlightTimeHobbs {
		f.HobbsStartTenths, f.HobbsEndTenths = nil, nil
	}
	if changedMethod && f.FlightTimeMethod != models.FlightTimeManual {
		f.ManualTotalTime = nil
	}
	applyNullable(&f.ManualTotalTime, req.ManualTotalTime)
	applyNullable(&f.HobbsStartTenths, req.HobbsStartTenths)
	applyNullable(&f.HobbsEndTenths, req.HobbsEndTenths)
	if f.IsSimulator {
		if f.FlightTimeMethod != models.FlightTimeBlock || f.HobbsStartTenths != nil || f.HobbsEndTenths != nil || f.ManualTotalTime != nil {
			return models.ErrHobbsMethod
		}
		return nil
	}
	if f.FlightTimeMethod == models.FlightTimeHobbs || f.FlightTimeMethod == models.FlightTimeManual || changedMethod {
		minutes, err := selectedFlightMinutes(f)
		if err != nil {
			return err
		}
		f.TotalTime = minutes
	} else if req.OffBlockTime.IsSpecified() || req.OnBlockTime.IsSpecified() {
		if f.OffBlockTime == nil || f.OnBlockTime == nil || *f.OffBlockTime == "" || *f.OnBlockTime == "" {
			return f.ValidateFlightTimeSource()
		}
		minutes, err := selectedFlightMinutes(f)
		if err != nil {
			return err
		}
		f.TotalTime = minutes
	} else if req.TotalTime != nil {
		f.TotalTime = *req.TotalTime
	}
	return f.ValidateFlightTimeSource()
}
