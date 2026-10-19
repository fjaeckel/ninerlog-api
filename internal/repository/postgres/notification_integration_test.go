//go:build integration

package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/testutil"
	"github.com/google/uuid"
)

func TestNotificationClaimIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	userRepo := postgres.NewUserRepository(db)
	repo := postgres.NewNotificationRepository(db)
	ctx := context.Background()

	newUser := func(t *testing.T) uuid.UUID {
		t.Helper()
		u := testutil.CreateTestUser("notif-"+uuid.NewString()+"@example.com", "Notif User", "hash")
		if err := userRepo.Create(ctx, u); err != nil {
			t.Fatalf("create user: %v", err)
		}
		return u.ID
	}
	row := func(userID, refID uuid.UUID, days int, date time.Time) *models.NotificationLog {
		refType, subject := "credential", "subject"
		return &models.NotificationLog{
			UserID: userID, NotificationType: string(models.NotifCategoryCurrencyPassenger),
			ReferenceID: &refID, ReferenceType: &refType, DaysBeforeExpiry: &days,
			ExpiryReferenceDate: &date, Subject: &subject,
		}
	}
	lapse := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)

	t.Run("second claim of the same key conflicts", func(t *testing.T) {
		userID, refID := newUser(t), uuid.New()
		first, err := repo.ClaimNotification(ctx, row(userID, refID, 7, lapse))
		if err != nil || !first {
			t.Fatalf("first claim = %v, %v; want true, nil", first, err)
		}
		second, err := repo.ClaimNotification(ctx, row(userID, refID, 7, lapse))
		if err != nil || second {
			t.Fatalf("second claim = %v, %v; want false, nil", second, err)
		}
		other, err := repo.ClaimNotification(ctx, row(userID, refID, 7, lapse.AddDate(0, 6, 0)))
		if err != nil || !other {
			t.Fatalf("claim for another date = %v, %v; want true, nil", other, err)
		}
	})

	t.Run("never-current date is deduplicated by the index", func(t *testing.T) {
		userID, refID := newUser(t), uuid.New()
		for i, want := range []bool{true, false} {
			got, err := repo.ClaimNotification(ctx, row(userID, refID, 0, models.NeverCurrentDate))
			if err != nil || got != want {
				t.Fatalf("claim %d = %v, %v; want %v", i, got, err, want)
			}
		}
	})

	t.Run("lapse claim is covered by a row dated on or after it", func(t *testing.T) {
		userID, refID := newUser(t), uuid.New()
		if ok, err := repo.ClaimNotification(ctx, row(userID, refID, 0, time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC))); err != nil || !ok {
			t.Fatalf("seed claim = %v, %v", ok, err)
		}
		covered, err := repo.ClaimLapseNotification(ctx, row(userID, refID, 0, lapse))
		if err != nil || covered {
			t.Fatalf("earlier lapse claim = %v, %v; want false, nil", covered, err)
		}
		later, err := repo.ClaimLapseNotification(ctx, row(userID, refID, 0, time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)))
		if err != nil || !later {
			t.Fatalf("later lapse claim = %v, %v; want true, nil", later, err)
		}
	})

	t.Run("released claim can be claimed again", func(t *testing.T) {
		userID, refID := newUser(t), uuid.New()
		r := row(userID, refID, 0, lapse)
		if ok, err := repo.ClaimLapseNotification(ctx, r); err != nil || !ok {
			t.Fatalf("claim = %v, %v", ok, err)
		}
		if err := repo.ReleaseNotification(ctx, r.ID); err != nil {
			t.Fatalf("release: %v", err)
		}
		if ok, err := repo.ClaimLapseNotification(ctx, row(userID, refID, 0, lapse)); err != nil || !ok {
			t.Fatalf("claim after release = %v, %v; want true, nil", ok, err)
		}
	})

	t.Run("skipped thresholds are left out of the history", func(t *testing.T) {
		userID, refID := newUser(t), uuid.New()
		skipped := row(userID, refID, 30, lapse)
		skipped.Skipped = true
		if _, err := repo.ClaimNotification(ctx, skipped); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.ClaimNotification(ctx, row(userID, refID, 7, lapse)); err != nil {
			t.Fatal(err)
		}
		logs, total, err := repo.GetNotificationHistory(ctx, userID, 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 || len(logs) != 1 || *logs[0].DaysBeforeExpiry != 7 {
			t.Errorf("history = %d rows (total %d), want the one sent row", len(logs), total)
		}
	})
}

func TestNotificationLogMigrationBackfillIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	t.Cleanup(func() { testutil.TeardownTestDB(t, db) })
	ctx := context.Background()

	readMigration := func(t *testing.T, dir string) string {
		t.Helper()
		b, err := os.ReadFile("../../../db/migrations/000077_notification_log_concerned_date." + dir + ".sql")
		if err != nil {
			t.Fatalf("read migration: %v", err)
		}
		return string(b)
	}
	if _, err := db.ExecContext(ctx, readMigration(t, "down")); err != nil {
		t.Fatalf("down migration: %v", err)
	}
	upApplied := false
	t.Cleanup(func() {
		if upApplied {
			return
		}
		if _, err := db.ExecContext(ctx, readMigration(t, "up")); err != nil {
			t.Errorf("restore up migration: %v", err)
		}
	})

	user := testutil.CreateTestUser("backfill-"+uuid.NewString()+"@example.com", "Backfill", "hash")
	if err := postgres.NewUserRepository(db).Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	license := &models.License{UserID: user.ID, RegulatoryAuthority: "EASA", LicenseType: "PPL", LicenseNumber: "PPL-1", IssueDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), IssuingAuthority: "EASA"}
	if err := postgres.NewLicenseRepository(db).Create(ctx, license); err != nil {
		t.Fatal(err)
	}
	ratingExpiry := time.Date(2027, 5, 31, 0, 0, 0, 0, time.UTC)
	rating := &models.ClassRating{LicenseID: license.ID, ClassType: models.ClassTypeSEPLand, IssueDate: license.IssueDate, ExpiryDate: &ratingExpiry}
	if err := postgres.NewClassRatingRepository(db).Create(ctx, rating); err != nil {
		t.Fatal(err)
	}

	paxRef := uuid.New()
	legacy := []struct {
		kind   string
		ref    *uuid.UUID
		days   *int
		sentAt string
	}{
		{"currency_passenger", &paxRef, intPtr(0), "2026-04-10 07:00:00+00"},
		{"currency_passenger", &paxRef, intPtr(0), "2026-04-10 07:00:02+00"},
		{"currency_revalidation", &rating.ID, intPtr(0), "2026-02-01 08:00:00+00"},
		{"credential_medical", nil, nil, "2025-01-05 08:00:00+00"},
	}
	for _, l := range legacy {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO notification_log (user_id, notification_type, reference_id, days_before_expiry, sent_at) VALUES ($1, $2, $3, $4, $5)`,
			user.ID, l.kind, l.ref, l.days, l.sentAt); err != nil {
			t.Fatalf("insert legacy row: %v", err)
		}
	}

	if _, err := db.ExecContext(ctx, readMigration(t, "up")); err != nil {
		t.Fatalf("up migration: %v", err)
	}
	upApplied = true

	type got struct {
		kind string
		ref  uuid.UUID
		days int
		date string
	}
	rows, err := db.QueryContext(ctx, `SELECT notification_type, reference_id, days_before_expiry, to_char(expiry_reference_date, 'YYYY-MM-DD') FROM notification_log WHERE user_id = $1 ORDER BY notification_type`, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var all []got
	for rows.Next() {
		var g got
		if err := rows.Scan(&g.kind, &g.ref, &g.days, &g.date); err != nil {
			t.Fatal(err)
		}
		all = append(all, g)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []got{
		{"credential_medical", uuid.Nil, 0, "2025-01-05"},
		{"currency_passenger", paxRef, 0, "2026-04-10"},
		{"currency_revalidation", rating.ID, 0, "2027-05-31"},
	}
	if len(all) != len(want) {
		t.Fatalf("rows after backfill = %+v, want %+v", all, want)
	}
	for i := range want {
		if all[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, all[i], want[i])
		}
	}

	repo := postgres.NewNotificationRepository(db)
	refType, subject := "passenger_currency", "s"
	zero := 0
	sameLapse := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	claimed, err := repo.ClaimLapseNotification(ctx, &models.NotificationLog{
		UserID: user.ID, NotificationType: "currency_passenger", ReferenceID: &paxRef, ReferenceType: &refType,
		DaysBeforeExpiry: &zero, ExpiryReferenceDate: &sameLapse, Subject: &subject,
	})
	if err != nil || claimed {
		t.Errorf("lapse already notified before the migration claimed again = %v, %v", claimed, err)
	}

	if _, err := db.ExecContext(ctx,
		`INSERT INTO notification_log (user_id, notification_type, reference_id, days_before_expiry, sent_at) VALUES ($1, 'x', $2, 0, NOW())`,
		user.ID, uuid.New()); err == nil {
		t.Error("insert without a concerned date succeeded, want NOT NULL violation")
	}
}

func intPtr(v int) *int { return &v }
