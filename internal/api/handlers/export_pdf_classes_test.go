package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
)

// regClasses maps upper-cased registrations to classes regardless of date.
type regClasses map[string]string

func (c regClasses) classFor(f *models.Flight) string {
	return c[strings.ToUpper(f.AircraftReg)]
}

func TestFleetClassesFollowTheFlightDate(t *testing.T) {
	sep, mep := "SEP_LAND", "MEP_LAND"
	until := time.Date(2019, 12, 31, 0, 0, 0, 0, time.UTC)
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	classes := fleetClasses{service.NewFleetIndex([]*models.Aircraft{
		{Registration: "N123AB", AircraftClass: &sep, ValidTo: &until},
		{Registration: "N123AB", AircraftClass: &mep, ValidFrom: &from},
	})}

	if got := classOf(classes, &models.Flight{AircraftReg: "N123AB", Date: until}); got != sep {
		t.Errorf("class on %s = %q, want %q", until.Format(time.DateOnly), got, sep)
	}
	if got := classOf(classes, &models.Flight{AircraftReg: "N123AB", Date: from}); got != mep {
		t.Errorf("class on %s = %q, want %q", from.Format(time.DateOnly), got, mep)
	}
	if got := classOf(nil, &models.Flight{AircraftReg: "N123AB", Date: from}); got != "" {
		t.Errorf("nil lookup = %q, want empty", got)
	}
}
