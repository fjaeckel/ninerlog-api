package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

// fakeAdminRepo flips IsAdmin on the users held by a mockUserRepo, mirroring
// the guards of the postgres implementation.
type fakeAdminRepo struct {
	repository.AdminRepository
	users *mockUserRepo
	audit []*repository.AdminAuditLogEntry
}

func (f *fakeAdminRepo) byID(id uuid.UUID) *models.User {
	for _, u := range f.users.users {
		if u.ID == id {
			return u
		}
	}
	return nil
}

func (f *fakeAdminRepo) SetAdmin(_ context.Context, id uuid.UUID) error {
	u := f.byID(id)
	if u == nil {
		return repository.ErrNotFound
	}
	u.IsAdmin = true
	return nil
}

func (f *fakeAdminRepo) RevokeAdmin(_ context.Context, id uuid.UUID) (bool, error) {
	u := f.byID(id)
	if u == nil || !u.IsAdmin {
		return false, nil
	}
	for _, o := range f.users.users {
		if o.ID != id && o.IsAdmin && o.EmailVerified && !o.Disabled {
			u.IsAdmin = false
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeAdminRepo) PromoteVerifiedByEmail(_ context.Context, email string) (uuid.UUID, error) {
	for _, u := range f.users.users {
		if strings.EqualFold(u.Email, email) && u.EmailVerified && !u.IsAdmin {
			u.IsAdmin = true
			return u.ID, nil
		}
	}
	return uuid.Nil, nil
}

func (f *fakeAdminRepo) InsertAuditLog(_ context.Context, e *repository.AdminAuditLogEntry) error {
	f.audit = append(f.audit, e)
	return nil
}

func newAdminRoleFixture(t *testing.T, bootstrap string) (*service.AdminRoleService, *mockUserRepo, *fakeAdminRepo) {
	t.Helper()
	users := newMockUserRepo()
	admins := &fakeAdminRepo{users: users}
	return service.NewAdminRoleService(users, admins, bootstrap), users, admins
}

func addUser(t *testing.T, users *mockUserRepo, email string, isAdmin bool) *models.User {
	t.Helper()
	u := &models.User{Email: email, Name: email}
	if err := users.Create(context.Background(), u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	u.IsAdmin = isAdmin
	return u
}

func TestIsAdmin(t *testing.T) {
	tests := []struct {
		name      string
		user      *models.User
		bootstrap string
		want      bool
	}{
		{"nil user", nil, "a@x.com", false},
		{"flagged verified", &models.User{Email: "b@x.com", EmailVerified: true, IsAdmin: true}, "", true},
		{"flagged unverified", &models.User{Email: "b@x.com", IsAdmin: true}, "", false},
		{"bootstrap match case-insensitive", &models.User{Email: "A@X.com", EmailVerified: true}, "a@x.COM", true},
		{"bootstrap match unverified", &models.User{Email: "a@x.com"}, "a@x.com", false},
		{"no flag no match", &models.User{Email: "c@x.com", EmailVerified: true}, "a@x.com", false},
		{"no flag no bootstrap", &models.User{Email: "c@x.com", EmailVerified: true}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.IsAdmin(tt.user, tt.bootstrap); got != tt.want {
				t.Errorf("IsAdmin = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAdminRole_Grant(t *testing.T) {
	ctx := context.Background()
	svc, users, _ := newAdminRoleFixture(t, "")

	target := addUser(t, users, "pilot@x.com", false)
	if _, err := svc.Grant(ctx, target.ID); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if !target.IsAdmin {
		t.Error("flag not set")
	}
	if _, err := svc.Grant(ctx, target.ID); err != nil {
		t.Errorf("second Grant should be idempotent, got %v", err)
	}

	unverified := addUser(t, users, "new@x.com", false)
	unverified.EmailVerified = false
	if _, err := svc.Grant(ctx, unverified.ID); !errors.Is(err, service.ErrAdminTargetUnverified) {
		t.Errorf("unverified: got %v", err)
	}

	disabled := addUser(t, users, "off@x.com", false)
	disabled.Disabled = true
	if _, err := svc.Grant(ctx, disabled.ID); !errors.Is(err, service.ErrAdminTargetDisabled) {
		t.Errorf("disabled: got %v", err)
	}

	if _, err := svc.Grant(ctx, uuid.New()); !errors.Is(err, service.ErrAdminTargetNotFound) {
		t.Errorf("missing: got %v", err)
	}
}

func TestAdminRole_Revoke(t *testing.T) {
	ctx := context.Background()
	svc, users, _ := newAdminRoleFixture(t, "boot@x.com")

	a := addUser(t, users, "a@x.com", true)
	b := addUser(t, users, "b@x.com", true)

	if _, err := svc.Revoke(ctx, a.ID); err != nil {
		t.Fatalf("Revoke with another admin left: %v", err)
	}
	if a.IsAdmin {
		t.Error("flag not cleared")
	}
	if _, err := svc.Revoke(ctx, a.ID); err != nil {
		t.Errorf("revoking a non-admin should be idempotent, got %v", err)
	}
	if _, err := svc.Revoke(ctx, b.ID); !errors.Is(err, service.ErrLastAdmin) {
		t.Errorf("last admin: got %v", err)
	}
	if !b.IsAdmin {
		t.Error("last admin lost the flag")
	}

	// A disabled admin does not count as a remaining admin.
	c := addUser(t, users, "c@x.com", true)
	c.Disabled = true
	if _, err := svc.Revoke(ctx, b.ID); !errors.Is(err, service.ErrLastAdmin) {
		t.Errorf("only a disabled admin left: got %v", err)
	}

	boot := addUser(t, users, "Boot@X.com", true)
	if _, err := svc.Revoke(ctx, boot.ID); !errors.Is(err, service.ErrBootstrapAdmin) {
		t.Errorf("bootstrap admin: got %v", err)
	}

	if _, err := svc.Revoke(ctx, uuid.New()); !errors.Is(err, service.ErrAdminTargetNotFound) {
		t.Errorf("missing: got %v", err)
	}
}

func TestAdminRole_Bootstrap(t *testing.T) {
	ctx := context.Background()

	t.Run("startup promotion", func(t *testing.T) {
		svc, users, admins := newAdminRoleFixture(t, "OPS@x.com")
		u := addUser(t, users, "ops@x.com", false)

		id, err := svc.PromoteBootstrap(ctx)
		if err != nil || id != u.ID || !u.IsAdmin {
			t.Fatalf("PromoteBootstrap = %v, %v; flag %v", id, err, u.IsAdmin)
		}
		if len(admins.audit) != 1 || admins.audit[0].Action != "bootstrap_admin" {
			t.Errorf("audit = %+v", admins.audit)
		}
		if id, _ := svc.PromoteBootstrap(ctx); id != uuid.Nil {
			t.Error("second promotion should be a no-op")
		}
		if len(admins.audit) != 1 {
			t.Error("no-op promotion wrote an audit entry")
		}
	})

	t.Run("startup promotion skips unverified holder", func(t *testing.T) {
		svc, users, _ := newAdminRoleFixture(t, "ops@x.com")
		u := addUser(t, users, "ops@x.com", false)
		u.EmailVerified = false
		if id, _ := svc.PromoteBootstrap(ctx); id != uuid.Nil || u.IsAdmin {
			t.Error("unverified holder was promoted")
		}
	})

	t.Run("no bootstrap email", func(t *testing.T) {
		svc, users, _ := newAdminRoleFixture(t, "")
		addUser(t, users, "ops@x.com", false)
		if id, err := svc.PromoteBootstrap(ctx); id != uuid.Nil || err != nil {
			t.Errorf("PromoteBootstrap = %v, %v", id, err)
		}
	})

	t.Run("persist on first admin request", func(t *testing.T) {
		svc, users, admins := newAdminRoleFixture(t, "ops@x.com")
		u := addUser(t, users, "ops@x.com", false)
		if err := svc.PersistBootstrap(ctx, u); err != nil || !u.IsAdmin {
			t.Fatalf("PersistBootstrap = %v; flag %v", err, u.IsAdmin)
		}
		if err := svc.PersistBootstrap(ctx, u); err != nil || len(admins.audit) != 1 {
			t.Errorf("second persist: err %v, audit %d", err, len(admins.audit))
		}

		other := addUser(t, users, "other@x.com", false)
		if err := svc.PersistBootstrap(ctx, other); err != nil || other.IsAdmin {
			t.Error("non-holder was flagged")
		}
	})
}
