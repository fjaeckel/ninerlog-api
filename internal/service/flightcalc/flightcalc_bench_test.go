package flightcalc

import (
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// BenchmarkApplyAutoCalculations benchmarks the core flight auto-calculation pipeline.
// This is a hot-path function called on every flight create/update.
func BenchmarkApplyAutoCalculations(b *testing.B) {
	dep := "EDNY"
	arr := "EDDS"
	depTime := "08:10:00"
	arrTime := "09:50:00"
	offBlock := "08:00:00"
	onBlock := "10:00:00"
	route := "EDNY,EDTL,EDDS"
	remarks := "Benchmark test flight"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		flight := &models.Flight{
			ID:            uuid.New(),
			UserID:        uuid.New(),
			Date:          time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
			AircraftReg:   "D-EABC",
			AircraftType:  "C172",
			DepartureICAO: &dep,
			ArrivalICAO:   &arr,
			DepartureTime: &depTime,
			ArrivalTime:   &arrTime,
			OffBlockTime:  &offBlock,
			OnBlockTime:   &onBlock,
			TotalTime:     120,
			IsPIC:         true,
			PICTime:       120,
			NightTime:     0,
			IFRTime:       30,
			LandingsDay:   2,
			LandingsNight: 0,
			Route:         &route,
			Remarks:       &remarks,
		}
		ApplyAutoCalculations(flight)
	}
}

// BenchmarkApplyAutoCalculations_NightFlight benchmarks with night-time calculation.
func BenchmarkApplyAutoCalculations_NightFlight(b *testing.B) {
	dep := "EDNY"
	arr := "EDDS"
	depTime := "20:30:00"
	arrTime := "22:15:00"
	offBlock := "20:20:00"
	onBlock := "22:25:00"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		flight := &models.Flight{
			ID:            uuid.New(),
			UserID:        uuid.New(),
			Date:          time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC), // Winter for more night
			AircraftReg:   "D-EABC",
			AircraftType:  "C172",
			DepartureICAO: &dep,
			ArrivalICAO:   &arr,
			DepartureTime: &depTime,
			ArrivalTime:   &arrTime,
			OffBlockTime:  &offBlock,
			OnBlockTime:   &onBlock,
			TotalTime:     105,
			IsPIC:         true,
			PICTime:       105,
			LandingsDay:   0,
			LandingsNight: 1,
		}
		ApplyAutoCalculations(flight)
	}
}

// BenchmarkApplyAutoCalculations_WithCrew benchmarks with crew members (affects PIC/Dual).
func BenchmarkApplyAutoCalculations_WithCrew(b *testing.B) {
	dep := "EDNY"
	arr := "EDDS"
	depTime := "10:00:00"
	arrTime := "11:30:00"
	offBlock := "09:50:00"
	onBlock := "11:40:00"
	instrName := "John Instructor"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		flight := &models.Flight{
			ID:             uuid.New(),
			UserID:         uuid.New(),
			Date:           time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
			AircraftReg:    "D-EABC",
			AircraftType:   "C172",
			DepartureICAO:  &dep,
			ArrivalICAO:    &arr,
			DepartureTime:  &depTime,
			ArrivalTime:    &arrTime,
			OffBlockTime:   &offBlock,
			OnBlockTime:    &onBlock,
			TotalTime:      90,
			IsPIC:          false,
			IsDual:         true,
			DualTime:       90,
			LandingsDay:    3,
			InstructorName: &instrName,
			CrewMembers: []models.FlightCrewMember{
				{Name: "John Instructor", Role: models.CrewRoleInstructor},
				{Name: "Jane Student", Role: models.CrewRoleStudent},
			},
		}
		ApplyAutoCalculations(flight)
	}
}
