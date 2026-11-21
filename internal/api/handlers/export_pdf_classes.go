package handlers

import (
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
)

// aircraftClassLookup resolves the aircraft class a flight was flown in.
type aircraftClassLookup interface {
	classFor(f *models.Flight) string
}

// classOf returns the class of f from classes, or "" when classes is nil.
func classOf(classes aircraftClassLookup, f *models.Flight) string {
	if classes == nil {
		return ""
	}
	return classes.classFor(f)
}

// fleetClasses resolves classes from the fleet entry valid on the flight date.
type fleetClasses struct {
	fleet service.FleetIndex
}

func (c fleetClasses) classFor(f *models.Flight) string {
	a := c.fleet.Resolve(f.AircraftReg, f.Date)
	if a == nil || a.AircraftClass == nil {
		return ""
	}
	return *a.AircraftClass
}
