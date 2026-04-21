package service

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// BenchmarkCreateFlight benchmarks flight creation through the service layer.
func BenchmarkCreateFlight(b *testing.B) {
	flightRepo := newMockFlightRepo()
	svc := NewFlightService(flightRepo)
	ctx := context.Background()
	userID := uuid.New()

	dep := "EDNY"
	arr := "EDDS"
	offBlock := "08:00:00"
	onBlock := "09:30:00"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		flight := &models.Flight{
			UserID:        userID,
			Date:          time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
			AircraftReg:   "D-EABC",
			AircraftType:  "C172",
			DepartureICAO: &dep,
			ArrivalICAO:   &arr,
			OffBlockTime:  &offBlock,
			OnBlockTime:   &onBlock,
			TotalTime:     90,
			IsPIC:         true,
			PICTime:       90,
			LandingsDay:   1,
		}
		_ = svc.CreateFlight(ctx, flight)
	}
}

// BenchmarkListFlights benchmarks listing flights with a populated repository.
func BenchmarkListFlights(b *testing.B) {
	flightRepo := newMockFlightRepo()
	svc := NewFlightService(flightRepo)
	ctx := context.Background()
	userID := uuid.New()

	// Pre-populate with 500 flights
	for i := 0; i < 500; i++ {
		dep := "EDNY"
		arr := "EDDS"
		flight := &models.Flight{
			UserID:        userID,
			Date:          time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			AircraftReg:   "D-EABC",
			AircraftType:  "C172",
			DepartureICAO: &dep,
			ArrivalICAO:   &arr,
			TotalTime:     90,
			IsPIC:         true,
			PICTime:       90,
			LandingsDay:   1,
		}
		_ = svc.CreateFlight(ctx, flight)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.ListFlights(ctx, userID, nil)
	}
}

// BenchmarkGetFlight benchmarks getting a single flight by ID.
func BenchmarkGetFlight(b *testing.B) {
	flightRepo := newMockFlightRepo()
	svc := NewFlightService(flightRepo)
	ctx := context.Background()
	userID := uuid.New()

	// Create a flight to retrieve
	dep := "EDNY"
	arr := "EDDS"
	flight := &models.Flight{
		UserID:        userID,
		Date:          time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
		AircraftReg:   "D-EABC",
		AircraftType:  "C172",
		DepartureICAO: &dep,
		ArrivalICAO:   &arr,
		TotalTime:     90,
		IsPIC:         true,
		PICTime:       90,
		LandingsDay:   1,
	}
	_ = svc.CreateFlight(ctx, flight)
	flightID := flight.ID

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.GetFlight(ctx, flightID, userID)
	}
}
