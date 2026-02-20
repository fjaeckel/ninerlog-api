package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

// Mock aircraft repository
type mockAircraftRepo struct {
	aircraft map[uuid.UUID]*models.Aircraft
}

func newMockAircraftRepo() *mockAircraftRepo {
	return &mockAircraftRepo{
		aircraft: make(map[uuid.UUID]*models.Aircraft),
	}
}

func (m *mockAircraftRepo) Create(ctx context.Context, aircraft *models.Aircraft) error {
	// Check for duplicate registration per user
	for _, a := range m.aircraft {
		if a.UserID == aircraft.UserID && a.Registration == aircraft.Registration {
			return repository.ErrDuplicateRegistration
		}
	}
	aircraft.ID = uuid.New()
	aircraft.CreatedAt = time.Now()
	aircraft.UpdatedAt = time.Now()
	m.aircraft[aircraft.ID] = aircraft
	return nil
}

func (m *mockAircraftRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Aircraft, error) {
	a, exists := m.aircraft[id]
	if !exists {
		return nil, repository.ErrNotFound
	}
	return a, nil
}

func (m *mockAircraftRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.Aircraft, error) {
	var result []*models.Aircraft
	for _, a := range m.aircraft {
		if a.UserID == userID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (m *mockAircraftRepo) Update(ctx context.Context, aircraft *models.Aircraft) error {
	if _, exists := m.aircraft[aircraft.ID]; !exists {
		return repository.ErrNotFound
	}
	// Check for duplicate registration per user (excluding self)
	for _, a := range m.aircraft {
		if a.ID != aircraft.ID && a.UserID == aircraft.UserID && a.Registration == aircraft.Registration {
			return repository.ErrDuplicateRegistration
		}
	}
	aircraft.UpdatedAt = time.Now()
	m.aircraft[aircraft.ID] = aircraft
	return nil
}

func (m *mockAircraftRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if _, exists := m.aircraft[id]; !exists {
		return repository.ErrNotFound
	}
	delete(m.aircraft, id)
	return nil
}

func (m *mockAircraftRepo) CountByUserID(ctx context.Context, userID uuid.UUID) (int, error) {
	count := 0
	for _, a := range m.aircraft {
		if a.UserID == userID {
			count++
		}
	}
	return count, nil
}

func setupAircraftService() *service.AircraftService {
	return service.NewAircraftService(newMockAircraftRepo())
}

func TestCreateAircraft(t *testing.T) {
	svc := setupAircraftService()
	ctx := context.Background()
	userID := uuid.New()

	aircraft := &models.Aircraft{
		UserID:       userID,
		Registration: "D-EFGH",
		Type:         "C172",
		Make:         "Cessna",
		Model:        "172 Skyhawk",
		IsActive:     true,
	}

	err := svc.CreateAircraft(ctx, aircraft)
	if err != nil {
		t.Fatalf("CreateAircraft failed: %v", err)
	}

	if aircraft.ID == uuid.Nil {
		t.Error("Expected aircraft ID to be set")
	}
	if aircraft.Registration != "D-EFGH" {
		t.Errorf("Expected registration D-EFGH, got %s", aircraft.Registration)
	}
}

func TestCreateAircraftValidation(t *testing.T) {
	svc := setupAircraftService()
	ctx := context.Background()
	userID := uuid.New()

	// Missing registration
	aircraft := &models.Aircraft{
		UserID: userID,
		Type:   "C172",
		Make:   "Cessna",
		Model:  "172",
	}
	err := svc.CreateAircraft(ctx, aircraft)
	if err == nil {
		t.Error("Expected validation error for missing registration")
	}

	// Missing type
	aircraft = &models.Aircraft{
		UserID:       userID,
		Registration: "D-EFGH",
		Make:         "Cessna",
		Model:        "172",
	}
	err = svc.CreateAircraft(ctx, aircraft)
	if err == nil {
		t.Error("Expected validation error for missing type")
	}

	// Missing make
	aircraft = &models.Aircraft{
		UserID:       userID,
		Registration: "D-EFGH",
		Type:         "C172",
		Model:        "172",
	}
	err = svc.CreateAircraft(ctx, aircraft)
	if err == nil {
		t.Error("Expected validation error for missing make")
	}

	// Missing model
	aircraft = &models.Aircraft{
		UserID:       userID,
		Registration: "D-EFGH",
		Type:         "C172",
		Make:         "Cessna",
	}
	err = svc.CreateAircraft(ctx, aircraft)
	if err == nil {
		t.Error("Expected validation error for missing model")
	}
}

func TestCreateAircraftDuplicateRegistration(t *testing.T) {
	svc := setupAircraftService()
	ctx := context.Background()
	userID := uuid.New()

	aircraft1 := &models.Aircraft{
		UserID:       userID,
		Registration: "D-EFGH",
		Type:         "C172",
		Make:         "Cessna",
		Model:        "172 Skyhawk",
		IsActive:     true,
	}
	err := svc.CreateAircraft(ctx, aircraft1)
	if err != nil {
		t.Fatalf("First create failed: %v", err)
	}

	aircraft2 := &models.Aircraft{
		UserID:       userID,
		Registration: "D-EFGH",
		Type:         "PA28",
		Make:         "Piper",
		Model:        "Cherokee",
		IsActive:     true,
	}
	err = svc.CreateAircraft(ctx, aircraft2)
	if err != service.ErrDuplicateRegistration {
		t.Errorf("Expected ErrDuplicateRegistration, got %v", err)
	}
}

func TestCreateAircraftSameRegDifferentUser(t *testing.T) {
	svc := setupAircraftService()
	ctx := context.Background()

	aircraft1 := &models.Aircraft{
		UserID:       uuid.New(),
		Registration: "D-EFGH",
		Type:         "C172",
		Make:         "Cessna",
		Model:        "172",
		IsActive:     true,
	}
	err := svc.CreateAircraft(ctx, aircraft1)
	if err != nil {
		t.Fatalf("First create failed: %v", err)
	}

	aircraft2 := &models.Aircraft{
		UserID:       uuid.New(),
		Registration: "D-EFGH",
		Type:         "C172",
		Make:         "Cessna",
		Model:        "172",
		IsActive:     true,
	}
	err = svc.CreateAircraft(ctx, aircraft2)
	if err != nil {
		t.Errorf("Should allow same registration for different user, got %v", err)
	}
}

func TestGetAircraft(t *testing.T) {
	svc := setupAircraftService()
	ctx := context.Background()
	userID := uuid.New()

	aircraft := &models.Aircraft{
		UserID:       userID,
		Registration: "D-EFGH",
		Type:         "C172",
		Make:         "Cessna",
		Model:        "172 Skyhawk",
		IsActive:     true,
	}
	_ = svc.CreateAircraft(ctx, aircraft)

	result, err := svc.GetAircraft(ctx, aircraft.ID, userID)
	if err != nil {
		t.Fatalf("GetAircraft failed: %v", err)
	}
	if result.Registration != "D-EFGH" {
		t.Errorf("Expected D-EFGH, got %s", result.Registration)
	}
}

func TestGetAircraftNotFound(t *testing.T) {
	svc := setupAircraftService()
	ctx := context.Background()

	_, err := svc.GetAircraft(ctx, uuid.New(), uuid.New())
	if err != service.ErrAircraftNotFound {
		t.Errorf("Expected ErrAircraftNotFound, got %v", err)
	}
}

func TestGetAircraftUnauthorized(t *testing.T) {
	svc := setupAircraftService()
	ctx := context.Background()
	ownerID := uuid.New()
	otherID := uuid.New()

	aircraft := &models.Aircraft{
		UserID:       ownerID,
		Registration: "D-EFGH",
		Type:         "C172",
		Make:         "Cessna",
		Model:        "172",
		IsActive:     true,
	}
	_ = svc.CreateAircraft(ctx, aircraft)

	_, err := svc.GetAircraft(ctx, aircraft.ID, otherID)
	if err != service.ErrUnauthorizedAircraft {
		t.Errorf("Expected ErrUnauthorizedAircraft, got %v", err)
	}
}

func TestListAircraft(t *testing.T) {
	svc := setupAircraftService()
	ctx := context.Background()
	userID := uuid.New()
	otherID := uuid.New()

	_ = svc.CreateAircraft(ctx, &models.Aircraft{
		UserID: userID, Registration: "D-AAAA", Type: "C172", Make: "Cessna", Model: "172", IsActive: true,
	})
	_ = svc.CreateAircraft(ctx, &models.Aircraft{
		UserID: userID, Registration: "D-BBBB", Type: "PA28", Make: "Piper", Model: "Cherokee", IsActive: true,
	})
	_ = svc.CreateAircraft(ctx, &models.Aircraft{
		UserID: otherID, Registration: "D-CCCC", Type: "C152", Make: "Cessna", Model: "152", IsActive: true,
	})

	list, err := svc.ListAircraft(ctx, userID)
	if err != nil {
		t.Fatalf("ListAircraft failed: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("Expected 2 aircraft for user, got %d", len(list))
	}
}

func TestUpdateAircraft(t *testing.T) {
	svc := setupAircraftService()
	ctx := context.Background()
	userID := uuid.New()

	aircraft := &models.Aircraft{
		UserID:       userID,
		Registration: "D-EFGH",
		Type:         "C172",
		Make:         "Cessna",
		Model:        "172 Skyhawk",
		IsActive:     true,
	}
	_ = svc.CreateAircraft(ctx, aircraft)

	aircraft.Model = "172S Skyhawk SP"
	aircraft.IsComplex = true
	err := svc.UpdateAircraft(ctx, aircraft, userID)
	if err != nil {
		t.Fatalf("UpdateAircraft failed: %v", err)
	}

	updated, _ := svc.GetAircraft(ctx, aircraft.ID, userID)
	if updated.Model != "172S Skyhawk SP" {
		t.Errorf("Expected updated model, got %s", updated.Model)
	}
	if !updated.IsComplex {
		t.Error("Expected isComplex to be true")
	}
}

func TestUpdateAircraftUnauthorized(t *testing.T) {
	svc := setupAircraftService()
	ctx := context.Background()
	ownerID := uuid.New()
	otherID := uuid.New()

	aircraft := &models.Aircraft{
		UserID:       ownerID,
		Registration: "D-EFGH",
		Type:         "C172",
		Make:         "Cessna",
		Model:        "172",
		IsActive:     true,
	}
	_ = svc.CreateAircraft(ctx, aircraft)

	aircraft.Model = "Hacked"
	err := svc.UpdateAircraft(ctx, aircraft, otherID)
	if err != service.ErrUnauthorizedAircraft {
		t.Errorf("Expected ErrUnauthorizedAircraft, got %v", err)
	}
}

func TestDeleteAircraft(t *testing.T) {
	svc := setupAircraftService()
	ctx := context.Background()
	userID := uuid.New()

	aircraft := &models.Aircraft{
		UserID:       userID,
		Registration: "D-EFGH",
		Type:         "C172",
		Make:         "Cessna",
		Model:        "172",
		IsActive:     true,
	}
	_ = svc.CreateAircraft(ctx, aircraft)

	err := svc.DeleteAircraft(ctx, aircraft.ID, userID)
	if err != nil {
		t.Fatalf("DeleteAircraft failed: %v", err)
	}

	_, err = svc.GetAircraft(ctx, aircraft.ID, userID)
	if err != service.ErrAircraftNotFound {
		t.Errorf("Expected ErrAircraftNotFound after delete, got %v", err)
	}
}

func TestDeleteAircraftNotFound(t *testing.T) {
	svc := setupAircraftService()
	ctx := context.Background()

	err := svc.DeleteAircraft(ctx, uuid.New(), uuid.New())
	if err != service.ErrAircraftNotFound {
		t.Errorf("Expected ErrAircraftNotFound, got %v", err)
	}
}

func TestDeleteAircraftUnauthorized(t *testing.T) {
	svc := setupAircraftService()
	ctx := context.Background()
	ownerID := uuid.New()
	otherID := uuid.New()

	aircraft := &models.Aircraft{
		UserID:       ownerID,
		Registration: "D-EFGH",
		Type:         "C172",
		Make:         "Cessna",
		Model:        "172",
		IsActive:     true,
	}
	_ = svc.CreateAircraft(ctx, aircraft)

	err := svc.DeleteAircraft(ctx, aircraft.ID, otherID)
	if err != service.ErrUnauthorizedAircraft {
		t.Errorf("Expected ErrUnauthorizedAircraft, got %v", err)
	}
}

func TestCountAircraft(t *testing.T) {
	svc := setupAircraftService()
	ctx := context.Background()
	userID := uuid.New()

	count, _ := svc.CountAircraft(ctx, userID)
	if count != 0 {
		t.Errorf("Expected 0, got %d", count)
	}

	_ = svc.CreateAircraft(ctx, &models.Aircraft{
		UserID: userID, Registration: "D-AAAA", Type: "C172", Make: "Cessna", Model: "172", IsActive: true,
	})
	_ = svc.CreateAircraft(ctx, &models.Aircraft{
		UserID: userID, Registration: "D-BBBB", Type: "PA28", Make: "Piper", Model: "Cherokee", IsActive: true,
	})

	count, _ = svc.CountAircraft(ctx, userID)
	if count != 2 {
		t.Errorf("Expected 2, got %d", count)
	}
}

func TestCreateAircraftWithAircraftClass(t *testing.T) {
	svc := setupAircraftService()
	ctx := context.Background()
	userID := uuid.New()

	sepLand := "SEP_LAND"
	aircraft := &models.Aircraft{
		UserID:        userID,
		Registration:  "D-EFGH",
		Type:          "C172",
		Make:          "Cessna",
		Model:         "172",
		AircraftClass: &sepLand,
		IsActive:      true,
	}

	err := svc.CreateAircraft(ctx, aircraft)
	if err != nil {
		t.Fatalf("CreateAircraft with aircraft class failed: %v", err)
	}

	result, _ := svc.GetAircraft(ctx, aircraft.ID, userID)
	if result.AircraftClass == nil || *result.AircraftClass != "SEP_LAND" {
		t.Error("Expected aircraft class SEP_LAND")
	}
}

func TestCreateAircraftWithCustomClass(t *testing.T) {
	svc := setupAircraftService()
	ctx := context.Background()
	userID := uuid.New()

	customClass := "ULTRALIGHT"
	aircraft := &models.Aircraft{
		UserID:        userID,
		Registration:  "D-ULXX",
		Type:          "WT9",
		Make:          "Aerospool",
		Model:         "Dynamic",
		AircraftClass: &customClass,
		IsActive:      true,
	}

	err := svc.CreateAircraft(ctx, aircraft)
	if err != nil {
		t.Fatalf("CreateAircraft with custom class failed: %v", err)
	}

	result, _ := svc.GetAircraft(ctx, aircraft.ID, userID)
	if result.AircraftClass == nil || *result.AircraftClass != "ULTRALIGHT" {
		t.Error("Expected aircraft class ULTRALIGHT")
	}
}
