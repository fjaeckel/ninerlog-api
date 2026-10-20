package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrLicenseNotFound      = errors.New("license not found")
	ErrInvalidLicense       = errors.New("invalid license data")
	ErrLicenseAlreadyExists = errors.New("license already exists")
	ErrUnauthorizedAccess   = errors.New("unauthorized access to license")
	// ErrInvalidIssuingCountry is returned for an issuing country that is not
	// an ISO 3166-1 alpha-2 code.
	ErrInvalidIssuingCountry = errors.New("issuing country must be a two-letter ISO 3166-1 code")
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
	country, err := NormalizeIssuingCountry(license.IssuingCountry)
	if err != nil {
		return err
	}
	license.IssuingCountry = country

	return s.licenseRepo.Create(ctx, license)
}

// NormalizeIssuingCountry upper-cases an ISO 3166-1 alpha-2 code; a blank
// value clears it.
func NormalizeIssuingCountry(country *string) (*string, error) {
	if country == nil {
		return nil, nil
	}
	code := strings.ToUpper(strings.TrimSpace(*country))
	if code == "" {
		return nil, nil
	}
	if len(code) != 2 || code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' {
		return nil, ErrInvalidIssuingCountry
	}
	return &code, nil
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
	return s.licenseRepo.GetByUserID(ctx, userID, nil)
}

// ListLicensesUpdatedSince retrieves the user's licenses that changed strictly
// after the given instant, for delta-syncing clients.
func (s *LicenseService) ListLicensesUpdatedSince(ctx context.Context, userID uuid.UUID, updatedSince time.Time) ([]*models.License, error) {
	return s.licenseRepo.GetByUserID(ctx, userID, &updatedSince)
}

// UpdateLicense updates a license and verifies user ownership. On success
// license holds the stored values.
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
	country, err := NormalizeIssuingCountry(license.IssuingCountry)
	if err != nil {
		return err
	}
	existing.IssuingCountry = country

	if err := models.ValidateLicenseTextFields(existing); err != nil {
		return err
	}

	if err := s.licenseRepo.Update(ctx, existing); err != nil {
		return err
	}
	*license = *existing
	return nil
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
