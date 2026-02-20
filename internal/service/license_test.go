package service

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// Mock license repository
type mockLicenseRepo struct {
	licenses map[uuid.UUID]*models.License
}

func newMockLicenseRepo() *mockLicenseRepo {
	return &mockLicenseRepo{
		licenses: make(map[uuid.UUID]*models.License),
	}
}

func (m *mockLicenseRepo) Create(ctx context.Context, license *models.License) error {
	license.ID = uuid.New()
	license.CreatedAt = time.Now()
	license.UpdatedAt = time.Now()
	m.licenses[license.ID] = license
	return nil
}

func (m *mockLicenseRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.License, error) {
	license, exists := m.licenses[id]
	if !exists {
		return nil, repository.ErrNotFound
	}
	return license, nil
}

func (m *mockLicenseRepo) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.License, error) {
	var result []*models.License
	for _, license := range m.licenses {
		if license.UserID == userID {
			result = append(result, license)
		}
	}
	return result, nil
}

func (m *mockLicenseRepo) Update(ctx context.Context, license *models.License) error {
	if _, exists := m.licenses[license.ID]; !exists {
		return repository.ErrNotFound
	}
	license.UpdatedAt = time.Now()
	m.licenses[license.ID] = license
	return nil
}

func (m *mockLicenseRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if _, exists := m.licenses[id]; !exists {
		return repository.ErrNotFound
	}
	delete(m.licenses, id)
	return nil
}

func TestCreateLicense(t *testing.T) {
	repo := newMockLicenseRepo()
	service := NewLicenseService(repo)
	ctx := context.Background()

	userID := uuid.New()
	issueDate := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)

	license := &models.License{
		UserID:              userID,
		RegulatoryAuthority: "EASA",
		LicenseType:         "EASA_PPL",
		LicenseNumber:       "PPL-123456",
		IssueDate:           issueDate,
		IssuingAuthority:    "EASA",
	}

	err := service.CreateLicense(ctx, license)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if license.ID == uuid.Nil {
		t.Error("Expected license ID to be set")
	}
}

func TestCreateInvalidLicense(t *testing.T) {
	repo := newMockLicenseRepo()
	service := NewLicenseService(repo)
	ctx := context.Background()

	// Missing required fields
	license := &models.License{
		LicenseType: "EASA_PPL",
	}

	err := service.CreateLicense(ctx, license)
	if err != ErrInvalidLicense {
		t.Errorf("Expected ErrInvalidLicense, got %v", err)
	}
}

func TestGetLicense(t *testing.T) {
	repo := newMockLicenseRepo()
	service := NewLicenseService(repo)
	ctx := context.Background()

	userID := uuid.New()
	issueDate := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)

	license := &models.License{
		UserID:              userID,
		RegulatoryAuthority: "EASA",
		LicenseType:         "EASA_PPL",
		LicenseNumber:       "PPL-123456",
		IssueDate:           issueDate,
		IssuingAuthority:    "EASA",
	}

	_ = service.CreateLicense(ctx, license)

	retrieved, err := service.GetLicense(ctx, license.ID, userID)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if retrieved.ID != license.ID {
		t.Error("Expected to retrieve the same license")
	}
}

func TestGetLicenseUnauthorized(t *testing.T) {
	repo := newMockLicenseRepo()
	service := NewLicenseService(repo)
	ctx := context.Background()

	userID := uuid.New()
	otherUserID := uuid.New()
	issueDate := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)

	license := &models.License{
		UserID:              userID,
		RegulatoryAuthority: "EASA",
		LicenseType:         "EASA_PPL",
		LicenseNumber:       "PPL-123456",
		IssueDate:           issueDate,
		IssuingAuthority:    "EASA",
	}

	_ = service.CreateLicense(ctx, license)

	_, err := service.GetLicense(ctx, license.ID, otherUserID)
	if err != ErrUnauthorizedAccess {
		t.Errorf("Expected ErrUnauthorizedAccess, got %v", err)
	}
}

func TestListLicenses(t *testing.T) {
	repo := newMockLicenseRepo()
	service := NewLicenseService(repo)
	ctx := context.Background()

	userID := uuid.New()
	issueDate := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)

	// Create two licenses
	license1 := &models.License{
		UserID:              userID,
		RegulatoryAuthority: "EASA",
		LicenseType:         "EASA_PPL",
		LicenseNumber:       "PPL-123456",
		IssueDate:           issueDate,
		IssuingAuthority:    "EASA",
	}
	_ = service.CreateLicense(ctx, license1)

	license2 := &models.License{
		UserID:              userID,
		RegulatoryAuthority: "EASA",
		LicenseType:         "EASA_SPL",
		LicenseNumber:       "SPL-789012",
		IssueDate:           issueDate,
		IssuingAuthority:    "EASA",
	}
	_ = service.CreateLicense(ctx, license2)

	// List all licenses
	licenses, err := service.ListLicenses(ctx, userID)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(licenses) != 2 {
		t.Errorf("Expected 2 licenses, got %d", len(licenses))
	}
}

func TestUpdateLicense(t *testing.T) {
	repo := newMockLicenseRepo()
	service := NewLicenseService(repo)
	ctx := context.Background()

	userID := uuid.New()
	issueDate := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)

	license := &models.License{
		UserID:              userID,
		RegulatoryAuthority: "EASA",
		LicenseType:         "EASA_PPL",
		LicenseNumber:       "PPL-123456",
		IssueDate:           issueDate,
		IssuingAuthority:    "EASA",
	}

	_ = service.CreateLicense(ctx, license)

	// Update license
	license.LicenseNumber = "PPL-999999"
	license.RequiresSeparateLogbook = true

	err := service.UpdateLicense(ctx, license, userID)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify update
	updated, _ := service.GetLicense(ctx, license.ID, userID)
	if updated.LicenseNumber != "PPL-999999" {
		t.Error("Expected license number to be updated")
	}
	if !updated.RequiresSeparateLogbook {
		t.Error("Expected RequiresSeparateLogbook to be true")
	}
}

func TestDeleteLicense(t *testing.T) {
	repo := newMockLicenseRepo()
	service := NewLicenseService(repo)
	ctx := context.Background()

	userID := uuid.New()
	issueDate := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC)

	license := &models.License{
		UserID:              userID,
		RegulatoryAuthority: "EASA",
		LicenseType:         "EASA_PPL",
		LicenseNumber:       "PPL-123456",
		IssueDate:           issueDate,
		IssuingAuthority:    "EASA",
	}

	_ = service.CreateLicense(ctx, license)

	err := service.DeleteLicense(ctx, license.ID, userID)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify deletion
	_, err = service.GetLicense(ctx, license.ID, userID)
	if err != ErrLicenseNotFound {
		t.Errorf("Expected ErrLicenseNotFound, got %v", err)
	}
}
