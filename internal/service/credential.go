package service

import (
	"context"
	"errors"

	"github.com/fjaeckel/pilotlog-api/internal/models"
	"github.com/fjaeckel/pilotlog-api/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrCredentialNotFound     = errors.New("credential not found")
	ErrUnauthorizedCredential = errors.New("unauthorized access to credential")
)

type CredentialService struct {
	credentialRepo repository.CredentialRepository
}

func NewCredentialService(credentialRepo repository.CredentialRepository) *CredentialService {
	return &CredentialService{credentialRepo: credentialRepo}
}

func (s *CredentialService) CreateCredential(ctx context.Context, credential *models.Credential) error {
	return s.credentialRepo.Create(ctx, credential)
}

func (s *CredentialService) GetCredential(ctx context.Context, id, userID uuid.UUID) (*models.Credential, error) {
	credential, err := s.credentialRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrCredentialNotFound
		}
		return nil, err
	}
	if credential.UserID != userID {
		return nil, ErrUnauthorizedCredential
	}
	return credential, nil
}

func (s *CredentialService) ListCredentials(ctx context.Context, userID uuid.UUID) ([]*models.Credential, error) {
	return s.credentialRepo.GetByUserID(ctx, userID)
}

func (s *CredentialService) UpdateCredential(ctx context.Context, credential *models.Credential, userID uuid.UUID) error {
	existing, err := s.credentialRepo.GetByID(ctx, credential.ID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrCredentialNotFound
		}
		return err
	}
	if existing.UserID != userID {
		return ErrUnauthorizedCredential
	}
	return s.credentialRepo.Update(ctx, credential)
}

func (s *CredentialService) DeleteCredential(ctx context.Context, id, userID uuid.UUID) error {
	credential, err := s.credentialRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrCredentialNotFound
		}
		return err
	}
	if credential.UserID != userID {
		return ErrUnauthorizedCredential
	}
	return s.credentialRepo.Delete(ctx, id)
}
