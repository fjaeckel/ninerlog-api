package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

type mockPrivilegeRepo struct {
	rows map[uuid.UUID]*models.LicencePrivilege
}

func (m *mockPrivilegeRepo) Create(_ context.Context, p *models.LicencePrivilege) error {
	p.ID = uuid.New()
	cp := *p
	m.rows[p.ID] = &cp
	return nil
}

func (m *mockPrivilegeRepo) GetByID(_ context.Context, id uuid.UUID) (*models.LicencePrivilege, error) {
	p, ok := m.rows[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (m *mockPrivilegeRepo) ListByLicense(_ context.Context, licenseID uuid.UUID) ([]*models.LicencePrivilege, error) {
	out := []*models.LicencePrivilege{}
	for _, p := range m.rows {
		if p.LicenseID == licenseID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *mockPrivilegeRepo) ListByUser(_ context.Context, userID uuid.UUID) ([]*models.LicencePrivilege, error) {
	out := []*models.LicencePrivilege{}
	for _, p := range m.rows {
		if p.UserID == userID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *mockPrivilegeRepo) Update(_ context.Context, p *models.LicencePrivilege) error {
	if _, ok := m.rows[p.ID]; !ok {
		return repository.ErrNotFound
	}
	cp := *p
	m.rows[p.ID] = &cp
	return nil
}

func (m *mockPrivilegeRepo) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.rows[id]; !ok {
		return repository.ErrNotFound
	}
	delete(m.rows, id)
	return nil
}

func newPrivilegeFixture() (*LicencePrivilegeService, *mockPrivilegeRepo, uuid.UUID, uuid.UUID, uuid.UUID) {
	licRepo := newMockLicenseRepo()
	petra, other := uuid.New(), uuid.New()
	ppl := &models.License{UserID: petra, RegulatoryAuthority: "EASA", LicenseType: "PPL(A)"}
	_ = licRepo.Create(context.Background(), ppl)
	repo := &mockPrivilegeRepo{rows: map[uuid.UUID]*models.LicencePrivilege{}}
	return NewLicencePrivilegeService(repo, licRepo), repo, petra, other, ppl.ID
}

func TestLicencePrivilegeService_Create(t *testing.T) {
	ctx := context.Background()
	svc, _, petra, other, ppl := newPrivilegeFixture()
	str := func(s string) *string { return &s }
	tests := []struct {
		name    string
		userID  uuid.UUID
		license uuid.UUID
		in      LicencePrivilegeInput
		wantErr error
	}{
		{name: "P job 3 Petra records sailplane towing on her PPL", userID: petra, license: ppl, in: LicencePrivilegeInput{Kind: models.PrivilegeSailplaneTowing}},
		{name: "launch method trained with a method", userID: petra, license: ppl, in: LicencePrivilegeInput{Kind: models.PrivilegeLaunchMethodTrained, Detail: str("winch")}},
		{name: "launch method trained without a method", userID: petra, license: ppl, in: LicencePrivilegeInput{Kind: models.PrivilegeLaunchMethodTrained}, wantErr: models.ErrInvalidLicencePrivilege},
		{name: "unknown kind", userID: petra, license: ppl, in: LicencePrivilegeInput{Kind: "WINGWALKING"}, wantErr: models.ErrInvalidLicencePrivilege},
		{name: "another user's licence is not found", userID: other, license: ppl, in: LicencePrivilegeInput{Kind: models.PrivilegeCloudFlying}, wantErr: ErrLicenseNotFound},
		{name: "missing licence is not found", userID: petra, license: uuid.New(), in: LicencePrivilegeInput{Kind: models.PrivilegeCloudFlying}, wantErr: ErrLicenseNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := svc.Create(ctx, tc.userID, tc.license, tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Create() error = %v, want %v", err, tc.wantErr)
			}
			if err == nil && (p.UserID != petra || p.LicenseID != ppl || p.ID == uuid.Nil) {
				t.Errorf("unexpected privilege %+v", p)
			}
		})
	}
}

func TestLicencePrivilegeService_UpdateDeleteOwnership(t *testing.T) {
	ctx := context.Background()
	svc, repo, petra, other, ppl := newPrivilegeFixture()
	exp := time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC)
	fi, err := svc.Create(ctx, petra, ppl, LicencePrivilegeInput{Kind: models.PrivilegeFIS, ExpiresOn: &exp})
	if err != nil {
		t.Fatal(err)
	}
	otherLic := uuid.New()

	t.Run("patch sets notes and clears expiry", func(t *testing.T) {
		n := "refresher booked"
		p, err := svc.Update(ctx, petra, ppl, fi.ID, LicencePrivilegePatch{Notes: &n, ClearExpiresOn: true})
		if err != nil {
			t.Fatal(err)
		}
		if p.ExpiresOn != nil || p.Notes == nil || *p.Notes != n {
			t.Errorf("unexpected privilege after patch: %+v", p)
		}
	})
	t.Run("patch to launch method without detail is rejected", func(t *testing.T) {
		k := models.PrivilegeLaunchMethodTrained
		if _, err := svc.Update(ctx, petra, ppl, fi.ID, LicencePrivilegePatch{Kind: &k}); !errors.Is(err, models.ErrInvalidLicencePrivilege) {
			t.Errorf("error = %v, want invalid", err)
		}
		if repo.rows[fi.ID].Kind != models.PrivilegeFIS {
			t.Error("rejected patch was stored")
		}
	})
	ownership := []struct {
		name    string
		userID  uuid.UUID
		license uuid.UUID
		id      uuid.UUID
		wantErr error
	}{
		{name: "another user is not found", userID: other, license: ppl, id: fi.ID, wantErr: ErrLicenseNotFound},
		{name: "wrong licence is not found", userID: petra, license: otherLic, id: fi.ID, wantErr: ErrLicenseNotFound},
		{name: "unknown privilege is not found", userID: petra, license: ppl, id: uuid.New(), wantErr: ErrLicencePrivilegeNotFound},
	}
	for _, tc := range ownership {
		t.Run("update: "+tc.name, func(t *testing.T) {
			if _, err := svc.Update(ctx, tc.userID, tc.license, tc.id, LicencePrivilegePatch{}); !errors.Is(err, tc.wantErr) {
				t.Errorf("error = %v, want %v", err, tc.wantErr)
			}
		})
		t.Run("delete: "+tc.name, func(t *testing.T) {
			if err := svc.Delete(ctx, tc.userID, tc.license, tc.id); !errors.Is(err, tc.wantErr) {
				t.Errorf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
	t.Run("list for another user is not found", func(t *testing.T) {
		if _, err := svc.List(ctx, other, ppl); !errors.Is(err, ErrLicenseNotFound) {
			t.Errorf("error = %v, want not found", err)
		}
	})
	t.Run("privilege on another licence of the same user is not found", func(t *testing.T) {
		lic2 := &models.License{UserID: petra}
		_ = svc.licenseRepo.Create(ctx, lic2)
		if err := svc.Delete(ctx, petra, lic2.ID, fi.ID); !errors.Is(err, ErrLicencePrivilegeNotFound) {
			t.Errorf("error = %v, want not found", err)
		}
	})
	t.Run("owner deletes", func(t *testing.T) {
		if err := svc.Delete(ctx, petra, ppl, fi.ID); err != nil {
			t.Fatal(err)
		}
		list, _ := svc.ListAll(ctx, petra)
		if len(list) != 0 {
			t.Errorf("%d privileges left, want 0", len(list))
		}
	})
}
