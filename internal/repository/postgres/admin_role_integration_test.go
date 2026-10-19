//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/testutil"
	"github.com/google/uuid"
)

func TestAdminRoleIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	adminRepo := postgres.NewAdminRepository(db)

	newUser := func(t *testing.T, email string, verified bool) uuid.UUID {
		t.Helper()
		u := testutil.CreateTestUser(email, "Admin Role", "hash")
		if err := userRepo.Create(ctx, u); err != nil {
			t.Fatalf("create user: %v", err)
		}
		if verified {
			if err := userRepo.MarkEmailVerified(ctx, u.ID); err != nil {
				t.Fatalf("verify: %v", err)
			}
		}
		return u.ID
	}
	isAdmin := func(t *testing.T, id uuid.UUID) bool {
		t.Helper()
		u, err := userRepo.GetByID(ctx, id)
		if err != nil {
			t.Fatalf("get user: %v", err)
		}
		return u.IsAdmin
	}

	t.Run("new accounts are not admins", func(t *testing.T) {
		id := newUser(t, "plain-"+uuid.NewString()+"@example.com", true)
		if isAdmin(t, id) {
			t.Error("new account has the admin flag")
		}
	})

	t.Run("SetAdmin and missing account", func(t *testing.T) {
		id := newUser(t, "set-"+uuid.NewString()+"@example.com", true)
		if err := adminRepo.SetAdmin(ctx, id); err != nil {
			t.Fatalf("SetAdmin: %v", err)
		}
		if err := adminRepo.SetAdmin(ctx, id); err != nil {
			t.Fatalf("SetAdmin twice: %v", err)
		}
		if !isAdmin(t, id) {
			t.Error("flag not set")
		}
		if err := adminRepo.SetAdmin(ctx, uuid.New()); !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("missing account: got %v", err)
		}
	})

	t.Run("PromoteVerifiedByEmail", func(t *testing.T) {
		local := "Boot-" + uuid.NewString()
		unverified := newUser(t, local+"-u@example.com", false)
		if id, err := adminRepo.PromoteVerifiedByEmail(ctx, local+"-u@example.com"); err != nil || id != uuid.Nil {
			t.Errorf("unverified promoted: %v, %v", id, err)
		}
		if isAdmin(t, unverified) {
			t.Error("unverified account flagged")
		}

		verified := newUser(t, local+"@example.com", true)
		id, err := adminRepo.PromoteVerifiedByEmail(ctx, "boot-"+local[5:]+"@EXAMPLE.com")
		if err != nil || id != verified {
			t.Fatalf("PromoteVerifiedByEmail = %v, %v; want %v", id, err, verified)
		}
		if id, _ := adminRepo.PromoteVerifiedByEmail(ctx, local+"@example.com"); id != uuid.Nil {
			t.Error("second promotion returned an id")
		}
	})

	// Clear every flag so the guard below sees only this subtest's admins.
	if _, err := db.ExecContext(ctx, "UPDATE users SET is_admin = false"); err != nil {
		t.Fatalf("reset flags: %v", err)
	}

	t.Run("RevokeAdmin keeps one active admin", func(t *testing.T) {
		a := newUser(t, "ra-"+uuid.NewString()+"@example.com", true)
		b := newUser(t, "rb-"+uuid.NewString()+"@example.com", true)
		for _, id := range []uuid.UUID{a, b} {
			if err := adminRepo.SetAdmin(ctx, id); err != nil {
				t.Fatalf("SetAdmin: %v", err)
			}
		}
		if n, err := adminRepo.CountAdmins(ctx); err != nil || n != 2 {
			t.Fatalf("CountAdmins = %d, %v", n, err)
		}

		yes := true
		rows, total, err := adminRepo.ListUsers(ctx, repository.AdminUserFilter{IsAdmin: &yes}, 50, 0)
		if err != nil || total != 2 || len(rows) != 2 {
			t.Fatalf("ListUsers(isAdmin) = %d rows, total %d, %v", len(rows), total, err)
		}
		for _, r := range rows {
			if !r.IsAdmin {
				t.Errorf("row %s not flagged", r.Email)
			}
		}

		// Concurrent revocations of both admins: exactly one succeeds.
		var wg sync.WaitGroup
		results := make([]bool, 2)
		for i, id := range []uuid.UUID{a, b} {
			wg.Add(1)
			go func(i int, id uuid.UUID) {
				defer wg.Done()
				ok, err := adminRepo.RevokeAdmin(ctx, id)
				if err != nil {
					t.Errorf("RevokeAdmin: %v", err)
				}
				results[i] = ok
			}(i, id)
		}
		wg.Wait()
		if results[0] == results[1] {
			t.Fatalf("revocations = %v, want exactly one success", results)
		}
		if n, _ := adminRepo.CountAdmins(ctx); n != 1 {
			t.Errorf("CountAdmins after race = %d, want 1", n)
		}

		// A disabled admin does not keep the last active one revocable.
		c := newUser(t, "rc-"+uuid.NewString()+"@example.com", true)
		if err := adminRepo.SetAdmin(ctx, c); err != nil {
			t.Fatalf("SetAdmin: %v", err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE users SET disabled = true WHERE id = $1", c); err != nil {
			t.Fatalf("disable: %v", err)
		}
		remaining := a
		if results[0] {
			remaining = b
		}
		if ok, err := adminRepo.RevokeAdmin(ctx, remaining); err != nil || ok {
			t.Errorf("revoked the last active admin: %v, %v", ok, err)
		}
		if ok, err := adminRepo.RevokeAdmin(ctx, c); err != nil || !ok {
			t.Errorf("revoking the disabled admin: %v, %v", ok, err)
		}
	})
}
