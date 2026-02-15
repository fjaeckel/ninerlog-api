package currency

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/repository"
	"github.com/google/uuid"
)

// ── Mock flight data provider ───────────────────────────────────────────

type mockFlightDataProvider struct {
	progressByClass map[models.ClassType]*Progress
	progressAll     *Progress
}

func newMockFlightDataProvider() *mockFlightDataProvider {
	return &mockFlightDataProvider{
		progressByClass: make(map[models.ClassType]*Progress),
	}
}

func (m *mockFlightDataProvider) GetProgressByAircraftClass(_ context.Context, _ uuid.UUID, classType models.ClassType, _ time.Time) (*Progress, error) {
	if p, ok := m.progressByClass[classType]; ok {
		return p, nil
	}
	return &Progress{}, nil
}

func (m *mockFlightDataProvider) GetProgressAll(_ context.Context, _ uuid.UUID, _ time.Time) (*Progress, error) {
	if m.progressAll != nil {
		return m.progressAll, nil
	}
	return &Progress{}, nil
}

// ── Mock repositories ───────────────────────────────────────────────────

type mockLicenseRepo struct {
	licenses map[uuid.UUID]*models.License
}

func newMockLicenseRepo() *mockLicenseRepo {
	return &mockLicenseRepo{licenses: make(map[uuid.UUID]*models.License)}
}

func (m *mockLicenseRepo) Create(_ context.Context, lic *models.License) error {
	lic.ID = uuid.New()
	m.licenses[lic.ID] = lic
	return nil
}

func (m *mockLicenseRepo) GetByID(_ context.Context, id uuid.UUID) (*models.License, error) {
	l, ok := m.licenses[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return l, nil
}

func (m *mockLicenseRepo) GetByUserID(_ context.Context, userID uuid.UUID) ([]*models.License, error) {
	var result []*models.License
	for _, l := range m.licenses {
		if l.UserID == userID {
			result = append(result, l)
		}
	}
	return result, nil
}

func (m *mockLicenseRepo) Update(_ context.Context, lic *models.License) error {
	m.licenses[lic.ID] = lic
	return nil
}

func (m *mockLicenseRepo) Delete(_ context.Context, id uuid.UUID) error {
	delete(m.licenses, id)
	return nil
}

type mockCRRepo struct {
	ratings map[uuid.UUID][]*models.ClassRating
}

func newMockCRRepo() *mockCRRepo {
	return &mockCRRepo{ratings: make(map[uuid.UUID][]*models.ClassRating)}
}

func (m *mockCRRepo) GetByLicenseID(_ context.Context, licenseID uuid.UUID) ([]*models.ClassRating, error) {
	return m.ratings[licenseID], nil
}

// ── Mock evaluator for service tests ────────────────────────────────────

type mockEvaluator struct {
	authority string
	status    Status
}

func (e *mockEvaluator) Authority() string { return e.authority }

func (e *mockEvaluator) Evaluate(_ context.Context, rating *models.ClassRating, license *models.License, _ FlightDataProvider) ClassRatingCurrency {
	return ClassRatingCurrency{
		ClassRatingID:       rating.ID,
		ClassType:           rating.ClassType,
		LicenseID:           license.ID,
		RegulatoryAuthority: license.RegulatoryAuthority,
		LicenseType:         license.LicenseType,
		Status:              e.status,
		Message:             "mock evaluation",
	}
}

// ── Helper functions ────────────────────────────────────────────────────

func futureDate(months int) *time.Time {
	t := time.Now().AddDate(0, months, 0)
	return &t
}

func pastDate(days int) *time.Time {
	t := time.Now().AddDate(0, 0, -days)
	return &t
}

// ── Registry Tests ──────────────────────────────────────────────────────

func TestRegistryGetAndRegister(t *testing.T) {
	reg := NewRegistry()
	eval := &mockEvaluator{authority: "FAA", status: StatusCurrent}
	reg.Register(eval)

	if !reg.HasEvaluator("FAA") {
		t.Error("Expected HasEvaluator(FAA) = true")
	}
	if reg.HasEvaluator("EASA") {
		t.Error("Expected HasEvaluator(EASA) = false")
	}
	if reg.Get("FAA") != eval {
		t.Error("Expected Get(FAA) to return registered evaluator")
	}
	if reg.Get("EASA") != nil {
		t.Error("Expected Get(EASA) to return nil")
	}
}

// ── OtherEvaluator Tests ────────────────────────────────────────────────

func TestOtherEvaluator_NoExpiry(t *testing.T) {
	eval := NewOtherEvaluator()
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand}
	license := &models.License{ID: uuid.New(), RegulatoryAuthority: "OTHER", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, nil)
	if result.Status != StatusUnknown {
		t.Errorf("Status = %s, want unknown", result.Status)
	}
}

func TestOtherEvaluator_Expired(t *testing.T) {
	eval := NewOtherEvaluator()
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, ExpiryDate: pastDate(30)}
	license := &models.License{ID: uuid.New(), RegulatoryAuthority: "OTHER", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, nil)
	if result.Status != StatusExpired {
		t.Errorf("Status = %s, want expired", result.Status)
	}
}

func TestOtherEvaluator_Expiring(t *testing.T) {
	eval := NewOtherEvaluator()
	soon := time.Now().Add(30 * 24 * time.Hour)
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeMEPLand, ExpiryDate: &soon}
	license := &models.License{ID: uuid.New(), RegulatoryAuthority: "OTHER", LicenseType: "CPL"}

	result := eval.Evaluate(context.Background(), rating, license, nil)
	if result.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring", result.Status)
	}
}

func TestOtherEvaluator_Current(t *testing.T) {
	eval := NewOtherEvaluator()
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeIR, ExpiryDate: futureDate(12)}
	license := &models.License{ID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, nil)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current", result.Status)
	}
}

// ── Service Tests ───────────────────────────────────────────────────────

func TestServiceEvaluateAll_Empty(t *testing.T) {
	svc := NewService(NewRegistry(), newMockLicenseRepo(), newMockCRRepo(), newMockFlightDataProvider())

	result, err := svc.EvaluateAll(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}
	if len(result.Ratings) != 0 {
		t.Errorf("Expected empty ratings, got %d", len(result.Ratings))
	}
}

func TestServiceEvaluateAll_WithMockEvaluator(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	reg := NewRegistry()
	reg.Register(&mockEvaluator{authority: "FAA", status: StatusCurrent})

	userID := uuid.New()
	lic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "FAA", LicenseType: "PPL"}
	licRepo.licenses[lic.ID] = lic
	crRepo.ratings[lic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: lic.ID, ClassType: models.ClassTypeSEPLand},
	}

	svc := NewService(reg, licRepo, crRepo, newMockFlightDataProvider())
	result, err := svc.EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}
	if len(result.Ratings) != 1 {
		t.Fatalf("Expected 1 rating, got %d", len(result.Ratings))
	}
	if result.Ratings[0].Status != StatusCurrent {
		t.Errorf("Status = %s, want current", result.Ratings[0].Status)
	}
}

func TestServiceEvaluateAll_FallbackToOther(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()

	userID := uuid.New()
	lic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "DGCA", LicenseType: "PPL"}
	licRepo.licenses[lic.ID] = lic
	crRepo.ratings[lic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: lic.ID, ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(12)},
	}

	svc := NewService(NewRegistry(), licRepo, crRepo, newMockFlightDataProvider())
	result, err := svc.EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}
	if len(result.Ratings) != 1 {
		t.Fatalf("Expected 1 rating, got %d", len(result.Ratings))
	}
	if result.Ratings[0].Status != StatusCurrent {
		t.Errorf("Status = %s, want current (fallback)", result.Ratings[0].Status)
	}
}

func TestServiceEvaluateAll_MultiLicense(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	reg := NewRegistry()
	reg.Register(&mockEvaluator{authority: "FAA", status: StatusCurrent})
	reg.Register(&mockEvaluator{authority: "EASA", status: StatusExpiring})

	userID := uuid.New()
	faaLic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "FAA", LicenseType: "PPL"}
	easaLic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	licRepo.licenses[faaLic.ID] = faaLic
	licRepo.licenses[easaLic.ID] = easaLic

	crRepo.ratings[faaLic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: faaLic.ID, ClassType: models.ClassTypeSEPLand},
	}
	crRepo.ratings[easaLic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: easaLic.ID, ClassType: models.ClassTypeSEPLand},
		{ID: uuid.New(), LicenseID: easaLic.ID, ClassType: models.ClassTypeIR},
	}

	svc := NewService(reg, licRepo, crRepo, newMockFlightDataProvider())
	result, err := svc.EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}
	if len(result.Ratings) != 3 {
		t.Fatalf("Expected 3 ratings, got %d", len(result.Ratings))
	}

	faaCount, easaCount := 0, 0
	for _, r := range result.Ratings {
		if r.RegulatoryAuthority == "FAA" {
			faaCount++
		}
		if r.RegulatoryAuthority == "EASA" {
			easaCount++
		}
	}
	if faaCount != 1 || easaCount != 2 {
		t.Errorf("FAA=%d EASA=%d, want FAA=1 EASA=2", faaCount, easaCount)
	}
}

// ── EASA Evaluator Tests ────────────────────────────────────────────────

func TestEASA_SEP_AllRequirementsMet_Current(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalHours: 15, PICHours: 8, Landings: 20, InstructorHours: 2, Flights: 12,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(12), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current", result.Status)
	}
	if len(result.Requirements) != 4 {
		t.Fatalf("Expected 4 requirements, got %d", len(result.Requirements))
	}
	for _, req := range result.Requirements {
		if !req.Met {
			t.Errorf("Requirement %q should be met", req.Name)
		}
	}
}

func TestEASA_SEP_InsufficientPIC_Expiring(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalHours: 15, PICHours: 3, Landings: 20, InstructorHours: 2,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(6), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring", result.Status)
	}
	for _, req := range result.Requirements {
		if req.Name == "PIC Hours" && req.Met {
			t.Error("PIC Hours should NOT be met")
		}
	}
}

func TestEASA_SEP_InsufficientLandings_Expiring(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalHours: 15, PICHours: 8, Landings: 5, InstructorHours: 2,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(6), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring", result.Status)
	}
}

func TestEASA_SEP_Expired(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalHours: 15, PICHours: 8, Landings: 20, InstructorHours: 2,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, ExpiryDate: pastDate(30), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpired {
		t.Errorf("Status = %s, want expired", result.Status)
	}
}

func TestEASA_SEP_NoExpiry_Unknown(t *testing.T) {
	eval := NewEASAEvaluator()
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, newMockFlightDataProvider())
	if result.Status != StatusUnknown {
		t.Errorf("Status = %s, want unknown", result.Status)
	}
}

func TestEASA_SEP_ExpiringSoon_AllMet(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalHours: 15, PICHours: 8, Landings: 20, InstructorHours: 2,
	}

	expiry := time.Now().AddDate(0, 0, 60) // within 90-day window
	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, ExpiryDate: &expiry, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring (soon but all met)", result.Status)
	}
}

func TestEASA_TMG_Current(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeTMG] = &Progress{
		TotalHours: 14, PICHours: 7, Landings: 15, InstructorHours: 1.5,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeTMG, ExpiryDate: futureDate(12), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current (TMG)", result.Status)
	}
}

func TestEASA_MEP_AllMet(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeMEPLand] = &Progress{
		Flights: 12, InstructorHours: 1.5,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeMEPLand, ExpiryDate: futureDate(6), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "CPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current (MEP)", result.Status)
	}
	if len(result.Requirements) != 2 {
		t.Fatalf("Expected 2 requirements for MEP, got %d", len(result.Requirements))
	}
}

func TestEASA_MEP_InsufficientSectors_Expiring(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeMEPLand] = &Progress{
		Flights: 5, InstructorHours: 1.5,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeMEPLand, ExpiryDate: futureDate(6), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "CPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring", result.Status)
	}
}

func TestEASA_IR_Current(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressAll = &Progress{IFRHours: 15}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeIR, ExpiryDate: futureDate(6), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current (IR)", result.Status)
	}
}

func TestEASA_IR_InsufficientIFR_Expiring(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressAll = &Progress{IFRHours: 5}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeIR, ExpiryDate: futureDate(6), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring (IR)", result.Status)
	}
}

func TestEASA_IR_Expired(t *testing.T) {
	eval := NewEASAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressAll = &Progress{IFRHours: 15}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeIR, ExpiryDate: pastDate(30), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "EASA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpired {
		t.Errorf("Status = %s, want expired (IR)", result.Status)
	}
}

// ── FAA Evaluator Tests ─────────────────────────────────────────────────

func TestFAA_Passenger_AllCurrent(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		Landings: 5, NightLandings: 4, DayLandings: 1, Flights: 5, TotalHours: 8,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current", result.Status)
	}
	if len(result.Requirements) != 2 {
		t.Fatalf("Expected 2 requirements, got %d", len(result.Requirements))
	}
	for _, req := range result.Requirements {
		if !req.Met {
			t.Errorf("Requirement %q should be met", req.Name)
		}
	}
}

func TestFAA_Passenger_DayCurrentNightNot(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		Landings: 4, NightLandings: 1, DayLandings: 3, Flights: 4,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring (day only)", result.Status)
	}
	for _, req := range result.Requirements {
		if req.Name == "Day Passenger Currency" && !req.Met {
			t.Error("Day currency should be met")
		}
		if req.Name == "Night Passenger Currency" && req.Met {
			t.Error("Night currency should NOT be met")
		}
	}
}

func TestFAA_Passenger_NotCurrent(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		Landings: 2, NightLandings: 0, DayLandings: 2, Flights: 2,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpired {
		t.Errorf("Status = %s, want expired", result.Status)
	}
}

func TestFAA_Passenger_ZeroActivity(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeSEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpired {
		t.Errorf("Status = %s, want expired (zero activity)", result.Status)
	}
}

func TestFAA_Passenger_MEP(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressByClass[models.ClassTypeMEPLand] = &Progress{
		Landings: 5, NightLandings: 3, DayLandings: 2, Flights: 5,
	}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeMEPLand, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current (MEP)", result.Status)
	}
}

func TestFAA_IR_Current(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressAll = &Progress{IFRHours: 10}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeIR, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusCurrent {
		t.Errorf("Status = %s, want current (IR)", result.Status)
	}
}

func TestFAA_IR_InsufficientIFR(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressAll = &Progress{IFRHours: 3}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeIR, LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpiring {
		t.Errorf("Status = %s, want expiring (IR)", result.Status)
	}
}

func TestFAA_IR_Expired(t *testing.T) {
	eval := NewFAAEvaluator()
	dp := newMockFlightDataProvider()
	dp.progressAll = &Progress{IFRHours: 10}

	rating := &models.ClassRating{ID: uuid.New(), ClassType: models.ClassTypeIR, ExpiryDate: pastDate(30), LicenseID: uuid.New()}
	license := &models.License{ID: rating.LicenseID, UserID: uuid.New(), RegulatoryAuthority: "FAA", LicenseType: "PPL"}

	result := eval.Evaluate(context.Background(), rating, license, dp)
	if result.Status != StatusExpired {
		t.Errorf("Status = %s, want expired (IR)", result.Status)
	}
}

// ── Integration-style Tests (Service + Real Evaluators) ─────────────────

func TestService_EASA_FAA_MixedSetup(t *testing.T) {
	licRepo := newMockLicenseRepo()
	crRepo := newMockCRRepo()
	dp := newMockFlightDataProvider()

	reg := NewRegistry()
	reg.Register(NewEASAEvaluator())
	reg.Register(NewFAAEvaluator())

	userID := uuid.New()

	// EASA license with SEP rating — good progress
	easaLic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL"}
	licRepo.licenses[easaLic.ID] = easaLic
	crRepo.ratings[easaLic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: easaLic.ID, ClassType: models.ClassTypeSEPLand, ExpiryDate: futureDate(12)},
	}
	dp.progressByClass[models.ClassTypeSEPLand] = &Progress{
		TotalHours: 20, PICHours: 10, Landings: 25, InstructorHours: 2, Flights: 15,
		NightLandings: 5,
	}

	// FAA license with SEP rating — same data (mock shares class key)
	faaLic := &models.License{ID: uuid.New(), UserID: userID, RegulatoryAuthority: "FAA", LicenseType: "PPL"}
	licRepo.licenses[faaLic.ID] = faaLic
	crRepo.ratings[faaLic.ID] = []*models.ClassRating{
		{ID: uuid.New(), LicenseID: faaLic.ID, ClassType: models.ClassTypeSEPLand},
	}

	svc := NewService(reg, licRepo, crRepo, dp)
	result, err := svc.EvaluateAll(context.Background(), userID)
	if err != nil {
		t.Fatalf("EvaluateAll() error = %v", err)
	}
	if len(result.Ratings) != 2 {
		t.Fatalf("Expected 2 ratings, got %d", len(result.Ratings))
	}

	for _, r := range result.Ratings {
		if r.RegulatoryAuthority == "EASA" && r.Status != StatusCurrent {
			t.Errorf("EASA rating status = %s, want current", r.Status)
		}
		if r.RegulatoryAuthority == "FAA" && r.Status != StatusCurrent {
			t.Errorf("FAA rating status = %s, want current", r.Status)
		}
	}
}
