package service

import (
	"context"
	"errors"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrLicenseNotFound      = errors.New("license not found")
	ErrInvalidLicense       = errors.New("invalid license data")
	ErrLicenseAlreadyExists = errors.New("license already exists")
	ErrUnauthorizedAccess   = errors.New("unauthorized access to license")
)

type LicenseService struct {
	licenseRepo repository.LicenseRepository
}

func NewLicenseService(licenseRepo repository.LicenseRepository) *LicenseService {
	return &LicenseService{
		licenseRepo: licenseRepo,
	}
}

// CreateLicense creates a new license for a user
func (s *LicenseService) CreateLicense(ctx context.Context, license *models.License) error {
	if err := license.Validate(); err != nil {
		return ErrInvalidLicense
	}
	if err := models.ValidateLicenseTextFields(license); err != nil {
		return err
	}

	return s.licenseRepo.Create(ctx, license)
}

// GetLicense retrieves a license by ID and verifies user ownership
func (s *LicenseService) GetLicense(ctx context.Context, licenseID, userID uuid.UUID) (*models.License, error) {
	license, err := s.licenseRepo.GetByID(ctx, licenseID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrLicenseNotFound
		}
		return nil, err
	}

	// Verify ownership
	if license.UserID != userID {
		return nil, ErrUnauthorizedAccess
	}

	return license, nil
}

// ListLicenses retrieves all licenses for a user
func (s *LicenseService) ListLicenses(ctx context.Context, userID uuid.UUID) ([]*models.License, error) {
	return s.licenseRepo.GetByUserID(ctx, userID)
}

// UpdateLicense updates a license and verifies user ownership
func (s *LicenseService) UpdateLicense(ctx context.Context, license *models.License, userID uuid.UUID) error {
	// Verify ownership
	existing, err := s.licenseRepo.GetByID(ctx, license.ID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrLicenseNotFound
		}
		return err
	}

	if existing.UserID != userID {
		return ErrUnauthorizedAccess
	}

	// Only allow updating certain fields
	existing.RegulatoryAuthority = license.RegulatoryAuthority
	existing.LicenseType = license.LicenseType
	existing.LicenseNumber = license.LicenseNumber
	existing.IssuingAuthority = license.IssuingAuthority
	existing.RequiresSeparateLogbook = license.RequiresSeparateLogbook

	if err := models.ValidateLicenseTextFields(existing); err != nil {
		return err
	}

	return s.licenseRepo.Update(ctx, existing)
}

// DeleteLicense deletes a license and verifies user ownership
func (s *LicenseService) DeleteLicense(ctx context.Context, licenseID, userID uuid.UUID) error {
	// Verify ownership
	license, err := s.licenseRepo.GetByID(ctx, licenseID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrLicenseNotFound
		}
		return err
	}

	if license.UserID != userID {
		return ErrUnauthorizedAccess
	}

	return s.licenseRepo.Delete(ctx, licenseID)
}
