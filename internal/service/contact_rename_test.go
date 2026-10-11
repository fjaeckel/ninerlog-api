package service

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// renameRepo is a contact repository that hands the flights it "rewrites" to
// the rename's Corrections callback.
type renameRepo struct {
	repository.ContactRepository
	contact  *models.Contact
	affected []*models.Flight

	rename      *repository.CrewRename
	corrections map[uuid.UUID]bool
}

func (r *renameRepo) GetByID(context.Context, uuid.UUID) (*models.Contact, error) {
	c := *r.contact
	return &c, nil
}

func (r *renameRepo) UpdateWithCrewRename(ctx context.Context, c *models.Contact, rename *repository.CrewRename) (int, error) {
	r.rename = rename
	if rename != nil && rename.Corrections != nil {
		got, err := rename.Corrections(ctx, r.affected)
		if err != nil {
			return 0, err
		}
		r.corrections = got
	}
	return len(r.affected), nil
}

func TestContactService_RenameClassifiesAffectedFlights(t *testing.T) {
	ctx := context.Background()

	setup := func(t *testing.T, strict bool) (*correctionEnv, *ContactService, *renameRepo, map[string]*models.Flight) {
		e := newCorrectionEnv(t, strict, "DE")
		fresh := e.seed(t, 0, time.Hour)
		late := e.seed(t, 3, 72*time.Hour)
		voided := e.seed(t, 0, time.Hour)
		certified := e.seed(t, 10, 10*24*time.Hour)
		voidedAt := testNow.Add(-time.Hour)
		e.sigs.byID[uuid.New()] = &models.FlightSignature{FlightID: voided.ID, Status: models.SignatureStatusVoided, VoidedAt: &voidedAt}
		e.lbRepo.certs = append(e.lbRepo.certs, models.LogbookCertification{UserID: e.userID, CertifiedThrough: certified.Date})

		repo := &renameRepo{
			contact:  &models.Contact{ID: uuid.New(), UserID: e.userID, Name: "Hans Müller"},
			affected: []*models.Flight{fresh, late, voided, certified},
		}
		svc := NewContactService(repo)
		svc.SetLogbookService(e.logbook)
		return e, svc, repo, map[string]*models.Flight{"fresh": fresh, "late": late, "voided": voided, "certified": certified}
	}

	t.Run("strict mode flags flights that are corrections under rules 1 to 3", func(t *testing.T) {
		e, svc, repo, f := setup(t, true)
		_, err := svc.UpdateContact(ctx, &models.Contact{ID: repo.contact.ID, Name: "Hans Mueller"}, e.userID)
		if err != nil {
			t.Fatalf("UpdateContact() = %v", err)
		}
		want := map[string]bool{"fresh": false, "late": true, "voided": true, "certified": true}
		for name, isCorrection := range want {
			if got := repo.corrections[f[name].ID]; got != isCorrection {
				t.Errorf("%s: correction = %v, want %v", name, got, isCorrection)
			}
		}
		if repo.rename.Reason != "Crew contact renamed: Hans Müller → Hans Mueller" {
			t.Errorf("reason = %q", repo.rename.Reason)
		}
	})

	t.Run("a rename is never refused for lack of a reason", func(t *testing.T) {
		e, svc, repo, _ := setup(t, true)
		if _, err := svc.UpdateContact(ctx, &models.Contact{ID: repo.contact.ID, Name: "Hans Mueller"}, e.userID); err != nil {
			t.Fatalf("UpdateContact() = %v", err)
		}
	})

	t.Run("non-strict user is not classified", func(t *testing.T) {
		e, svc, repo, _ := setup(t, false)
		if _, err := svc.UpdateContact(ctx, &models.Contact{ID: repo.contact.ID, Name: "Hans Mueller"}, e.userID); err != nil {
			t.Fatalf("UpdateContact() = %v", err)
		}
		if repo.rename.Corrections != nil || repo.corrections != nil {
			t.Error("a non-strict rename was classified")
		}
		if e.licenses.listCalls != 0 {
			t.Errorf("licence list calls = %d, want 0", e.licenses.listCalls)
		}
	})

	t.Run("an unchanged name is not classified", func(t *testing.T) {
		e, svc, repo, _ := setup(t, true)
		if _, err := svc.UpdateContact(ctx, &models.Contact{ID: repo.contact.ID, Name: "Hans Müller"}, e.userID); err != nil {
			t.Fatalf("UpdateContact() = %v", err)
		}
		if repo.rename.Corrections != nil {
			t.Error("an unchanged name was classified")
		}
	})
}
