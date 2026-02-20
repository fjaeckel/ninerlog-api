package service

import (
	"context"
	"errors"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrContactNotFound     = errors.New("contact not found")
	ErrUnauthorizedContact = errors.New("unauthorized access to contact")
)

type ContactService struct {
	contactRepo repository.ContactRepository
}

func NewContactService(contactRepo repository.ContactRepository) *ContactService {
	return &ContactService{contactRepo: contactRepo}
}

func (s *ContactService) CreateContact(ctx context.Context, contact *models.Contact) error {
	if contact.Name == "" {
		return errors.New("contact name is required")
	}
	if err := models.ValidateContactTextFields(contact); err != nil {
		return err
	}
	return s.contactRepo.Create(ctx, contact)
}

// FindOrCreateContact finds an existing contact by exact name (case-insensitive)
// or creates a new one. Returns the contact and whether it was newly created.
func (s *ContactService) FindOrCreateContact(ctx context.Context, userID uuid.UUID, name string) (*models.Contact, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false, errors.New("contact name is required")
	}
	existing, err := s.contactRepo.GetByExactName(ctx, userID, name)
	if err == nil {
		return existing, false, nil
	}
	// Not found — create
	contact := &models.Contact{
		UserID: userID,
		Name:   name,
	}
	if err := s.contactRepo.Create(ctx, contact); err != nil {
		return nil, false, err
	}
	return contact, true, nil
}

func (s *ContactService) GetContact(ctx context.Context, id, userID uuid.UUID) (*models.Contact, error) {
	contact, err := s.contactRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrContactNotFound
		}
		return nil, err
	}
	if contact.UserID != userID {
		return nil, ErrUnauthorizedContact
	}
	return contact, nil
}

func (s *ContactService) ListContacts(ctx context.Context, userID uuid.UUID) ([]*models.Contact, error) {
	return s.contactRepo.GetByUserID(ctx, userID)
}

func (s *ContactService) SearchContacts(ctx context.Context, userID uuid.UUID, query string, limit int) ([]*models.Contact, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	return s.contactRepo.Search(ctx, userID, query, limit)
}

func (s *ContactService) UpdateContact(ctx context.Context, contact *models.Contact, userID uuid.UUID) error {
	existing, err := s.contactRepo.GetByID(ctx, contact.ID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrContactNotFound
		}
		return err
	}
	if existing.UserID != userID {
		return ErrUnauthorizedContact
	}
	if contact.Name == "" {
		return errors.New("contact name is required")
	}
	if err := models.ValidateContactTextFields(contact); err != nil {
		return err
	}
	return s.contactRepo.Update(ctx, contact)
}

func (s *ContactService) DeleteContact(ctx context.Context, id, userID uuid.UUID) error {
	contact, err := s.contactRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrContactNotFound
		}
		return err
	}
	if contact.UserID != userID {
		return ErrUnauthorizedContact
	}
	return s.contactRepo.Delete(ctx, id)
}
