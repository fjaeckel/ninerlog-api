package service

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// License delta-sync coverage sits beside the unexported license mock.
func TestListLicensesUpdatedSince(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	otherUserID := uuid.New()

	old := time.Now().Add(-2 * time.Hour)
	watermark := time.Now().Add(-1 * time.Hour)
	recent := time.Now()

	repo := newMockLicenseRepo()
	svc := NewLicenseService(repo)

	seedLicense(repo, userID, "DE-PPL-1", old)
	fresh := seedLicense(repo, userID, "DE-CPL-1", recent)
	seedLicense(repo, otherUserID, "US-PPL-1", recent)

	got, err := svc.ListLicensesUpdatedSince(ctx, userID, watermark)
	if err != nil {
		t.Fatalf("ListLicensesUpdatedSince: %v", err)
	}
	if len(got) != 1 || got[0].ID != fresh.ID {
		t.Fatalf("got %d licenses, want only %s", len(got), fresh.LicenseNumber)
	}

	// A licence whose updatedAt equals the watermark is excluded.
	atBoundary, err := svc.ListLicensesUpdatedSince(ctx, userID, fresh.UpdatedAt)
	if err != nil {
		t.Fatalf("boundary list: %v", err)
	}
	if len(atBoundary) != 0 {
		t.Errorf("watermark equal to updatedAt returned %d licenses, want 0", len(atBoundary))
	}

	all, err := svc.ListLicenses(ctx, userID)
	if err != nil {
		t.Fatalf("ListLicenses: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("unfiltered list returned %d licenses, want 2", len(all))
	}
}

func seedLicense(repo *mockLicenseRepo, userID uuid.UUID, number string, updatedAt time.Time) *models.License {
	l := &models.License{
		ID: uuid.New(), UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL",
		LicenseNumber: number, IssueDate: updatedAt.AddDate(-1, 0, 0), IssuingAuthority: "LBA",
		CreatedAt: updatedAt, UpdatedAt: updatedAt,
	}
	repo.licenses[l.ID] = l
	return l
}
