//go:build integration

package postgres_test

import (
	"context"
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
	email := func(prefix string) string { return prefix + "-" + uuid.NewString() + "@example.com" }
	isAdmin := func(t *testing.T, id uuid.UUID) bool {
		t.Helper()
		u, err := userRepo.GetByID(ctx, id)
		if err != nil {
			t.Fatalf("get user: %v", err)
		}
		return u.IsAdmin
	}
	grant := func(t *testing.T, id uuid.UUID) {
		t.Helper()
		if r, err := adminRepo.SetAdmin(ctx, id); err != nil || r != repository.GrantApplied {
			t.Fatalf("SetAdmin = %v, %v", r, err)
		}
	}
	reset := func(t *testing.T) {
		t.Helper()
		if _, err := db.ExecContext(ctx, "UPDATE users SET is_admin = false"); err != nil {
			t.Fatalf("reset flags: %v", err)
		}
	}

	t.Run("new accounts are not admins", func(t *testing.T) {
		if isAdmin(t, newUser(t, email("plain"), true)) {
			t.Error("new account has the admin flag")
		}
	})

	t.Run("SetAdmin outcomes", func(t *testing.T) {
		id := newUser(t, email("set"), true)
		grant(t, id)
		if r, _ := adminRepo.SetAdmin(ctx, id); r != repository.GrantAlreadyAdmin {
			t.Errorf("second SetAdmin = %v", r)
		}
		if r, _ := adminRepo.SetAdmin(ctx, uuid.New()); r != repository.GrantNotFound {
			t.Errorf("missing = %v", r)
		}
		unverified := newUser(t, email("unv"), false)
		if r, _ := adminRepo.SetAdmin(ctx, unverified); r != repository.GrantUnverified || isAdmin(t, unverified) {
			t.Errorf("unverified = %v", r)
		}
		disabled := newUser(t, email("dis"), true)
		if _, err := db.ExecContext(ctx, "UPDATE users SET disabled = true WHERE id = $1", disabled); err != nil {
			t.Fatal(err)
		}
		if r, _ := adminRepo.SetAdmin(ctx, disabled); r != repository.GrantDisabled || isAdmin(t, disabled) {
			t.Errorf("disabled = %v", r)
		}
	})

	t.Run("PromoteVerifiedByEmail", func(t *testing.T) {
		local := "Boot-" + uuid.NewString()
		unverified := newUser(t, local+"-u@example.com", false)
		if id, err := adminRepo.PromoteVerifiedByEmail(ctx, local+"-u@example.com"); err != nil || id != uuid.Nil || isAdmin(t, unverified) {
			t.Errorf("unverified promoted: %v, %v", id, err)
		}

		disabledLocal := "Off-" + uuid.NewString()
		disabled := newUser(t, disabledLocal+"@example.com", true)
		if _, err := db.ExecContext(ctx, "UPDATE users SET disabled = true WHERE id = $1", disabled); err != nil {
			t.Fatal(err)
		}
		if id, _ := adminRepo.PromoteVerifiedByEmail(ctx, disabledLocal+"@example.com"); id != uuid.Nil || isAdmin(t, disabled) {
			t.Error("disabled account promoted")
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

	t.Run("RevokeAdmin keeps one active admin under concurrency", func(t *testing.T) {
		reset(t)
		a := newUser(t, email("ra"), true)
		b := newUser(t, email("rb"), true)
		grant(t, a)
		grant(t, b)
		if n, err := adminRepo.CountAdmins(ctx, ""); err != nil || n != 2 {
			t.Fatalf("CountAdmins = %d, %v", n, err)
		}

		var wg sync.WaitGroup
		results := make([]repository.AdminChange, 2)
		for i, id := range []uuid.UUID{a, b} {
			wg.Add(1)
			go func(i int, id uuid.UUID) {
				defer wg.Done()
				ch, err := adminRepo.RevokeAdmin(ctx, id, "")
				if err != nil {
					t.Errorf("RevokeAdmin: %v", err)
				}
				results[i] = ch
			}(i, id)
		}
		wg.Wait()
		applied := 0
		for _, r := range results {
			if r.Guard == repository.AdminGuardApplied {
				applied++
			} else if r.Guard != repository.AdminGuardLastAdmin {
				t.Errorf("unexpected guard %v", r.Guard)
			}
		}
		if applied != 1 {
			t.Fatalf("revocations = %+v, want exactly one applied", results)
		}
		if n, _ := adminRepo.CountAdmins(ctx, ""); n != 1 {
			t.Errorf("CountAdmins after race = %d, want 1", n)
		}
	})

	t.Run("concurrent revokes of the same admin are idempotent", func(t *testing.T) {
		reset(t)
		keep := newUser(t, email("keep"), true)
		target := newUser(t, email("target"), true)
		grant(t, keep)
		grant(t, target)

		var wg sync.WaitGroup
		results := make([]repository.AdminChange, 4)
		for i := range results {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				ch, err := adminRepo.RevokeAdmin(ctx, target, "")
				if err != nil {
					t.Errorf("RevokeAdmin: %v", err)
				}
				results[i] = ch
			}(i)
		}
		wg.Wait()
		cleared := 0
		for _, r := range results {
			if r.Guard != repository.AdminGuardApplied {
				t.Errorf("guard = %v, want applied", r.Guard)
			}
			if r.FlagCleared {
				cleared++
			}
		}
		if cleared != 1 {
			t.Errorf("FlagCleared reported %d times, want 1", cleared)
		}
	})

	t.Run("an unflagged ADMIN_EMAIL holder counts as an active admin", func(t *testing.T) {
		reset(t)
		boot := email("boot")
		bootID := newUser(t, boot, true)
		only := newUser(t, email("only"), true)
		grant(t, only)

		if n, _ := adminRepo.CountAdmins(ctx, boot); n != 2 {
			t.Errorf("CountAdmins with bootstrap = %d, want 2", n)
		}
		if ch, err := adminRepo.RevokeAdmin(ctx, bootID, boot); err != nil || ch.Guard != repository.AdminGuardBootstrap {
			t.Errorf("revoke bootstrap = %+v, %v", ch, err)
		}
		if ch, err := adminRepo.RevokeAdmin(ctx, only, boot); err != nil || ch.Guard != repository.AdminGuardApplied || !ch.FlagCleared {
			t.Errorf("revoke beside unflagged bootstrap = %+v, %v", ch, err)
		}

		yes, no := true, false
		rows, _, err := adminRepo.ListUsers(ctx, repository.AdminUserFilter{IsAdmin: &yes, BootstrapEmail: boot}, 100, 0)
		if err != nil || len(rows) != 1 || rows[0].ID != bootID {
			t.Errorf("isAdmin=true listing = %d rows, %v", len(rows), err)
		}
		rows, _, _ = adminRepo.ListUsers(ctx, repository.AdminUserFilter{IsAdmin: &no, BootstrapEmail: boot}, 1000, 0)
		for _, r := range rows {
			if r.ID == bootID {
				t.Error("isAdmin=false listing includes the bootstrap holder")
			}
		}
	})

	t.Run("a flagged unverified account is not counted or listed as admin", func(t *testing.T) {
		reset(t)
		active := newUser(t, email("act"), true)
		stale := newUser(t, email("stale"), true)
		grant(t, active)
		grant(t, stale)
		if _, err := db.ExecContext(ctx, "UPDATE users SET email_verified = false WHERE id = $1", stale); err != nil {
			t.Fatal(err)
		}
		if n, _ := adminRepo.CountAdmins(ctx, ""); n != 1 {
			t.Errorf("CountAdmins = %d, want 1", n)
		}
		yes := true
		rows, _, _ := adminRepo.ListUsers(ctx, repository.AdminUserFilter{IsAdmin: &yes}, 100, 0)
		if len(rows) != 1 || rows[0].ID != active {
			t.Errorf("isAdmin=true listing = %d rows", len(rows))
		}

		// The only active admin's last-admin guard is not satisfied by it,
		// but the stale flag itself can always be revoked.
		if ch, _ := adminRepo.RevokeAdmin(ctx, active, ""); ch.Guard != repository.AdminGuardLastAdmin {
			t.Errorf("revoke last active = %+v", ch)
		}
		if ch, _ := adminRepo.RevokeAdmin(ctx, stale, ""); ch.Guard != repository.AdminGuardApplied || !ch.FlagCleared {
			t.Errorf("revoke stale flag = %+v", ch)
		}
	})

	t.Run("DisableUser and DeleteUser are guarded and atomic", func(t *testing.T) {
		reset(t)
		a := newUser(t, email("da"), true)
		b := newUser(t, email("db"), true)
		grant(t, a)
		grant(t, b)

		ch, err := adminRepo.DisableUser(ctx, a, "")
		if err != nil || ch.Guard != repository.AdminGuardApplied || !ch.FlagCleared {
			t.Fatalf("disable admin = %+v, %v", ch, err)
		}
		u, _ := userRepo.GetByID(ctx, a)
		if !u.Disabled || u.IsAdmin {
			t.Errorf("after disable: disabled %v, flag %v", u.Disabled, u.IsAdmin)
		}

		if ch, _ := adminRepo.DisableUser(ctx, b, ""); ch.Guard != repository.AdminGuardLastAdmin {
			t.Errorf("disable last admin = %+v", ch)
		}
		if ch, _ := adminRepo.DeleteUser(ctx, b, ""); ch.Guard != repository.AdminGuardLastAdmin {
			t.Errorf("delete last admin = %+v", ch)
		}
		u, _ = userRepo.GetByID(ctx, b)
		if u == nil || u.Disabled || !u.IsAdmin {
			t.Error("refused removal changed the last admin")
		}

		plain := newUser(t, email("dp"), true)
		if ch, err := adminRepo.DeleteUser(ctx, plain, ""); err != nil || ch.Guard != repository.AdminGuardApplied || ch.FlagCleared {
			t.Errorf("delete non-admin = %+v, %v", ch, err)
		}
		if _, err := userRepo.GetByID(ctx, plain); err == nil {
			t.Error("deleted account still exists")
		}
		if ch, _ := adminRepo.DeleteUser(ctx, plain, ""); ch.Guard != repository.AdminGuardNotFound {
			t.Errorf("delete missing = %+v", ch)
		}
	})

	t.Run("grant and disable race leaves no disabled admin", func(t *testing.T) {
		reset(t)
		keep := newUser(t, email("gk"), true)
		grant(t, keep)
		for i := 0; i < 20; i++ {
			target := newUser(t, email("gr"), true)
			var wg sync.WaitGroup
			wg.Add(2)
			go func() { defer wg.Done(); _, _ = adminRepo.SetAdmin(ctx, target) }()
			go func() { defer wg.Done(); _, _ = adminRepo.DisableUser(ctx, target, "") }()
			wg.Wait()
			u, _ := userRepo.GetByID(ctx, target)
			if u.Disabled && u.IsAdmin {
				t.Fatalf("iteration %d: account is disabled and flagged", i)
			}
		}
	})
}
