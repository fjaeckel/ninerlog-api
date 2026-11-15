package handlers

import (
	"errors"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/oapi-codegen/nullable"
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
	if err := f.ValidateHobbsObservations(); err != nil {
		return 0, err
	}
	if f.ManualTotalTime != nil {
		if *f.ManualTotalTime <= 0 || *f.ManualTotalTime > 2147483647 {
			return 0, models.ErrManualTime
		}
		return *f.ManualTotalTime, nil
	}
	switch models.EffectiveFlightTimeMethod(f.FlightTimeMethod) {
	case models.FlightTimeManual:
		return 0, models.ErrManualTime
	case models.FlightTimeHobbs:
		return models.HobbsMinutes(f.HobbsStartTenths, f.HobbsEndTenths)
	case models.FlightTimeBlock:
		if f.OffBlockTime == nil || f.OnBlockTime == nil {
			return 0, errFlightFieldsMissing
		}
		return calculateBlockTime(*f.OffBlockTime, *f.OnBlockTime)
	default:
		return 0, models.ErrFlightTimeMethod
	}
}

// Removing calculation evidence is a pilot decision, not a silent reclassification.
func reviewClockRemoval(f *models.Flight, req *generated.UpdateFlightJSONRequestBody) error {
	method := models.EffectiveFlightTimeMethod(f.FlightTimeMethod)
	if req.FlightTimeMethod != nil {
		method = string(*req.FlightTimeMethod)
	}
	if method == models.FlightTimeBlock || f.IsSimulator {
		return nil
	}
	cleared := func(old *string, update nullable.Nullable[string]) bool {
		if old == nil || *old == "" || !update.IsSpecified() {
			return false
		}
		if update.IsNull() {
			return true
		}
		v, _ := update.Get()
		return v == ""
	}
	if !cleared(f.OffBlockTime, req.OffBlockTime) && !cleared(f.OnBlockTime, req.OnBlockTime) && !cleared(f.DepartureTime, req.DepartureTime) && !cleared(f.ArrivalTime, req.ArrivalTime) {
		return nil
	}
	if req.ClockTimePolicy == nil {
		return errors.New("review clock-derived values before removing timestamps: choose preserve or recalculate")
	}
	switch string(*req.ClockTimePolicy) {
	case "preserve":
		f.NightTimeOverride = true
		f.TakeoffsDayOverride, f.TakeoffsNightOverride = true, true
		f.LandingsDayOverride, f.LandingsNightOverride = true, true
	case "recalculate":
		if !f.NightTimeOverride {
			f.NightTime = 0
		}
	default:
		return errors.New("invalid clock time policy")
	}
	return nil
}

func applyFlightTimeUpdate(f *models.Flight, req *generated.UpdateFlightJSONRequestBody) error {
	oldMethod := models.EffectiveFlightTimeMethod(f.FlightTimeMethod)
	f.FlightTimeMethod = oldMethod
	if req.FlightTimeMethod != nil {
		f.FlightTimeMethod = string(*req.FlightTimeMethod)
	}
	changedMethod := oldMethod != f.FlightTimeMethod
	applyNullable(&f.ManualTotalTime, req.ManualTotalTime)
	applyNullable(&f.HobbsStartTenths, req.HobbsStartTenths)
	applyNullable(&f.HobbsEndTenths, req.HobbsEndTenths)
	if f.IsSimulator {
		if f.FlightTimeMethod != models.FlightTimeBlock || f.HobbsStartTenths != nil || f.HobbsEndTenths != nil || f.ManualTotalTime != nil {
			return models.ErrHobbsMethod
		}
		return nil
	}
	if f.ManualTotalTime != nil || req.ManualTotalTime.IsSpecified() || f.FlightTimeMethod == models.FlightTimeHobbs || f.FlightTimeMethod == models.FlightTimeManual || changedMethod {
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
