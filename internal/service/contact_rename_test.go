package service

import (
	"context"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// renameRepo is a contact repository that records the reason of a rename.
type renameRepo struct {
	repository.ContactRepository
	contact *models.Contact

	calls  int
	reason string
}

func (r *renameRepo) GetByID(context.Context, uuid.UUID) (*models.Contact, error) {
	c := *r.contact
	return &c, nil
}

func (r *renameRepo) UpdateWithCrewRename(_ context.Context, _ *models.Contact, reason string) (int, error) {
	r.calls++
	r.reason = reason
	return 0, nil
}

func TestContactService_RenameReason(t *testing.T) {
	tests := []struct {
		name       string
		newName    string
		wantReason string
	}{
		{"rename names both spellings", "Hans Mueller", "Crew contact renamed: Hans Müller → Hans Mueller"},
		{"unchanged name resyncs crew", "Hans Müller", "Crew names updated to contact: Hans Müller"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := uuid.New()
			repo := &renameRepo{contact: &models.Contact{ID: uuid.New(), UserID: userID, Name: "Hans Müller"}}
			svc := NewContactService(repo)

			if _, err := svc.UpdateContact(context.Background(), &models.Contact{ID: repo.contact.ID, Name: tt.newName}, userID); err != nil {
				t.Fatalf("UpdateContact() = %v", err)
			}
			if repo.calls != 1 || repo.reason != tt.wantReason {
				t.Errorf("calls = %d, reason = %q, want %q", repo.calls, repo.reason, tt.wantReason)
			}
		})
	}
}
