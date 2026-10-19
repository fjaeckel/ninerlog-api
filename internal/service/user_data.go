package service

import (
	"context"

	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// UserDataService wipes a user's data while keeping the account.
type UserDataService struct {
	content repository.UserContentRepository
}

// NewUserDataService creates a UserDataService.
func NewUserDataService(content repository.UserContentRepository) *UserDataService {
	return &UserDataService{content: content}
}

// DeleteAll removes the user's logbook content, contacts, credentials,
// licences, aircraft, import history and notification history in one
// transaction.
func (s *UserDataService) DeleteAll(ctx context.Context, userID uuid.UUID) error {
	return s.content.DeleteAllContent(ctx, userID)
}

// DeleteFlights removes the user's flights, their crew members and the import
// history in one transaction, returning the number of flights deleted.
func (s *UserDataService) DeleteFlights(ctx context.Context, userID uuid.UUID) (int64, error) {
	return s.content.DeleteFlightsAndHistory(ctx, userID)
}
