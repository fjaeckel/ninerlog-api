package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

// The delta-sync list methods must pass the watermark down to the repository
// rather than fetching everything and trimming afterwards — a sync client's
// whole reason for sending updatedSince is to avoid paging the full logbook.
// These tests drive the in-memory repositories, which apply the same
// strictly-after rule as the SQL predicate.

func TestListsUpdatedSince(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	otherUserID := uuid.New()

	old := time.Now().Add(-2 * time.Hour)
	watermark := time.Now().Add(-1 * time.Hour)
	recent := time.Now()

	t.Run("aircraft", func(t *testing.T) {
		repo := newMockAircraftRepo()
		svc := service.NewAircraftService(repo)

		seedAircraft(repo, userID, "D-EAAA", old)
		fresh := seedAircraft(repo, userID, "D-EBBB", recent)
		seedAircraft(repo, otherUserID, "D-ECCC", recent)

		got, err := svc.ListAircraftUpdatedSince(ctx, userID, watermark)
		if err != nil {
			t.Fatalf("ListAircraftUpdatedSince: %v", err)
		}
		if len(got) != 1 || got[0].ID != fresh.ID {
			t.Fatalf("got %d aircraft, want only %s", len(got), fresh.Registration)
		}

		// A record whose updatedAt equals the watermark is excluded: the
		// client already has it, and returning it would loop forever.
		atBoundary, err := svc.ListAircraftUpdatedSince(ctx, userID, fresh.UpdatedAt)
		if err != nil {
			t.Fatalf("boundary list: %v", err)
		}
		if len(atBoundary) != 0 {
			t.Errorf("watermark equal to updatedAt returned %d aircraft, want 0", len(atBoundary))
		}

		all, err := svc.ListAircraft(ctx, userID)
		if err != nil {
			t.Fatalf("ListAircraft: %v", err)
		}
		if len(all) != 2 {
			t.Errorf("unfiltered list returned %d aircraft, want 2", len(all))
		}
	})

	t.Run("contacts", func(t *testing.T) {
		repo := newMockContactRepo()
		svc := service.NewContactService(repo)

		seedContact(repo, userID, "Stale Contact", old)
		fresh := seedContact(repo, userID, "Fresh Contact", recent)
		seedContact(repo, otherUserID, "Other User", recent)

		got, err := svc.ListContactsUpdatedSince(ctx, userID, watermark)
		if err != nil {
			t.Fatalf("ListContactsUpdatedSince: %v", err)
		}
		if len(got) != 1 || got[0].ID != fresh.ID {
			t.Fatalf("got %d contacts, want only %s", len(got), fresh.Name)
		}

		all, err := svc.ListContacts(ctx, userID)
		if err != nil {
			t.Fatalf("ListContacts: %v", err)
		}
		if len(all) != 2 {
			t.Errorf("unfiltered list returned %d contacts, want 2", len(all))
		}
	})

	t.Run("credentials", func(t *testing.T) {
		repo := newMockCredentialRepo()
		svc := service.NewCredentialService(repo)

		seedCredential(repo, userID, models.CredentialTypeEASAClass2Medical, old)
		fresh := seedCredential(repo, userID, models.CredentialTypeLangICAOLevel4, recent)
		seedCredential(repo, otherUserID, models.CredentialTypeLangICAOLevel6, recent)

		got, err := svc.ListCredentialsUpdatedSince(ctx, userID, watermark)
		if err != nil {
			t.Fatalf("ListCredentialsUpdatedSince: %v", err)
		}
		if len(got) != 1 || got[0].ID != fresh.ID {
			t.Fatalf("got %d credentials, want only the fresh one", len(got))
		}

		all, err := svc.ListCredentials(ctx, userID)
		if err != nil {
			t.Fatalf("ListCredentials: %v", err)
		}
		if len(all) != 2 {
			t.Errorf("unfiltered list returned %d credentials, want 2", len(all))
		}
	})
}

func seedAircraft(repo *mockAircraftRepo, userID uuid.UUID, reg string, updatedAt time.Time) *models.Aircraft {
	a := &models.Aircraft{
		ID: uuid.New(), UserID: userID, Registration: reg, Type: "C172",
		Make: "Cessna", Model: "172S", IsActive: true,
		CreatedAt: updatedAt, UpdatedAt: updatedAt,
	}
	repo.aircraft[a.ID] = a
	return a
}

func seedContact(repo *mockContactRepo, userID uuid.UUID, name string, updatedAt time.Time) *models.Contact {
	c := &models.Contact{
		ID: uuid.New(), UserID: userID, Name: name,
		CreatedAt: updatedAt, UpdatedAt: updatedAt,
	}
	repo.contacts[c.ID] = c
	return c
}

func seedCredential(repo *mockCredentialRepo, userID uuid.UUID, credType models.CredentialType, updatedAt time.Time) *models.Credential {
	c := &models.Credential{
		ID: uuid.New(), UserID: userID, CredentialType: credType,
		IssueDate: updatedAt.AddDate(-1, 0, 0), IssuingAuthority: "LBA",
		CreatedAt: updatedAt, UpdatedAt: updatedAt,
	}
	repo.credentials[c.ID] = c
	return c
}
