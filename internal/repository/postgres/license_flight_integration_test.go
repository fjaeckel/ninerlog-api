package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/repository"
	"github.com/fjaeckel/pilotlog-api/internal/repository/postgres"
	"github.com/fjaeckel/pilotlog-api/internal/testutil"
	"github.com/google/uuid"
)

func TestLicenseRepositoryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	licenseRepo := postgres.NewLicenseRepository(db)
	userRepo := postgres.NewUserRepository(db)

	ctx := context.Background()

	// Create a test user
	user := testutil.CreateTestUser("license-test@example.com", "Test User", "hashedpass")
	err := userRepo.Create(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	t.Run("Create and retrieve license", func(t *testing.T) {
		issueDate := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)
		license := &models.License{
			UserID:           user.ID,
			LicenseType:      models.LicenseTypeEASAPPL,
			LicenseNumber:    "PPL-123456",
			IssueDate:        issueDate,
			IssuingAuthority: "EASA",
			IsActive:         true,
		}

		err := licenseRepo.Create(ctx, license)
		if err != nil {
			t.Fatalf("Failed to create license: %v", err)
		}

		if license.ID == uuid.Nil {
			t.Error("Expected license ID to be set")
		}

		// Retrieve license
		retrieved, err := licenseRepo.GetByID(ctx, license.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve license: %v", err)
		}

		if retrieved.LicenseNumber != "PPL-123456" {
			t.Errorf("Expected license number PPL-123456, got %s", retrieved.LicenseNumber)
		}
	})

	t.Run("Get non-existent license", func(t *testing.T) {
		_, err := licenseRepo.GetByID(ctx, uuid.New())
		if err != repository.ErrNotFound {
			t.Errorf("Expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Get licenses by user ID", func(t *testing.T) {
		licenses, err := licenseRepo.GetByUserID(ctx, user.ID)
		if err != nil {
			t.Fatalf("Failed to get licenses: %v", err)
		}

		if len(licenses) < 1 {
			t.Error("Expected at least one license")
		}
	})

	t.Run("Update license", func(t *testing.T) {
		issueDate := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)
		expiryDate := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)

		license := &models.License{
			UserID:           user.ID,
			LicenseType:      models.LicenseTypeEASASPL,
			LicenseNumber:    "SPL-789012",
			IssueDate:        issueDate,
			IssuingAuthority: "EASA",
			IsActive:         true,
		}

		_ = licenseRepo.Create(ctx, license)

		license.ExpiryDate = &expiryDate
		license.IsActive = false

		err := licenseRepo.Update(ctx, license)
		if err != nil {
			t.Fatalf("Failed to update license: %v", err)
		}

		// Verify update
		updated, _ := licenseRepo.GetByID(ctx, license.ID)
		if updated.IsActive {
			t.Error("Expected license to be inactive")
		}
		if updated.ExpiryDate == nil || !updated.ExpiryDate.Equal(expiryDate) {
			t.Error("Expected expiry date to be updated")
		}
	})

	t.Run("Delete license", func(t *testing.T) {
		issueDate := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)
		license := &models.License{
			UserID:           user.ID,
			LicenseType:      models.LicenseTypeEASAIR,
			LicenseNumber:    "IR-345678",
			IssueDate:        issueDate,
			IssuingAuthority: "EASA",
			IsActive:         true,
		}

		_ = licenseRepo.Create(ctx, license)

		err := licenseRepo.Delete(ctx, license.ID)
		if err != nil {
			t.Fatalf("Failed to delete license: %v", err)
		}

		// Verify deletion
		_, err = licenseRepo.GetByID(ctx, license.ID)
		if err != repository.ErrNotFound {
			t.Error("Expected license to be deleted")
		}
	})
}

func TestFlightRepositoryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	flightRepo := postgres.NewFlightRepository(db)
	licenseRepo := postgres.NewLicenseRepository(db)
	userRepo := postgres.NewUserRepository(db)

	ctx := context.Background()

	// Create a test user
	user := testutil.CreateTestUser("flight-test@example.com", "Test User", "hashedpass")
	err := userRepo.Create(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	issueDate := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)
	license := &models.License{
		UserID:           user.ID,
		LicenseType:      models.LicenseTypeEASAPPL,
		LicenseNumber:    "PPL-123456",
		IssueDate:        issueDate,
		IssuingAuthority: "EASA",
		IsActive:         true,
	}
	_ = licenseRepo.Create(ctx, license)

	t.Run("Create and retrieve flight", func(t *testing.T) {
		flightDate := time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC)
		flight := &models.Flight{
			UserID:       user.ID,
			LicenseID:    license.ID,
			Date:         flightDate,
			AircraftReg:  "D-EFGH",
			AircraftType: "C172",
			TotalTime:    2.5,
			PICTime:      2.5,
			LandingsDay:  3,
		}

		err := flightRepo.Create(ctx, flight)
		if err != nil {
			t.Fatalf("Failed to create flight: %v", err)
		}

		if flight.ID == uuid.Nil {
			t.Error("Expected flight ID to be set")
		}

		// Retrieve flight
		retrieved, err := flightRepo.GetByID(ctx, flight.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve flight: %v", err)
		}

		if retrieved.AircraftReg != "D-EFGH" {
			t.Errorf("Expected aircraft reg D-EFGH, got %s", retrieved.AircraftReg)
		}
		if retrieved.TotalTime != 2.5 {
			t.Errorf("Expected total time 2.5, got %f", retrieved.TotalTime)
		}
	})

	t.Run("Get non-existent flight", func(t *testing.T) {
		_, err := flightRepo.GetByID(ctx, uuid.New())
		if err != repository.ErrNotFound {
			t.Errorf("Expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Get flights by user ID", func(t *testing.T) {
		flights, err := flightRepo.GetByUserID(ctx, user.ID, nil)
		if err != nil {
			t.Fatalf("Failed to get flights: %v", err)
		}

		if len(flights) < 1 {
			t.Error("Expected at least one flight")
		}
	})

	t.Run("Get flights with pagination", func(t *testing.T) {
		// Create multiple flights
		for i := 0; i < 5; i++ {
			flightDate := time.Date(2026, 1, 30+i, 0, 0, 0, 0, time.UTC)
			flight := &models.Flight{
				UserID:       user.ID,
				LicenseID:    license.ID,
				Date:         flightDate,
				AircraftReg:  "D-TEST",
				AircraftType: "C172",
				TotalTime:    1.0,
				PICTime:      1.0,
				LandingsDay:  1,
			}
			_ = flightRepo.Create(ctx, flight)
		}

		opts := &repository.FlightQueryOptions{
			Page:     1,
			PageSize: 3,
		}

		flights, err := flightRepo.GetByUserID(ctx, user.ID, opts)
		if err != nil {
			t.Fatalf("Failed to get flights: %v", err)
		}

		if len(flights) != 3 {
			t.Errorf("Expected 3 flights, got %d", len(flights))
		}
	})

	t.Run("Update flight", func(t *testing.T) {
		flightDate := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
		flight := &models.Flight{
			UserID:       user.ID,
			LicenseID:    license.ID,
			Date:         flightDate,
			AircraftReg:  "D-UPDATE",
			AircraftType: "C172",
			TotalTime:    2.0,
			PICTime:      2.0,
			LandingsDay:  2,
		}

		_ = flightRepo.Create(ctx, flight)

		flight.TotalTime = 3.0
		flight.PICTime = 3.0
		flight.NightTime = 0.5

		err := flightRepo.Update(ctx, flight)
		if err != nil {
			t.Fatalf("Failed to update flight: %v", err)
		}

		// Verify update
		updated, _ := flightRepo.GetByID(ctx, flight.ID)
		if updated.TotalTime != 3.0 {
			t.Errorf("Expected total time 3.0, got %f", updated.TotalTime)
		}
		if updated.NightTime != 0.5 {
			t.Errorf("Expected night time 0.5, got %f", updated.NightTime)
		}
	})

	t.Run("Delete flight", func(t *testing.T) {
		flightDate := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
		flight := &models.Flight{
			UserID:       user.ID,
			LicenseID:    license.ID,
			Date:         flightDate,
			AircraftReg:  "D-DELETE",
			AircraftType: "C172",
			TotalTime:    1.0,
			PICTime:      1.0,
			LandingsDay:  1,
		}

		_ = flightRepo.Create(ctx, flight)

		err := flightRepo.Delete(ctx, flight.ID)
		if err != nil {
			t.Fatalf("Failed to delete flight: %v", err)
		}

		// Verify deletion
		_, err = flightRepo.GetByID(ctx, flight.ID)
		if err != repository.ErrNotFound {
			t.Error("Expected flight to be deleted")
		}
	})

	t.Run("Count flights", func(t *testing.T) {
		count, err := flightRepo.CountByUserID(ctx, user.ID, nil)
		if err != nil {
			t.Fatalf("Failed to count flights: %v", err)
		}

		if count < 1 {
			t.Error("Expected at least one flight")
		}
	})
}
