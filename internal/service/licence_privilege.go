package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// ErrLicencePrivilegeNotFound is returned when a privilege does not exist,
// belongs to another user, or is not on the named licence.
var ErrLicencePrivilegeNotFound = errors.New("licence privilege not found")

// LicencePrivilegeInput carries the fields of a new privilege.
type LicencePrivilegeInput struct {
	Kind      models.LicencePrivilegeKind
	Detail    *string
	IssuedOn  *time.Time
	ExpiresOn *time.Time
	Notes     *string
}

// LicencePrivilegePatch carries a partial update. A nil pointer leaves the
// field unchanged; a Clear flag sets a nullable field to NULL.
type LicencePrivilegePatch struct {
	Kind           *models.LicencePrivilegeKind
	Detail         *string
	ClearDetail    bool
	IssuedOn       *time.Time
	ClearIssuedOn  bool
	ExpiresOn      *time.Time
	ClearExpiresOn bool
	Notes          *string
	ClearNotes     bool
}

// LicencePrivilegeService owns licence privilege rules and ownership checks.
type LicencePrivilegeService struct {
	repo        repository.LicencePrivilegeRepository
	licenseRepo repository.LicenseRepository
}

// NewLicencePrivilegeService returns a licence privilege service.
func NewLicencePrivilegeService(repo repository.LicencePrivilegeRepository, licenseRepo repository.LicenseRepository) *LicencePrivilegeService {
	return &LicencePrivilegeService{repo: repo, licenseRepo: licenseRepo}
}

// ownedLicense returns ErrLicenseNotFound when the licence is missing or
// belongs to another user.
func (s *LicencePrivilegeService) ownedLicense(ctx context.Context, licenseID, userID uuid.UUID) error {
	lic, err := s.licenseRepo.GetByID(ctx, licenseID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrLicenseNotFound
		}
		return fmt.Errorf("get license: %w", err)
	}
	if lic.UserID != userID {
		return ErrLicenseNotFound
	}
	return nil
}

// ownedPrivilege returns the privilege when the licence and the privilege
// both belong to the user and the privilege is on that licence.
func (s *LicencePrivilegeService) ownedPrivilege(ctx context.Context, licenseID, privilegeID, userID uuid.UUID) (*models.LicencePrivilege, error) {
	if err := s.ownedLicense(ctx, licenseID, userID); err != nil {
		return nil, err
	}
	p, err := s.repo.GetByID(ctx, privilegeID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrLicencePrivilegeNotFound
		}
		return nil, fmt.Errorf("get licence privilege: %w", err)
	}
	if p.UserID != userID || p.LicenseID != licenseID {
		return nil, ErrLicencePrivilegeNotFound
	}
	return p, nil
}

// List returns the licence's privileges.
func (s *LicencePrivilegeService) List(ctx context.Context, userID, licenseID uuid.UUID) ([]*models.LicencePrivilege, error) {
	if err := s.ownedLicense(ctx, licenseID, userID); err != nil {
		return nil, err
	}
	return s.repo.ListByLicense(ctx, licenseID)
}

// ListAll returns the user's privileges across licences.
func (s *LicencePrivilegeService) ListAll(ctx context.Context, userID uuid.UUID) ([]*models.LicencePrivilege, error) {
	return s.repo.ListByUser(ctx, userID)
}

// Create validates and stores a privilege on one of the user's licences.
func (s *LicencePrivilegeService) Create(ctx context.Context, userID, licenseID uuid.UUID, in LicencePrivilegeInput) (*models.LicencePrivilege, error) {
	if err := s.ownedLicense(ctx, licenseID, userID); err != nil {
		return nil, err
	}
	p := &models.LicencePrivilege{
		UserID:    userID,
		LicenseID: licenseID,
		Kind:      in.Kind,
		Detail:    in.Detail,
		IssuedOn:  in.IssuedOn,
		ExpiresOn: in.ExpiresOn,
		Notes:     in.Notes,
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("create licence privilege: %w", err)
	}
	return p, nil
}

// Update applies a partial update and revalidates the result.
func (s *LicencePrivilegeService) Update(ctx context.Context, userID, licenseID, privilegeID uuid.UUID, patch LicencePrivilegePatch) (*models.LicencePrivilege, error) {
	p, err := s.ownedPrivilege(ctx, licenseID, privilegeID, userID)
	if err != nil {
		return nil, err
	}
	if patch.Kind != nil {
		p.Kind = *patch.Kind
	}
	switch {
	case patch.ClearDetail:
		p.Detail = nil
	case patch.Detail != nil:
		p.Detail = patch.Detail
	}
	switch {
	case patch.ClearIssuedOn:
		p.IssuedOn = nil
	case patch.IssuedOn != nil:
		p.IssuedOn = patch.IssuedOn
	}
	switch {
	case patch.ClearExpiresOn:
		p.ExpiresOn = nil
	case patch.ExpiresOn != nil:
		p.ExpiresOn = patch.ExpiresOn
	}
	switch {
	case patch.ClearNotes:
		p.Notes = nil
	case patch.Notes != nil:
		p.Notes = patch.Notes
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, p); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrLicencePrivilegeNotFound
		}
		return nil, fmt.Errorf("update licence privilege: %w", err)
	}
	return p, nil
}

// Delete removes one of the user's privileges.
func (s *LicencePrivilegeService) Delete(ctx context.Context, userID, licenseID, privilegeID uuid.UUID) error {
	if _, err := s.ownedPrivilege(ctx, licenseID, privilegeID, userID); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, privilegeID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrLicencePrivilegeNotFound
		}
		return fmt.Errorf("delete licence privilege: %w", err)
	}
	return nil
}
