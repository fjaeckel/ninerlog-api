package service

import (
	"context"
	"errors"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrClassRatingNotFound     = errors.New("class rating not found")
	ErrInvalidClassType        = errors.New("invalid class type")
	ErrUnauthorizedClassRating = errors.New("unauthorized access to class rating")
)

// ClassRatingRepository defines the interface for class rating data access
type ClassRatingRepository interface {
	Create(ctx context.Context, cr *models.ClassRating) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.ClassRating, error)
	GetByLicenseID(ctx context.Context, licenseID uuid.UUID) ([]*models.ClassRating, error)
	Update(ctx context.Context, cr *models.ClassRating) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type ClassRatingService struct {
	classRatingRepo ClassRatingRepository
	licenseRepo     repository.LicenseRepository
}

func NewClassRatingService(crRepo ClassRatingRepository, licRepo repository.LicenseRepository) *ClassRatingService {
	return &ClassRatingService{classRatingRepo: crRepo, licenseRepo: licRepo}
}

func (s *ClassRatingService) CreateClassRating(ctx context.Context, cr *models.ClassRating, userID uuid.UUID) error {
	if !models.IsValidClassType(cr.ClassType) {
		return ErrInvalidClassType
	}
	// Verify license ownership
	license, err := s.licenseRepo.GetByID(ctx, cr.LicenseID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrLicenseNotFound
		}
		return err
	}
	if license.UserID != userID {
		return ErrUnauthorizedAccess
	}
	return s.classRatingRepo.Create(ctx, cr)
}

func (s *ClassRatingService) ListClassRatings(ctx context.Context, licenseID, userID uuid.UUID) ([]*models.ClassRating, error) {
	license, err := s.licenseRepo.GetByID(ctx, licenseID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrLicenseNotFound
		}
		return nil, err
	}
	if license.UserID != userID {
		return nil, ErrUnauthorizedAccess
	}
	return s.classRatingRepo.GetByLicenseID(ctx, licenseID)
}

func (s *ClassRatingService) UpdateClassRating(ctx context.Context, cr *models.ClassRating, userID uuid.UUID) error {
	existing, err := s.classRatingRepo.GetByID(ctx, cr.ID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrClassRatingNotFound
		}
		return err
	}
	// Verify license ownership
	license, err := s.licenseRepo.GetByID(ctx, existing.LicenseID)
	if err != nil {
		return err
	}
	if license.UserID != userID {
		return ErrUnauthorizedClassRating
	}
	existing.IssueDate = cr.IssueDate
	existing.ExpiryDate = cr.ExpiryDate
	existing.Notes = cr.Notes
	return s.classRatingRepo.Update(ctx, existing)
}

func (s *ClassRatingService) DeleteClassRating(ctx context.Context, ratingID, userID uuid.UUID) error {
	existing, err := s.classRatingRepo.GetByID(ctx, ratingID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrClassRatingNotFound
		}
		return err
	}
	license, err := s.licenseRepo.GetByID(ctx, existing.LicenseID)
	if err != nil {
		return err
	}
	if license.UserID != userID {
		return ErrUnauthorizedClassRating
	}
	return s.classRatingRepo.Delete(ctx, ratingID)
}
