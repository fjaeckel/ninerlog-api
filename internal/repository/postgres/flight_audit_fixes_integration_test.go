//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStrictLateEditIntegration covers the no-op save check and the contact
// linking transaction against real Postgres.
func TestStrictLateEditIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	flightRepo := postgres.NewFlightRepository(db)
	crewRepo := postgres.NewFlightCrewRepository(db)
	logbookRepo := postgres.NewLogbookRepository(db)
	contactRepo := postgres.NewContactRepository(db)
	logbookSvc := service.NewLogbookService(logbookRepo, postgres.NewLicenseRepository(db), flightRepo, postgres.NewFlightSignatureRepository(db))
	flightSvc := service.NewFlightService(flightRepo, nil)
	flightSvc.SetLogbookService(logbookSvc)
	contactSvc := service.NewContactService(contactRepo)

	lateFlight := func(t *testing.T, userID uuid.UUID) *models.Flight {
		t.Helper()
		off, on := "14:15:00", "15:15:00"
		f := reviewFlight(userID, time.Now().UTC().AddDate(0, 0, -10), time.Now().Add(-10*24*time.Hour))
		f.OffBlockTime, f.OnBlockTime = &off, &on
		crew := []models.FlightCrewMember{
			{Name: "Pat Instructor", Role: models.CrewRole("Instructor")},
			{Name: "Sam Student", Role: models.CrewRole("Student")},
		}
		require.NoError(t, flightRepo.CreateWithCrew(ctx, f, crew))
		return f
	}
	revisionCount := func(t *testing.T, userID, flightID uuid.UUID) int {
		t.Helper()
		revs, err := logbookRepo.ListRevisions(ctx, userID, flightID)
		require.NoError(t, err)
		return len(revs)
	}

	t.Run("a late save that changes nothing commits without a revision or a reason", func(t *testing.T) {
		userID := newReviewUser(t, ctx, userRepo, logbookRepo, true)
		f := lateFlight(t, userID)

		stored, err := flightRepo.GetByID(ctx, f.ID)
		require.NoError(t, err)
		off, on := "14:15", "15:15"
		stored.OffBlockTime, stored.OnBlockTime = &off, &on
		crew := []models.FlightCrewMember{
			{Name: "Sam Student", Role: models.CrewRole("Student")},
			{Name: "Pat Instructor", Role: models.CrewRole("Instructor")},
		}

		err = flightSvc.UpdateFlightWith(ctx, stored, userID, service.FlightWriteOptions{Crew: &crew})
		require.NoError(t, err)
		assert.Zero(t, revisionCount(t, userID, f.ID))
	})

	t.Run("a late change without a reason is refused and nothing is written", func(t *testing.T) {
		userID := newReviewUser(t, ctx, userRepo, logbookRepo, true)
		f := lateFlight(t, userID)

		stored, err := flightRepo.GetByID(ctx, f.ID)
		require.NoError(t, err)
		remarks := "changed"
		stored.Remarks = &remarks
		crew := []models.FlightCrewMember{{Name: "Someone Else", Role: models.CrewRole("Instructor")}}

		err = flightSvc.UpdateFlightWith(ctx, stored, userID, service.FlightWriteOptions{Crew: &crew})
		require.ErrorIs(t, err, service.ErrCorrectionReasonRequired)

		after, err := flightRepo.GetByID(ctx, f.ID)
		require.NoError(t, err)
		assert.Nil(t, after.Remarks)
		members, err := crewRepo.GetByFlightID(ctx, f.ID)
		require.NoError(t, err)
		assert.Len(t, members, 2)
		assert.Zero(t, revisionCount(t, userID, f.ID))
	})

	t.Run("a late change with a reason is recorded as a correction", func(t *testing.T) {
		userID := newReviewUser(t, ctx, userRepo, logbookRepo, true)
		f := lateFlight(t, userID)

		stored, err := flightRepo.GetByID(ctx, f.ID)
		require.NoError(t, err)
		remarks := "changed"
		stored.Remarks = &remarks

		require.NoError(t, flightSvc.UpdateFlightWith(ctx, stored, userID, service.FlightWriteOptions{CorrectionReason: "typo"}))
		revs, err := logbookRepo.ListRevisions(ctx, userID, f.ID)
		require.NoError(t, err)
		require.Len(t, revs, 1)
		assert.True(t, revs[0].IsCorrection)
		require.NotNil(t, revs[0].Reason)
		assert.Equal(t, "typo", *revs[0].Reason)
	})

	t.Run("contacts linked before a failed crew write are not created", func(t *testing.T) {
		userID := newReviewUser(t, ctx, userRepo, logbookRepo, false)
		f := reviewFlight(userID, time.Now().UTC(), time.Time{})
		crew := []models.FlightCrewMember{
			{Name: "Linked Person", Role: models.CrewRole("Instructor")},
			{Name: "Broken Row", Role: models.CrewRole("NoSuchRole")},
		}
		hook := func(ctx context.Context, repos repository.TxRepos) error {
			_, err := contactSvc.WithRepository(repos.Contacts()).LinkCrewMembers(ctx, userID, crew)
			return err
		}

		err := flightSvc.CreateFlightWith(ctx, f, service.FlightWriteOptions{Crew: &crew, BeforeWrite: hook})
		require.Error(t, err)
		assert.True(t, errors.Is(err, repository.ErrCrewWrite), "error = %v", err)

		_, err = contactRepo.GetByExactName(ctx, userID, "Linked Person")
		assert.ErrorIs(t, err, repository.ErrNotFound)
	})

	t.Run("contacts linked inside a successful write are kept", func(t *testing.T) {
		userID := newReviewUser(t, ctx, userRepo, logbookRepo, false)
		f := reviewFlight(userID, time.Now().UTC(), time.Time{})
		crew := []models.FlightCrewMember{{Name: "Kept Person", Role: models.CrewRole("Instructor")}}
		hook := func(ctx context.Context, repos repository.TxRepos) error {
			_, err := contactSvc.WithRepository(repos.Contacts()).LinkCrewMembers(ctx, userID, crew)
			return err
		}

		require.NoError(t, flightSvc.CreateFlightWith(ctx, f, service.FlightWriteOptions{Crew: &crew, BeforeWrite: hook}))
		contact, err := contactRepo.GetByExactName(ctx, userID, "Kept Person")
		require.NoError(t, err)
		members, err := crewRepo.GetByFlightID(ctx, f.ID)
		require.NoError(t, err)
		require.Len(t, members, 1)
		require.NotNil(t, members[0].ContactID)
		assert.Equal(t, contact.ID, *members[0].ContactID)
	})
}

// TestBulkDeleteClassificationIntegration covers the correction flag on the
// void records of a bulk delete.
func TestBulkDeleteClassificationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	flightRepo := postgres.NewFlightRepository(db)
	logbookRepo := postgres.NewLogbookRepository(db)
	logbookSvc := service.NewLogbookService(logbookRepo, postgres.NewLicenseRepository(db), flightRepo, postgres.NewFlightSignatureRepository(db))

	type seeded struct {
		userID              uuid.UUID
		fresh, late, signed *models.Flight
	}
	setup := func(t *testing.T) seeded {
		t.Helper()
		s := seeded{userID: newReviewUser(t, ctx, userRepo, logbookRepo, true)}
		today := time.Now().UTC().Truncate(24 * time.Hour)
		s.fresh = reviewFlight(s.userID, today, time.Time{})
		s.late = reviewFlight(s.userID, today.AddDate(0, 0, -5), time.Now().Add(-5*24*time.Hour))
		s.signed = reviewFlight(s.userID, today, time.Time{})
		for _, f := range []*models.Flight{s.fresh, s.late, s.signed} {
			require.NoError(t, flightRepo.CreateWithCrew(ctx, f, nil))
		}
		var sigID uuid.UUID
		require.NoError(t, db.QueryRowContext(ctx,
			`INSERT INTO flight_signatures (flight_id, user_id, method, status) VALUES ($1, $2, 'live', 'completed') RETURNING id`,
			s.signed.ID, s.userID).Scan(&sigID))
		_, err := db.ExecContext(ctx, `UPDATE flights SET signature_id = $1 WHERE id = $2`, sigID, s.signed.ID)
		require.NoError(t, err)
		return s
	}
	assertVoided := func(t *testing.T, s seeded) {
		t.Helper()
		want := map[uuid.UUID]bool{s.fresh.ID: false, s.late.ID: true, s.signed.ID: true}
		for id, correction := range want {
			revs, err := logbookRepo.ListRevisions(ctx, s.userID, id)
			require.NoError(t, err)
			require.Len(t, revs, 1, "flight %s", id)
			assert.Equal(t, models.RevisionKindDelete, revs[0].Kind)
			assert.Equal(t, repository.ChangeSourceUserEdit, revs[0].Source)
			assert.Equal(t, correction, revs[0].IsCorrection, "flight %s", id)
		}
	}

	t.Run("deleting all flights flags the late and the signed ones", func(t *testing.T) {
		s := setup(t)
		userData := service.NewUserDataService(postgres.NewUserContentRepository(db))
		userData.SetLogbookService(logbookSvc)

		deleted, err := userData.DeleteFlights(ctx, s.userID)
		require.NoError(t, err)
		assert.EqualValues(t, 3, deleted)
		assertVoided(t, s)
	})

	t.Run("deleting all content flags the late and the signed ones", func(t *testing.T) {
		s := setup(t)
		userData := service.NewUserDataService(postgres.NewUserContentRepository(db))
		userData.SetLogbookService(logbookSvc)

		require.NoError(t, userData.DeleteAll(ctx, s.userID))
		assertVoided(t, s)
	})

	t.Run("the flight service bulk delete flags the late and the signed ones", func(t *testing.T) {
		s := setup(t)
		flightSvc := service.NewFlightService(flightRepo, nil)
		flightSvc.SetLogbookService(logbookSvc)

		deleted, err := flightSvc.DeleteAllFlights(ctx, s.userID)
		require.NoError(t, err)
		assert.EqualValues(t, 3, deleted)
		assertVoided(t, s)
	})

	t.Run("a flight inside a certified range is flagged", func(t *testing.T) {
		userID := newReviewUser(t, ctx, userRepo, logbookRepo, true)
		today := time.Now().UTC().Truncate(24 * time.Hour)
		f := reviewFlight(userID, today.AddDate(0, 0, -30), time.Now())
		require.NoError(t, flightRepo.CreateWithCrew(ctx, f, nil))
		_, err := db.ExecContext(ctx, `INSERT INTO logbook_certifications (user_id, certified_through) VALUES ($1, $2)`, userID, today.AddDate(0, 0, -20))
		require.NoError(t, err)
		userData := service.NewUserDataService(postgres.NewUserContentRepository(db))
		userData.SetLogbookService(logbookSvc)

		_, err = userData.DeleteFlights(ctx, userID)
		require.NoError(t, err)
		revs, err := logbookRepo.ListRevisions(ctx, userID, f.ID)
		require.NoError(t, err)
		require.Len(t, revs, 1)
		assert.True(t, revs[0].IsCorrection)
	})

	t.Run("a standard account's bulk delete writes no revisions", func(t *testing.T) {
		userID := newReviewUser(t, ctx, userRepo, logbookRepo, false)
		f := reviewFlight(userID, time.Now().UTC().AddDate(0, 0, -5), time.Now().Add(-5*24*time.Hour))
		require.NoError(t, flightRepo.CreateWithCrew(ctx, f, nil))
		userData := service.NewUserDataService(postgres.NewUserContentRepository(db))
		userData.SetLogbookService(logbookSvc)

		deleted, err := userData.DeleteFlights(ctx, userID)
		require.NoError(t, err)
		assert.EqualValues(t, 1, deleted)
		revs, err := logbookRepo.ListRevisions(ctx, userID, f.ID)
		require.NoError(t, err)
		assert.Empty(t, revs)
	})
}

// TestLogbookAggregateQueriesIntegration covers the single-query mode overview
// and the last-correction aggregate.
func TestLogbookAggregateQueriesIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	flightRepo := postgres.NewFlightRepository(db)
	logbookRepo := postgres.NewLogbookRepository(db)

	t.Run("overview counts only void records the voided list can render", func(t *testing.T) {
		userID := newReviewUser(t, ctx, userRepo, logbookRepo, true)
		today := time.Now().UTC().Truncate(24 * time.Hour)
		_, err := db.ExecContext(ctx, `INSERT INTO logbook_certifications (user_id, certified_through) VALUES ($1, $2), ($1, $3)`,
			userID, today.AddDate(0, 0, -9), today.AddDate(0, 0, -3))
		require.NoError(t, err)

		insert := func(kind, snapshot string) {
			_, err := db.ExecContext(ctx,
				`INSERT INTO flight_revisions (user_id, flight_id, revision_no, kind, snapshot, source) VALUES ($1, $2, 1, $3, $4::jsonb, 'user_edit')`,
				userID, uuid.New(), kind, snapshot)
			require.NoError(t, err)
		}
		id := uuid.NewString()
		insert("delete", `{"id":"`+id+`","date":"2026-03-01"}`)
		insert("delete", `{"id":"`+id+`","date":"2026-03-02"}`)
		insert("delete", `{"id":"`+id+`","date":"March 3rd"}`)
		insert("delete", `{"id":"`+id+`"}`)
		insert("update", `{"id":"`+id+`","date":"2026-03-04"}`)

		overview, err := logbookRepo.GetOverview(ctx, userID)
		require.NoError(t, err)
		assert.True(t, overview.Strict)
		assert.NotNil(t, overview.Since)
		assert.Equal(t, 2, overview.VoidedCount)
		require.NotNil(t, overview.LatestCertifiedThrough)
		assert.Equal(t, today.AddDate(0, 0, -3).Format("2006-01-02"), overview.LatestCertifiedThrough.Format("2006-01-02"))
	})

	t.Run("overview of an unknown user is not found", func(t *testing.T) {
		_, err := logbookRepo.GetOverview(ctx, uuid.New())
		assert.ErrorIs(t, err, repository.ErrNotFound)
	})

	t.Run("last corrections are the newest correction update per flight", func(t *testing.T) {
		userID := newReviewUser(t, ctx, userRepo, logbookRepo, true)
		corrected := reviewFlight(userID, time.Now().UTC(), time.Time{})
		plain := reviewFlight(userID, time.Now().UTC(), time.Time{})
		deleted := reviewFlight(userID, time.Now().UTC(), time.Time{})
		for _, f := range []*models.Flight{corrected, plain, deleted} {
			require.NoError(t, flightRepo.CreateWithCrew(ctx, f, nil))
		}
		edit := func(f *models.Flight, remarks string, correction bool) {
			reason := "r"
			f.Remarks = &remarks
			cc := repository.ChangeContext{Source: repository.ChangeSourceUserEdit, Reason: &reason, IsCorrection: correction}
			require.NoError(t, flightRepo.Update(repository.WithChangeContext(ctx, cc), f))
		}
		edit(corrected, "one", true)
		edit(corrected, "two", false)
		edit(corrected, "three", true)
		edit(plain, "one", false)
		require.NoError(t, flightRepo.Delete(repository.WithChangeContext(ctx, repository.ChangeContext{
			Source: repository.ChangeSourceUserEdit, IsCorrection: true}), deleted.ID))

		last, err := logbookRepo.LastCorrections(ctx, userID)
		require.NoError(t, err)
		require.Len(t, last, 1)

		var want time.Time
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT MAX(changed_at) FROM flight_revisions WHERE flight_id = $1 AND is_correction`, corrected.ID).Scan(&want))
		assert.True(t, want.Equal(last[corrected.ID]), "got %v want %v", last[corrected.ID], want)
	})
}
