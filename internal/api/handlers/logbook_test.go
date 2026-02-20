package handlers

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func TestLogbookFilteringLogic(t *testing.T) {
	// Simulate the filtering logic used in ListFlights
	// A license with class ratings SEP_LAND and TMG should only show flights
	// on aircraft with matching aircraft_class

	allowedClasses := map[string]bool{
		"SEP_LAND": true,
		"TMG":      true,
	}

	// Mock aircraft registry: reg → aircraftClass
	regToClass := map[string]string{
		"D-EABC": "SEP_LAND",
		"D-KXYZ": "TMG",
		"D-EFGH": "MEP_LAND", // not in allowed classes
		"D-NOPE": "",         // no class assigned
	}

	flights := []*models.Flight{
		{AircraftReg: "D-EABC"}, // SEP_LAND — should pass
		{AircraftReg: "D-KXYZ"}, // TMG — should pass
		{AircraftReg: "D-EFGH"}, // MEP_LAND — should be filtered out
		{AircraftReg: "D-NOPE"}, // no class — should be filtered out
		{AircraftReg: "D-UNKN"}, // not in registry — should be filtered out
	}

	var filtered []*models.Flight
	for _, f := range flights {
		acClass := regToClass[f.AircraftReg]
		if acClass != "" && allowedClasses[acClass] {
			filtered = append(filtered, f)
		}
	}

	if len(filtered) != 2 {
		t.Fatalf("Expected 2 filtered flights, got %d", len(filtered))
	}
	if filtered[0].AircraftReg != "D-EABC" {
		t.Errorf("First filtered flight should be D-EABC, got %s", filtered[0].AircraftReg)
	}
	if filtered[1].AircraftReg != "D-KXYZ" {
		t.Errorf("Second filtered flight should be D-KXYZ, got %s", filtered[1].AircraftReg)
	}
}

func TestLogbookFilteringAllPass(t *testing.T) {
	// When all flights match allowed classes, nothing is filtered
	allowedClasses := map[string]bool{"SEP_LAND": true}
	regToClass := map[string]string{
		"D-EABC": "SEP_LAND",
		"D-EFGH": "SEP_LAND",
	}

	flights := []*models.Flight{
		{AircraftReg: "D-EABC"},
		{AircraftReg: "D-EFGH"},
	}

	var filtered []*models.Flight
	for _, f := range flights {
		acClass := regToClass[f.AircraftReg]
		if acClass != "" && allowedClasses[acClass] {
			filtered = append(filtered, f)
		}
	}

	if len(filtered) != 2 {
		t.Fatalf("Expected 2 flights (all pass), got %d", len(filtered))
	}
}

func TestLogbookFilteringNonePass(t *testing.T) {
	// When no flights match, empty list returned
	allowedClasses := map[string]bool{"IR": true}
	regToClass := map[string]string{
		"D-EABC": "SEP_LAND",
	}

	flights := []*models.Flight{
		{AircraftReg: "D-EABC"},
	}

	var filtered []*models.Flight
	for _, f := range flights {
		acClass := regToClass[f.AircraftReg]
		if acClass != "" && allowedClasses[acClass] {
			filtered = append(filtered, f)
		}
	}

	if len(filtered) != 0 {
		t.Fatalf("Expected 0 filtered flights, got %d", len(filtered))
	}
}
