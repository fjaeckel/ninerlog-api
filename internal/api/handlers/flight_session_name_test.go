package handlers

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

func TestConvertFlightSession_ResolvesAirportNames(t *testing.T) {
	airports.SetTestDB(map[string]airports.AirportInfo{
		"DE-0249": {Name: "Konz-Könen Glider Field", Latitude: 49.68, Longitude: 6.54},
	})
	defer airports.SetTestDB(nil)

	dep, arr := "DE-0249", "Meadow strip"
	got := convertToGeneratedFlightSession(&models.FlightSession{ID: uuid.New(), UserID: uuid.New(),
		Status: models.FlightSessionStatusOpen, DepartureICAO: &dep, ArrivalICAO: &arr})
	if got.DepartureAirportName == nil || *got.DepartureAirportName != "Konz-Könen Glider Field" {
		t.Errorf("DepartureAirportName = %v, want Konz-Könen Glider Field", got.DepartureAirportName)
	}
	if got.ArrivalAirportName != nil {
		t.Errorf("ArrivalAirportName = %v, want nil for free text", *got.ArrivalAirportName)
	}
}
