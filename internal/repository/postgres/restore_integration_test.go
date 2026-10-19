//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/repository/postgres"
	"github.com/fjaeckel/ninerlog-api/internal/testutil"
	"github.com/google/uuid"
)

func ptrTo[T any](v T) *T { return &v }

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

// fullFlight sets every column a backup carries, override flags included.
func fullFlight(userID uuid.UUID, n int) *models.Flight {
	return &models.Flight{
		ID:          uuid.New(),
		UserID:      userID,
		Date:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, n),
		AircraftReg: fmt.Sprintf("D-E%03d", n%1000), AircraftType: "C172",
		DepartureICAO: ptrTo("EDDF"), ArrivalICAO: ptrTo("EDDM"),
		OffBlockTime: ptrTo("08:00:00"), OnBlockTime: ptrTo("09:00:00"),
		DepartureTime: ptrTo("08:05:00"), ArrivalTime: ptrTo("08:55:00"),
		TotalTime: 60, IsPIC: true, IsDual: false, PICTime: 60, DualTime: 0, NightTime: 10, IFRTime: 5,
		LandingsDay: 2, LandingsNight: 1, AllLandings: 3, TakeoffsDay: 2, TakeoffsNight: 1,
		Route: ptrTo("EDDF,EDDM"), SoloTime: 3, CrossCountryTime: 55, Distance: 163.5,
		TakeoffsDayOverride: true, TakeoffsNightOverride: true, LandingsDayOverride: true, LandingsNightOverride: true,
		SICTimeOverride: true, MultiPilotTimeOverride: true, NightTimeOverride: true, CrossCountryTimeOverride: true,
		Remarks: ptrTo("remarks"), InstructorName: ptrTo("Sam Carter"), InstructorComments: ptrTo("good"), PICName: ptrTo("Self"),
		SICTime: 4, DualGivenTime: 6, SimulatedFlightTime: 7, GroundTrainingTime: 8, MultiPilotTime: 9,
		PICUSTime: 11, SPICTime: 12, ExaminerTime: 13, ReliefTime: 14,
		FSTDType: ptrTo("FNPT II"), IsSimulator: false, IsPassenger: false,
		ActualInstrumentTime: 15, SimulatedInstrumentTime: 16, Holds: 2, ApproachesCount: 1,
		Approaches: []models.ApproachEntry{{Type: "ILS", Airport: ptrTo("EDDM"), Runway: ptrTo("26L")}},
		IsIPC:      true, IsFlightReview: true, IsProficiencyCheck: true,
		Endorsements: ptrTo("night"), LaunchMethod: ptrTo("winch"),
	}
}

func TestRestoreStoreIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	store := postgres.NewRestoreStore(db, time.Minute)

	newUser := func(t *testing.T) uuid.UUID {
		t.Helper()
		u := testutil.CreateTestUser("restore-"+uuid.NewString()+"@example.com", "Restore User", "hash")
		if err := userRepo.Create(ctx, u); err != nil {
			t.Fatalf("create user: %v", err)
		}
		return u.ID
	}

	t.Run("flights, contacts and crew are written in batches and read back whole", func(t *testing.T) {
		userID := newUser(t)
		const flights = 1203 // more than two statement chunks

		contact := &models.Contact{ID: uuid.New(), UserID: userID, Name: "Sam Carter", Email: ptrTo("sam@example.invalid")}
		batch := make([]*models.Flight, flights)
		var crew []models.FlightCrewMember
		for i := range batch {
			batch[i] = fullFlight(userID, i)
			crew = append(crew, models.FlightCrewMember{
				ID: uuid.New(), FlightID: batch[i].ID, ContactID: &contact.ID, Name: contact.Name, Role: models.CrewRoleInstructor,
			})
		}

		err := store.InRestoreTx(ctx, userID, func(tx repository.RestoreTx) error {
			if err := tx.InsertContacts(ctx, []*models.Contact{contact}); err != nil {
				return err
			}
			if err := tx.InsertFlights(ctx, batch); err != nil {
				return err
			}
			return tx.InsertCrewMembers(ctx, crew)
		})
		if err != nil {
			t.Fatalf("restore tx: %v", err)
		}

		if got := countRows(t, db, `SELECT COUNT(*) FROM flights WHERE user_id = $1`, userID); got != flights {
			t.Fatalf("flights = %d, want %d", got, flights)
		}
		if got := countRows(t, db, `SELECT COUNT(*) FROM flight_crew_members WHERE contact_id = $1`, contact.ID); got != flights {
			t.Fatalf("crew rows = %d, want %d", got, flights)
		}

		flightRepo := postgres.NewFlightRepository(db)
		got, err := flightRepo.GetByID(ctx, batch[7].ID)
		if err != nil {
			t.Fatalf("read back flight: %v", err)
		}
		want := *batch[7]
		if !got.Date.Equal(want.Date) {
			t.Errorf("date = %v, want %v", got.Date, want.Date)
		}
		got.CreatedAt, got.UpdatedAt = time.Time{}, time.Time{}
		got.Date, want.Date = time.Time{}, time.Time{}
		if !reflect.DeepEqual(*got, want) {
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(want)
			t.Errorf("flight did not survive the batch insert\n got: %s\nwant: %s", gotJSON, wantJSON)
		}
	})

	t.Run("an error rolls every write back", func(t *testing.T) {
		userID := newUser(t)
		boom := errors.New("boom")

		err := store.InRestoreTx(ctx, userID, func(tx repository.RestoreTx) error {
			if err := tx.InsertAircraft(ctx, &models.Aircraft{UserID: userID, Registration: "D-EROL", Type: "C172", Make: "Cessna", Model: "172", IsActive: true}); err != nil {
				return err
			}
			if err := tx.InsertFlights(ctx, []*models.Flight{fullFlight(userID, 1), fullFlight(userID, 2)}); err != nil {
				return err
			}
			return boom
		})

		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want boom", err)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM flights WHERE user_id = $1`, userID); n != 0 {
			t.Errorf("flights after rollback = %d, want 0", n)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM aircraft WHERE user_id = $1`, userID); n != 0 {
			t.Errorf("aircraft after rollback = %d, want 0", n)
		}
	})

	t.Run("a failing statement in a batch rolls the whole restore back", func(t *testing.T) {
		userID := newUser(t)
		dup := fullFlight(userID, 1)

		err := store.InRestoreTx(ctx, userID, func(tx repository.RestoreTx) error {
			if err := tx.InsertAircraft(ctx, &models.Aircraft{UserID: userID, Registration: "D-EFAI", Type: "C172", Make: "Cessna", Model: "172", IsActive: true}); err != nil {
				return err
			}
			return tx.InsertFlights(ctx, []*models.Flight{dup, dup})
		})

		if err == nil {
			t.Fatal("duplicate flight ids were accepted")
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM aircraft WHERE user_id = $1`, userID); n != 0 {
			t.Errorf("aircraft after failed batch = %d, want 0", n)
		}
	})

	t.Run("count content reports each collection and ignores single-row settings", func(t *testing.T) {
		userID := newUser(t)
		var content repository.RestoreContent
		read := func() repository.RestoreContent {
			var c repository.RestoreContent
			if err := store.InRestoreTx(ctx, userID, func(tx repository.RestoreTx) error {
				var err error
				c, err = tx.CountContent(ctx, userID)
				return err
			}); err != nil {
				t.Fatalf("count content: %v", err)
			}
			return c
		}

		if content = read(); !content.Empty() {
			t.Fatalf("a fresh account is not empty: %+v", content)
		}

		if err := store.InRestoreTx(ctx, userID, func(tx repository.RestoreTx) error {
			if err := tx.UpsertNotificationPreferences(ctx, &models.NotificationPreferences{
				UserID: userID, EmailEnabled: true, EnabledCategories: []string{"rating_expiry"}, WarningDays: []int64{30}, CheckHour: 8,
			}); err != nil {
				return err
			}
			return tx.UpsertFlightBaseline(ctx, &models.FlightBaseline{UserID: userID, BaselineDate: time.Now().AddDate(-1, 0, 0), TotalMinutes: 60})
		}); err != nil {
			t.Fatalf("settings: %v", err)
		}
		if content = read(); !content.Empty() {
			t.Fatalf("single-row settings made the account non-empty: %+v", content)
		}

		rule := &models.CustomCurrencyRule{UserID: userID, Name: "r", Enabled: true, Definition: models.CustomCurrencyRuleBody{
			Window: models.CurrencyWindow{Amount: 90, Unit: "days"}, Requirements: []models.CurrencyRequirement{{Metric: "landings", Min: 3}},
		}}
		report := &models.CustomReport{UserID: userID, Name: "rep", Definition: models.CustomReportDefinition{
			Window: models.CustomReportWindow{Kind: models.ReportWindowAll}, GroupBy: models.ReportGroupYear, Metric: models.ReportMetrics[0],
		}}
		if err := store.InRestoreTx(ctx, userID, func(tx repository.RestoreTx) error {
			if err := tx.InsertCustomCurrencyRule(ctx, rule); err != nil {
				return err
			}
			return tx.InsertCustomReport(ctx, report)
		}); err != nil {
			t.Fatalf("rule and report: %v", err)
		}
		content = read()
		if !content.CustomCurrencyRules || !content.CustomReports || content.Flights || content.Aircraft {
			t.Fatalf("content = %+v, want only rules and reports", content)
		}
	})

	t.Run("wipe removes every restored collection and nothing else", func(t *testing.T) {
		userID, other := newUser(t), newUser(t)
		seed := func(uid uuid.UUID) {
			err := store.InRestoreTx(ctx, uid, func(tx repository.RestoreTx) error {
				lic := &models.License{UserID: uid, RegulatoryAuthority: "EASA", LicenseType: "PPL", LicenseNumber: "L-" + uid.String(), IssueDate: time.Now(), IssuingAuthority: "LBA"}
				if err := tx.InsertLicense(ctx, lic); err != nil {
					return err
				}
				if err := tx.InsertClassRating(ctx, &models.ClassRating{LicenseID: lic.ID, ClassType: models.ClassTypeSEPLand, IssueDate: time.Now()}); err != nil {
					return err
				}
				if err := tx.InsertCredential(ctx, &models.Credential{UserID: uid, CredentialType: models.CredentialTypeEASAClass2Medical, IssueDate: time.Now(), IssuingAuthority: "AME"}); err != nil {
					return err
				}
				if err := tx.InsertAircraft(ctx, &models.Aircraft{UserID: uid, Registration: "D-EWIP", Type: "C172", Make: "Cessna", Model: "172", IsActive: true}); err != nil {
					return err
				}
				contact := &models.Contact{ID: uuid.New(), UserID: uid, Name: "Wipe Me"}
				if err := tx.InsertContacts(ctx, []*models.Contact{contact}); err != nil {
					return err
				}
				f := fullFlight(uid, 1)
				if err := tx.InsertFlights(ctx, []*models.Flight{f}); err != nil {
					return err
				}
				if err := tx.InsertCrewMembers(ctx, []models.FlightCrewMember{{ID: uuid.New(), FlightID: f.ID, ContactID: &contact.ID, Name: "Wipe Me", Role: models.CrewRolePIC}}); err != nil {
					return err
				}
				if err := tx.InsertCustomCurrencyRule(ctx, &models.CustomCurrencyRule{UserID: uid, Name: "r", Definition: models.CustomCurrencyRuleBody{
					Window: models.CurrencyWindow{Amount: 90, Unit: "days"}, Requirements: []models.CurrencyRequirement{{Metric: "landings", Min: 3}},
				}}); err != nil {
					return err
				}
				if err := tx.InsertCustomReport(ctx, &models.CustomReport{UserID: uid, Name: "rep", Definition: models.CustomReportDefinition{
					Window: models.CustomReportWindow{Kind: models.ReportWindowAll}, GroupBy: models.ReportGroupYear, Metric: models.ReportMetrics[0],
				}}); err != nil {
					return err
				}
				if err := tx.UpsertNotificationPreferences(ctx, &models.NotificationPreferences{
					UserID: uid, EnabledCategories: []string{}, WarningDays: []int64{7}, CheckHour: 8,
				}); err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				t.Fatalf("seed: %v", err)
			}
			if _, err := db.Exec(`INSERT INTO flight_imports (user_id, file_name, import_format, import_status, total_rows)
				VALUES ($1, 'x.csv', 'NINERLOG_CSV', 'completed', 1)`, uid); err != nil {
				t.Fatalf("seed import history: %v", err)
			}
		}
		seed(userID)
		seed(other)

		if err := store.InRestoreTx(ctx, userID, func(tx repository.RestoreTx) error {
			return tx.WipeContent(ctx, userID)
		}); err != nil {
			t.Fatalf("wipe: %v", err)
		}

		for _, table := range []string{"flights", "aircraft", "licenses", "credentials", "contacts", "custom_currency_rules", "custom_reports", "flight_imports"} {
			if n := countRows(t, db, `SELECT COUNT(*) FROM `+table+` WHERE user_id = $1`, userID); n != 0 {
				t.Errorf("%s after wipe = %d, want 0", table, n)
			}
			if n := countRows(t, db, `SELECT COUNT(*) FROM `+table+` WHERE user_id = $1`, other); n != 1 {
				t.Errorf("%s of the other user = %d, want 1", table, n)
			}
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM class_ratings WHERE license_id IN (SELECT id FROM licenses WHERE user_id = $1)`, userID); n != 0 {
			t.Errorf("class ratings after wipe = %d", n)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM flight_crew_members WHERE flight_id IN (SELECT id FROM flights WHERE user_id = $1)`, other); n != 1 {
			t.Errorf("crew rows of the other user = %d, want 1", n)
		}
		if n := countRows(t, db, `SELECT COUNT(*) FROM notification_preferences WHERE user_id = $1`, userID); n != 1 {
			t.Errorf("notification preferences were wiped")
		}
	})

	t.Run("attachments are reported", func(t *testing.T) {
		userID := newUser(t)
		attachments := func() repository.RestoreAttachments {
			var a repository.RestoreAttachments
			if err := store.InRestoreTx(ctx, userID, func(tx repository.RestoreTx) error {
				var err error
				a, err = tx.CountAttachments(ctx, userID)
				return err
			}); err != nil {
				t.Fatalf("count attachments: %v", err)
			}
			return a
		}
		if attachments().Any() {
			t.Fatal("a fresh account reports attachments")
		}

		licenseRepo := postgres.NewLicenseRepository(db)
		license := &models.License{UserID: userID, RegulatoryAuthority: "EASA", LicenseType: "PPL", LicenseNumber: "PPL-ATT", IssueDate: time.Now(), IssuingAuthority: "LBA"}
		if err := licenseRepo.Create(ctx, license); err != nil {
			t.Fatalf("create license: %v", err)
		}
		fileRepo := postgres.NewDocumentFileRepository(db)
		data := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 1, 2, 3, 4}
		if err := fileRepo.Create(ctx, &models.DocumentFile{
			UserID: userID, LicenseID: &license.ID, ContentType: "image/png", ByteSize: len(data), Data: data,
		}, models.MaxDocumentFilesPerSubject); err != nil {
			t.Fatalf("create document file: %v", err)
		}
		if a := attachments(); !a.DocumentFiles || a.SignedFlights {
			t.Fatalf("attachments = %+v, want only document files", a)
		}

		flight := fullFlight(userID, 1)
		if err := store.InRestoreTx(ctx, userID, func(tx repository.RestoreTx) error {
			return tx.InsertFlights(ctx, []*models.Flight{flight})
		}); err != nil {
			t.Fatalf("insert flight: %v", err)
		}
		var sigID uuid.UUID
		if err := db.QueryRow(`INSERT INTO flight_signatures (flight_id, user_id, method, status)
			VALUES ($1, $2, 'live', 'completed') RETURNING id`, flight.ID, userID).Scan(&sigID); err != nil {
			t.Fatalf("insert signature: %v", err)
		}
		if _, err := db.Exec(`UPDATE flights SET signature_id = $1 WHERE id = $2`, sigID, flight.ID); err != nil {
			t.Fatalf("lock flight: %v", err)
		}
		if a := attachments(); !a.DocumentFiles || !a.SignedFlights {
			t.Fatalf("attachments = %+v, want both", a)
		}
	})

	t.Run("a restore for one user waits for another restore of the same user", func(t *testing.T) {
		userID := newUser(t)
		entered := make(chan struct{})
		release := make(chan struct{})
		firstDone := make(chan error, 1)

		go func() {
			firstDone <- store.InRestoreTx(ctx, userID, func(tx repository.RestoreTx) error {
				close(entered)
				<-release
				return nil
			})
		}()
		<-entered

		secondRan := make(chan struct{})
		go func() {
			_ = store.InRestoreTx(ctx, userID, func(tx repository.RestoreTx) error {
				close(secondRan)
				return nil
			})
		}()

		select {
		case <-secondRan:
			t.Fatal("a second restore for the same user ran while the first was open")
		case <-time.After(300 * time.Millisecond):
		}
		close(release)
		if err := <-firstDone; err != nil {
			t.Fatalf("first restore: %v", err)
		}
		select {
		case <-secondRan:
		case <-time.After(5 * time.Second):
			t.Fatal("the second restore never ran")
		}
	})
}

func TestUserContentRepositoryDeleteFlightsAndHistoryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	userRepo := postgres.NewUserRepository(db)
	store := postgres.NewRestoreStore(db, time.Minute)
	content := postgres.NewUserContentRepository(db)

	user := testutil.CreateTestUser("flightwipe-"+uuid.NewString()+"@example.com", "Wipe", "hash")
	if err := userRepo.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	f1, f2 := fullFlight(user.ID, 1), fullFlight(user.ID, 2)
	if err := store.InRestoreTx(ctx, user.ID, func(tx repository.RestoreTx) error {
		if err := tx.InsertAircraft(ctx, &models.Aircraft{UserID: user.ID, Registration: "D-EKEP", Type: "C172", Make: "Cessna", Model: "172", IsActive: true}); err != nil {
			return err
		}
		if err := tx.InsertFlights(ctx, []*models.Flight{f1, f2}); err != nil {
			return err
		}
		return tx.InsertCrewMembers(ctx, []models.FlightCrewMember{{ID: uuid.New(), FlightID: f1.ID, Name: "Crew", Role: models.CrewRolePIC}})
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO flight_imports (user_id, file_name, import_format, import_status, total_rows)
		VALUES ($1, 'x.csv', 'NINERLOG_CSV', 'completed', 1)`, user.ID); err != nil {
		t.Fatalf("seed import history: %v", err)
	}

	deleted, err := content.DeleteFlightsAndHistory(ctx, user.ID)
	if err != nil {
		t.Fatalf("delete flights: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}
	for _, q := range []string{
		`SELECT COUNT(*) FROM flights WHERE user_id = $1`,
		`SELECT COUNT(*) FROM flight_imports WHERE user_id = $1`,
	} {
		if n := countRows(t, db, q, user.ID); n != 0 {
			t.Errorf("%q = %d, want 0", q, n)
		}
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM aircraft WHERE user_id = $1`, user.ID); n != 1 {
		t.Errorf("aircraft = %d, want 1: scope flights must keep the rest", n)
	}
}
