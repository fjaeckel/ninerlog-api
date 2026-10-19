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

// fakeAdminRepo mirrors the guards of the postgres implementation over the
// users held by a mockUserRepo.
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

func isBoot(u *models.User, bootstrap string) bool {
	return u.EmailVerified && bootstrap != "" && strings.EqualFold(u.Email, bootstrap)
}

func active(u *models.User, bootstrap string) bool {
	return u.EmailVerified && !u.Disabled && (u.IsAdmin || isBoot(u, bootstrap))
}

func (f *fakeAdminRepo) SetAdmin(_ context.Context, id uuid.UUID) (repository.GrantResult, error) {
	u := f.byID(id)
	switch {
	case u == nil:
		return repository.GrantNotFound, nil
	case u.IsAdmin:
		return repository.GrantAlreadyAdmin, nil
	case !u.EmailVerified:
		return repository.GrantUnverified, nil
	case u.Disabled:
		return repository.GrantDisabled, nil
	}
	u.IsAdmin = true
	return repository.GrantApplied, nil
}

func (f *fakeAdminRepo) guarded(id uuid.UUID, bootstrap string, write func(u *models.User)) (repository.AdminChange, error) {
	u := f.byID(id)
	if u == nil {
		return repository.AdminChange{Guard: repository.AdminGuardNotFound}, nil
	}
	if isBoot(u, bootstrap) {
		return repository.AdminChange{Guard: repository.AdminGuardBootstrap}, nil
	}
	if active(u, bootstrap) {
		others := false
		for _, o := range f.users.users {
			if o.ID != id && active(o, bootstrap) {
				others = true
			}
		}
		if !others {
			return repository.AdminChange{Guard: repository.AdminGuardLastAdmin}, nil
		}
	}
	was := u.IsAdmin
	write(u)
	return repository.AdminChange{Guard: repository.AdminGuardApplied, FlagCleared: was}, nil
}

func (f *fakeAdminRepo) RevokeAdmin(_ context.Context, id uuid.UUID, bootstrap string) (repository.AdminChange, error) {
	return f.guarded(id, bootstrap, func(u *models.User) { u.IsAdmin = false })
}

func (f *fakeAdminRepo) DisableUser(_ context.Context, id uuid.UUID, bootstrap string) (repository.AdminChange, error) {
	return f.guarded(id, bootstrap, func(u *models.User) { u.IsAdmin = false; u.Disabled = true })
}

func (f *fakeAdminRepo) DeleteUser(_ context.Context, id uuid.UUID, bootstrap string) (repository.AdminChange, error) {
	return f.guarded(id, bootstrap, func(u *models.User) { delete(f.users.users, u.Email) })
}

func (f *fakeAdminRepo) PromoteVerifiedByEmail(_ context.Context, email string) (uuid.UUID, error) {
	for _, u := range f.users.users {
		if strings.EqualFold(u.Email, email) && u.EmailVerified && !u.Disabled && !u.IsAdmin {
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
	return service.NewAdminRoleService(admins, bootstrap), users, admins
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
	if granted, err := svc.Grant(ctx, target.ID); err != nil || !granted || !target.IsAdmin {
		t.Fatalf("Grant = %v, %v; flag %v", granted, err, target.IsAdmin)
	}
	if granted, err := svc.Grant(ctx, target.ID); err != nil || granted {
		t.Errorf("second Grant = %v, %v; want no-op", granted, err)
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

	if revoked, err := svc.Revoke(ctx, a.ID); err != nil || !revoked || a.IsAdmin {
		t.Fatalf("Revoke with another admin left = %v, %v; flag %v", revoked, err, a.IsAdmin)
	}
	if revoked, err := svc.Revoke(ctx, a.ID); err != nil || revoked {
		t.Errorf("revoking a non-admin = %v, %v; want no-op", revoked, err)
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

	// An unflagged, verified ADMIN_EMAIL holder counts as a remaining admin.
	boot := addUser(t, users, "Boot@X.com", false)
	if revoked, err := svc.Revoke(ctx, b.ID); err != nil || !revoked {
		t.Errorf("revoke alongside unflagged bootstrap = %v, %v", revoked, err)
	}
	if _, err := svc.Revoke(ctx, boot.ID); !errors.Is(err, service.ErrBootstrapAdmin) {
		t.Errorf("bootstrap admin: got %v", err)
	}

	// A flagged but unverified account is not active and can always be revoked.
	stale := addUser(t, users, "stale@x.com", true)
	stale.EmailVerified = false
	svcNoBoot, usersNoBoot, _ := newAdminRoleFixture(t, "")
	only := addUser(t, usersNoBoot, "stale@x.com", true)
	only.EmailVerified = false
	if revoked, err := svcNoBoot.Revoke(ctx, only.ID); err != nil || !revoked {
		t.Errorf("revoking an inactive flagged account = %v, %v", revoked, err)
	}

	if _, err := svc.Revoke(ctx, uuid.New()); !errors.Is(err, service.ErrAdminTargetNotFound) {
		t.Errorf("missing: got %v", err)
	}
}

func TestAdminRole_DisableDelete(t *testing.T) {
	ctx := context.Background()
	svc, users, _ := newAdminRoleFixture(t, "boot@x.com")

	boot := addUser(t, users, "boot@x.com", true)
	if _, err := svc.Disable(ctx, boot.ID); !errors.Is(err, service.ErrBootstrapAdmin) {
		t.Errorf("disable bootstrap: got %v", err)
	}
	if _, err := svc.Delete(ctx, boot.ID); !errors.Is(err, service.ErrBootstrapAdmin) {
		t.Errorf("delete bootstrap: got %v", err)
	}
	if boot.Disabled {
		t.Error("bootstrap admin was disabled")
	}

	plain := addUser(t, users, "plain@x.com", false)
	if revoked, err := svc.Disable(ctx, plain.ID); err != nil || revoked || !plain.Disabled {
		t.Errorf("disable non-admin = %v, %v; disabled %v", revoked, err, plain.Disabled)
	}

	a := addUser(t, users, "a@x.com", true)
	if revoked, err := svc.Disable(ctx, a.ID); err != nil || !revoked || a.IsAdmin || !a.Disabled {
		t.Errorf("disable admin = %v, %v; flag %v disabled %v", revoked, err, a.IsAdmin, a.Disabled)
	}

	b := addUser(t, users, "b@x.com", true)
	if revoked, err := svc.Delete(ctx, b.ID); err != nil || !revoked {
		t.Errorf("delete admin = %v, %v", revoked, err)
	}

	svc2, users2, _ := newAdminRoleFixture(t, "")
	last := addUser(t, users2, "last@x.com", true)
	if _, err := svc2.Disable(ctx, last.ID); !errors.Is(err, service.ErrLastAdmin) {
		t.Errorf("disable last admin: got %v", err)
	}
	if _, err := svc2.Delete(ctx, last.ID); !errors.Is(err, service.ErrLastAdmin) {
		t.Errorf("delete last admin: got %v", err)
	}
	if !last.IsAdmin || last.Disabled {
		t.Error("refused removal changed the last admin")
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

	t.Run("startup promotion skips unverified and disabled holders", func(t *testing.T) {
		svc, users, _ := newAdminRoleFixture(t, "ops@x.com")
		u := addUser(t, users, "ops@x.com", false)
		u.EmailVerified = false
		if id, _ := svc.PromoteBootstrap(ctx); id != uuid.Nil || u.IsAdmin {
			t.Error("unverified holder was promoted")
		}
		u.EmailVerified = true
		u.Disabled = true
		if id, _ := svc.PromoteBootstrap(ctx); id != uuid.Nil || u.IsAdmin {
			t.Error("disabled holder was promoted")
		}
	})

	t.Run("no bootstrap email", func(t *testing.T) {
		svc, users, _ := newAdminRoleFixture(t, "")
		addUser(t, users, "ops@x.com", false)
		if id, err := svc.PromoteBootstrap(ctx); id != uuid.Nil || err != nil {
			t.Errorf("PromoteBootstrap = %v, %v", id, err)
		}
	})

	t.Run("persist on first admin request audits once", func(t *testing.T) {
		svc, users, admins := newAdminRoleFixture(t, "ops@x.com")
		u := addUser(t, users, "ops@x.com", false)

		// A second request that loaded the account before the flag was written.
		stale := *u
		if err := svc.PersistBootstrap(ctx, u); err != nil || !u.IsAdmin {
			t.Fatalf("PersistBootstrap = %v; flag %v", err, u.IsAdmin)
		}
		if err := svc.PersistBootstrap(ctx, &stale); err != nil {
			t.Fatalf("concurrent PersistBootstrap: %v", err)
		}
		if len(admins.audit) != 1 {
			t.Errorf("audit entries = %d, want 1", len(admins.audit))
		}

		other := addUser(t, users, "other@x.com", false)
		if err := svc.PersistBootstrap(ctx, other); err != nil || other.IsAdmin {
			t.Error("non-holder was flagged")
		}
	})
}
