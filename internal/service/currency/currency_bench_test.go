package currency

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// BenchmarkEASAEvaluator benchmarks the EASA currency evaluator for a single class rating.
func BenchmarkEASAEvaluator(b *testing.B) {
	evaluator := NewEASAEvaluator()
	ctx := context.Background()

	userID := uuid.New()
	licenseID := uuid.New()
	now := time.Now()
	expiry := now.AddDate(1, 0, 0)

	rating := &models.ClassRating{
		ID:         uuid.New(),
		LicenseID:  licenseID,
		ClassType:  models.ClassTypeSEPLand,
		IssueDate:  now.AddDate(-2, 0, 0),
		ExpiryDate: &expiry,
	}
	license := &models.License{
		ID:                  licenseID,
		UserID:              userID,
		RegulatoryAuthority: "EASA",
		LicenseType:         "PPL",
		LicenseNumber:       "DE-PPL-BENCH",
		IssueDate:           now.AddDate(-3, 0, 0),
		IssuingAuthority:    "LBA",
	}

	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 6000,
		PICMinutes:   5000,
		Flights:      60,
		Landings:     80,
		DayLandings:  70,
		NightLandings: 10,
	}

	profCheck := now.AddDate(0, -6, 0)
	dp.lastProficiencyCheck = &profCheck

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		evaluator.Evaluate(ctx, rating, license, dp)
	}
}

// BenchmarkEvaluateAll benchmarks the full two-tier currency evaluation pipeline.
func BenchmarkEvaluateAll(b *testing.B) {
	ctx := context.Background()
	userID := uuid.New()
	now := time.Now()
	expiry := now.AddDate(1, 0, 0)

	// Set up registry with EASA evaluator
	registry := NewRegistry()
	registry.Register(NewEASAEvaluator())

	// Mock license repo
	licRepo := newMockLicenseRepo()
	licenseID := uuid.New()
	license := &models.License{
		ID:                  licenseID,
		UserID:              userID,
		RegulatoryAuthority: "EASA",
		LicenseType:         "PPL",
		LicenseNumber:       "DE-PPL-BENCH",
		IssueDate:           now.AddDate(-3, 0, 0),
		IssuingAuthority:    "LBA",
	}
	licRepo.licenses[licenseID] = license

	// Mock class rating repo
	crRepo := newMockCRRepo()
	crRepo.ratings[licenseID] = []*models.ClassRating{
		{
			ID:         uuid.New(),
			LicenseID:  licenseID,
			ClassType:  models.ClassTypeSEPLand,
			IssueDate:  now.AddDate(-2, 0, 0),
			ExpiryDate: &expiry,
		},
	}

	// Mock flight data provider
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalMinutes: 6000,
		PICMinutes:   5000,
		Flights:      60,
		Landings:     80,
		DayLandings:  70,
		NightLandings: 10,
	}
	dp.progressAll = &Progress{
		TotalMinutes: 6000,
		PICMinutes:   5000,
		Flights:      60,
		Landings:     80,
	}
	review := now.AddDate(0, -12, 0)
	dp.lastFlightReview = &review
	profCheck := now.AddDate(0, -6, 0)
	dp.lastProficiencyCheck = &profCheck

	svc := NewService(registry, licRepo, crRepo, dp)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.EvaluateAll(ctx, userID)
	}
}

// BenchmarkEvaluateAll_MultipleRatings benchmarks with multiple class ratings.
func BenchmarkEvaluateAll_MultipleRatings(b *testing.B) {
	ctx := context.Background()
	userID := uuid.New()
	now := time.Now()
	expiry := now.AddDate(1, 0, 0)

	registry := NewRegistry()
	registry.Register(NewEASAEvaluator())

	licRepo := newMockLicenseRepo()
	licenseID := uuid.New()
	license := &models.License{
		ID:                  licenseID,
		UserID:              userID,
		RegulatoryAuthority: "EASA",
		LicenseType:         "PPL",
		LicenseNumber:       "DE-PPL-BENCH",
		IssueDate:           now.AddDate(-3, 0, 0),
		IssuingAuthority:    "LBA",
	}
	licRepo.licenses[licenseID] = license

	// Multiple ratings
	crRepo := newMockCRRepo()
	crRepo.ratings[licenseID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: licenseID, ClassType: models.ClassTypeSEPLand, IssueDate: now.AddDate(-2, 0, 0), ExpiryDate: &expiry},
		{ID: uuid.New(), LicenseID: licenseID, ClassType: models.ClassTypeMEPLand, IssueDate: now.AddDate(-1, 0, 0), ExpiryDate: &expiry},
		{ID: uuid.New(), LicenseID: licenseID, ClassType: models.ClassTypeTMG, IssueDate: now.AddDate(-1, 0, 0), ExpiryDate: &expiry},
	}

	dp := newMockFlightDataProvider()
	for _, ct := range []models.ClassType{models.ClassTypeSEPLand, models.ClassTypeMEPLand, models.ClassTypeTMG} {
		dp.progressByClass[ct] = &Progress{
			TotalMinutes: 3000, PICMinutes: 2500, Flights: 30, Landings: 40,
			DayLandings: 35, NightLandings: 5,
		}
	}
	dp.progressAll = &Progress{TotalMinutes: 9000, PICMinutes: 7500, Flights: 90, Landings: 120}
	review := now.AddDate(0, -12, 0)
	dp.lastFlightReview = &review

	svc := NewService(registry, licRepo, crRepo, dp)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.EvaluateAll(ctx, userID)
	}
}
