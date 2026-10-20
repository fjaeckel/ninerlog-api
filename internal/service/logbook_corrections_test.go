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

// correctionEnv is a logbook environment with a FlightService wired to it.
type correctionEnv struct {
	*logbookEnv
	svc *FlightService
}

func newCorrectionEnv(t *testing.T, strict bool, licenceCountries ...string) *correctionEnv {
	t.Helper()
	e := newLogbookEnv(t, strict, licenceCountries...)
	svc := NewFlightService(e.flights, nil)
	svc.SetLogbookService(e.logbook)
	return &correctionEnv{logbookEnv: e, svc: svc}
}

// seed stores a flight dated daysAgo days before testNow with no block times.
func (e *logbookEnv) seed(t *testing.T, daysAgo int) *models.Flight {
	t.Helper()
	return e.seedBlocks(t, daysAgo, nil, nil)
}

// seedBlocks is seed with off-block and on-block times.
func (e *logbookEnv) seedBlocks(t *testing.T, daysAgo int, off, on *string) *models.Flight {
	t.Helper()
	day := time.Date(testNow.Year(), testNow.Month(), testNow.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -daysAgo)
	f := &models.Flight{
		UserID: e.userID, Date: day, AircraftReg: "D-EABC", AircraftType: "C172",
		OffBlockTime: off, OnBlockTime: on,
		TotalTime: 60, PICTime: 60, IsPIC: true, AllLandings: 1, LandingsDay: 1, TakeoffsDay: 1,
	}
	if err := e.flights.Create(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	f.CreatedAt = testNow
	return f
}

// edited returns a copy of the stored flight with the given mutation applied.
func (e *logbookEnv) edited(t *testing.T, id uuid.UUID, mutate func(*models.Flight)) *models.Flight {
	t.Helper()
	stored, err := e.flights.GetByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	cp := *stored
	mutate(&cp)
	return &cp
}

func strp(s string) *string { return &s }

func TestFlightService_UpdateClassification(t *testing.T) {
	tests := []struct {
		name           string
		strict         bool
		countries      []string
		daysAgo        int
		off, on        *string
		prepare        func(*testing.T, *correctionEnv, *models.Flight)
		mutate         func(*models.Flight)
		reason         string
		source         string
		wantErr        error
		wantCorrection bool
		wantReason     string
		wantWrite      bool
	}{
		{
			name: "edit on the flight day is free", strict: true, daysAgo: 0,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantWrite: true,
		},
		{
			name: "edit after the flight day needs a reason", strict: true, daysAgo: 1,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantErr: ErrCorrectionReasonRequired,
		},
		{
			name: "blank reason is no reason", strict: true, daysAgo: 1,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, reason: "   ", wantErr: ErrCorrectionReasonRequired,
		},
		{
			name: "late edit with a reason is a correction", strict: true, daysAgo: 3,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, reason: " typo ",
			wantCorrection: true, wantReason: "typo", wantWrite: true,
		},
		{
			name: "non-strict user is never classified", strict: false, daysAgo: 30,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantWrite: true,
		},
		{
			name: "system sources are never classified", strict: true, daysAgo: 30,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, source: repository.ChangeSourceRecalc, wantWrite: true,
		},
		{
			name: "no-op save after the window commits without a reason", strict: true, daysAgo: 30,
			mutate: func(f *models.Flight) {}, wantCorrection: true, wantWrite: true,
		},
		{
			name: "CH landing inside 48 hours is free even after the flight day", strict: true, countries: []string{"CH"},
			daysAgo: 1, off: strp("08:00"), on: strp("14:00"),
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantWrite: true,
		},
		{
			name: "CH landing past 48 hours needs a reason", strict: true, countries: []string{"CH"},
			daysAgo: 5, off: strp("08:00"), on: strp("14:00"),
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantErr: ErrCorrectionReasonRequired,
		},
		{
			name: "earliest deadline across licences wins", strict: true, countries: []string{"CH", "DE"},
			daysAgo: 1,
			mutate:  func(f *models.Flight) { f.Remarks = strp("x") }, wantErr: ErrCorrectionReasonRequired,
		},
		{
			name: "flight inside a certified range is a correction", strict: true, daysAgo: 0,
			prepare: func(t *testing.T, e *correctionEnv, f *models.Flight) {
				e.lbRepo.certs = append(e.lbRepo.certs, models.LogbookCertification{UserID: e.userID, CertifiedThrough: f.Date})
			},
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantErr: ErrCorrectionReasonRequired,
		},
		{
			name: "moving a flight out of a certified range is a correction", strict: true, daysAgo: 0,
			prepare: func(t *testing.T, e *correctionEnv, f *models.Flight) {
				e.lbRepo.certs = append(e.lbRepo.certs, models.LogbookCertification{UserID: e.userID, CertifiedThrough: f.Date.AddDate(0, 0, -1)})
			},
			mutate: func(f *models.Flight) { f.Date = f.Date.AddDate(0, 0, -2) }, wantErr: ErrCorrectionReasonRequired,
		},
		{
			name: "flight after the certified date is free", strict: true, daysAgo: 0,
			prepare: func(t *testing.T, e *correctionEnv, f *models.Flight) {
				e.lbRepo.certs = append(e.lbRepo.certs, models.LogbookCertification{UserID: e.userID, CertifiedThrough: f.Date.AddDate(0, 0, -1)})
			},
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantWrite: true,
		},
		{
			name: "signature history makes an edit a correction", strict: true, daysAgo: 0,
			prepare: func(t *testing.T, e *correctionEnv, f *models.Flight) {
				e.sigs.byID[uuid.New()] = &models.FlightSignature{FlightID: f.ID, Status: models.SignatureStatusCompleted}
			},
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantErr: ErrCorrectionReasonRequired,
		},
		{
			name: "void reason is the default correction reason", strict: true, daysAgo: 0,
			prepare: func(t *testing.T, e *correctionEnv, f *models.Flight) {
				voidedAt := testNow.Add(-time.Hour)
				e.sigs.byID[uuid.New()] = &models.FlightSignature{
					FlightID: f.ID, Status: models.SignatureStatusVoided, VoidedAt: &voidedAt, VoidedReason: strp("wrong landing count"),
				}
			},
			mutate: func(f *models.Flight) { f.AllLandings = 2 }, wantCorrection: true, wantReason: "wrong landing count", wantWrite: true,
		},
		{
			name: "raising an override after the flight day is a correction", strict: true, daysAgo: 1,
			mutate:  func(f *models.Flight) { f.NightTime = 10; f.NightTimeOverride = true },
			wantErr: ErrCorrectionReasonRequired,
		},
		{
			name: "raising an override on the flight day is free", strict: true, daysAgo: 0,
			mutate: func(f *models.Flight) { f.NightTime = 10; f.NightTimeOverride = true }, wantWrite: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newCorrectionEnv(t, tt.strict, tt.countries...)
			stored := e.seedBlocks(t, tt.daysAgo, tt.off, tt.on)
			if tt.prepare != nil {
				tt.prepare(t, e, stored)
			}
			updated := e.edited(t, stored.ID, tt.mutate)

			err := e.svc.UpdateFlightWith(context.Background(), updated, e.userID, FlightWriteOptions{
				CorrectionReason: tt.reason, Source: tt.source,
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if len(e.flights.writes) != 0 {
					t.Fatalf("a rejected edit wrote %d times", len(e.flights.writes))
				}
				return
			}
			if !tt.wantWrite || len(e.flights.writes) != 1 {
				t.Fatalf("writes = %d", len(e.flights.writes))
			}
			cc := e.flights.writes[0]
			wantSource := tt.source
			if wantSource == "" {
				wantSource = repository.ChangeSourceUserEdit
			}
			if cc.Source != wantSource {
				t.Errorf("source = %q, want %q", cc.Source, wantSource)
			}
			if cc.IsCorrection != tt.wantCorrection {
				t.Errorf("correction = %v, want %v", cc.IsCorrection, tt.wantCorrection)
			}
			gotReason := ""
			if cc.Reason != nil {
				gotReason = *cc.Reason
			}
			if gotReason != tt.wantReason {
				t.Errorf("reason = %q, want %q", gotReason, tt.wantReason)
			}
		})
	}
}

func TestFlightService_UpdateCrewInOneWrite(t *testing.T) {
	e := newCorrectionEnv(t, true)
	stored := e.seed(t, 0)
	updated := e.edited(t, stored.ID, func(f *models.Flight) { f.Remarks = strp("x") })
	crew := []models.FlightCrewMember{{Name: "Pat", Role: models.CrewRole("Instructor")}}

	if err := e.svc.UpdateFlightWith(context.Background(), updated, e.userID, FlightWriteOptions{Crew: &crew}); err != nil {
		t.Fatal(err)
	}
	if len(e.flights.crews) != 1 || len(e.flights.crews[0]) != 1 {
		t.Fatalf("crew writes = %+v", e.flights.crews)
	}
}

func TestFlightService_CrewOnlyChangeIsClassified(t *testing.T) {
	e := newCorrectionEnv(t, true)
	stored := e.seed(t, 3)
	e.crew.crew[stored.ID] = []models.FlightCrewMember{{Name: "Pat", Role: models.CrewRole("Instructor")}}

	same := e.edited(t, stored.ID, func(*models.Flight) {})
	sameCrew := []models.FlightCrewMember{{Name: "Pat", Role: models.CrewRole("Instructor")}}
	if err := e.svc.UpdateFlightWith(context.Background(), same, e.userID, FlightWriteOptions{Crew: &sameCrew}); err != nil {
		t.Fatalf("unchanged crew: %v", err)
	}

	changed := e.edited(t, stored.ID, func(*models.Flight) {})
	newCrew := []models.FlightCrewMember{{Name: "Sam", Role: models.CrewRole("Student")}}
	err := e.svc.UpdateFlightWith(context.Background(), changed, e.userID, FlightWriteOptions{Crew: &newCrew})
	if !errors.Is(err, ErrCorrectionReasonRequired) {
		t.Fatalf("changed crew error = %v, want ErrCorrectionReasonRequired", err)
	}
}

func TestFlightService_OwnershipBeforeClassification(t *testing.T) {
	e := newCorrectionEnv(t, true)
	stored := e.seed(t, 30)
	updated := e.edited(t, stored.ID, func(f *models.Flight) { f.Remarks = strp("x") })

	err := e.svc.UpdateFlightWith(context.Background(), updated, uuid.New(), FlightWriteOptions{})
	if !errors.Is(err, ErrUnauthorizedFlight) {
		t.Fatalf("error = %v, want ErrUnauthorizedFlight", err)
	}
}

func TestFlightService_DeleteRecordsReasonAndCorrection(t *testing.T) {
	tests := []struct {
		name           string
		strict         bool
		daysAgo        int
		reason         string
		wantCorrection bool
		wantReason     string
	}{
		{"same-day delete is not a correction", true, 0, "dup", false, "dup"},
		{"late delete is a correction without requiring a reason", true, 5, "", true, ""},
		{"late delete keeps the reason", true, 5, " entered twice ", true, "entered twice"},
		{"non-strict delete is not classified", false, 5, "", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newCorrectionEnv(t, tt.strict)
			stored := e.seed(t, tt.daysAgo)

			if err := e.svc.DeleteFlightWithReason(context.Background(), stored.ID, e.userID, tt.reason); err != nil {
				t.Fatal(err)
			}
			cc := e.flights.writes[0]
			if cc.Source != repository.ChangeSourceUserEdit || cc.IsCorrection != tt.wantCorrection {
				t.Errorf("context = %+v", cc)
			}
			gotReason := ""
			if cc.Reason != nil {
				gotReason = *cc.Reason
			}
			if gotReason != tt.wantReason {
				t.Errorf("reason = %q, want %q", gotReason, tt.wantReason)
			}
		})
	}
}

func TestFlightService_DownwardOverrides(t *testing.T) {
	tests := []struct {
		name       string
		countries  []string
		strict     bool
		mutate     func(*models.Flight)
		wantFields []string
	}{
		{"CH rejects a raised override", []string{"CH"}, true,
			func(f *models.Flight) { f.NightTime = 10; f.NightTimeOverride = true }, []string{"nightTime"}},
		{"CH accepts an override that does not exceed the calculation", []string{"CH"}, true,
			func(f *models.Flight) { f.NightTime = 0; f.NightTimeOverride = true }, nil},
		{"CH reports every offending field", []string{"CH"}, true,
			func(f *models.Flight) {
				f.NightTime = 5
				f.NightTimeOverride = true
				f.CrossCountryTime = 5
				f.CrossCountryTimeOverride = true
			}, []string{"nightTime", "crossCountryTime"}},
		{"other licences allow raising", []string{"DE"}, true,
			func(f *models.Flight) { f.NightTime = 10; f.NightTimeOverride = true }, nil},
		{"outside strict mode nothing is checked", []string{"CH"}, false,
			func(f *models.Flight) { f.NightTime = 10; f.NightTimeOverride = true }, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name+" on update", func(t *testing.T) {
			e := newCorrectionEnv(t, tt.strict, tt.countries...)
			stored := e.seed(t, 0)
			updated := e.edited(t, stored.ID, tt.mutate)

			err := e.svc.UpdateFlightWith(context.Background(), updated, e.userID, FlightWriteOptions{})
			assertOverrideFields(t, err, tt.wantFields)
		})
		t.Run(tt.name+" on create", func(t *testing.T) {
			e := newCorrectionEnv(t, tt.strict, tt.countries...)
			f := &models.Flight{
				UserID: e.userID, Date: testNow, AircraftReg: "D-EABC", AircraftType: "C172", TotalTime: 60,
				IsPIC: true, PICTime: 60,
			}
			tt.mutate(f)

			err := e.svc.CreateFlightChecked(context.Background(), f, FlightWriteOptions{})
			assertOverrideFields(t, err, tt.wantFields)
		})
	}

	t.Run("an unchanged stored override is not rechecked", func(t *testing.T) {
		e := newCorrectionEnv(t, true, "CH")
		stored := e.seed(t, 0)
		stored.NightTime = 10
		stored.NightTimeOverride = true
		updated := e.edited(t, stored.ID, func(f *models.Flight) { f.Remarks = strp("x") })

		if err := e.svc.UpdateFlightWith(context.Background(), updated, e.userID, FlightWriteOptions{}); err != nil {
			t.Fatalf("error = %v", err)
		}
	})
}

func assertOverrideFields(t *testing.T, err error, want []string) {
	t.Helper()
	if want == nil {
		if err != nil {
			t.Fatalf("error = %v, want none", err)
		}
		return
	}
	var oe *OverrideExceedsError
	if !errors.As(err, &oe) || !errors.Is(err, ErrOverrideExceedsCalculated) {
		t.Fatalf("error = %v, want OverrideExceedsError", err)
	}
	if len(oe.Fields) != len(want) {
		t.Fatalf("fields = %v, want %v", oe.Fields, want)
	}
	for i := range want {
		if oe.Fields[i] != want[i] {
			t.Fatalf("fields = %v, want %v", oe.Fields, want)
		}
	}
}

func TestFlightService_FreeEditWindowFollowsLanding(t *testing.T) {
	tests := []struct {
		name      string
		countries []string
		daysAgo   int
		off, on   *string
		wantErr   error
	}{
		{"CH inside 48 hours of landing is free", []string{"CH"}, 2, strp("08:00"), strp("14:00"), nil},
		{"CH past 48 hours of landing needs a reason", []string{"CH"}, 2, strp("06:00"), strp("10:00"), ErrCorrectionReasonRequired},
		{"CH without block times closes at the end of the flight day", []string{"CH"}, 1, nil, nil, ErrCorrectionReasonRequired},
		{"CH without block times is free on the flight day", []string{"CH"}, 0, nil, nil, nil},
		{"CH overnight landing rolls to the next day", []string{"CH"}, 2, strp("22:00"), strp("01:00"), nil},
		{"CH overnight landing still closes 48 hours later", []string{"CH"}, 3, strp("22:00"), strp("01:00"), ErrCorrectionReasonRequired},
		{"DE late entry is a correction", []string{"DE"}, 1, strp("08:00"), strp("14:00"), ErrCorrectionReasonRequired},
		{"DE flight day entry is free", []string{"DE"}, 0, strp("08:00"), strp("14:00"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newCorrectionEnv(t, true, tt.countries...)
			stored := e.seedBlocks(t, tt.daysAgo, tt.off, tt.on)
			updated := e.edited(t, stored.ID, func(f *models.Flight) { f.Remarks = strp("x") })

			err := e.svc.UpdateFlightWith(context.Background(), updated, e.userID, FlightWriteOptions{})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestFlightService_DeleteDefaultsToTheVoidReason(t *testing.T) {
	voidedAt := testNow.Add(-time.Hour)
	tests := []struct {
		name       string
		strict     bool
		reason     string
		wantReason string
	}{
		{"no reason takes the void reason", true, "", "wrong landing count"},
		{"a given reason wins", true, "entered twice", "entered twice"},
		{"non-strict delete takes nothing", false, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newCorrectionEnv(t, tt.strict)
			stored := e.seed(t, 0)
			e.sigs.byID[uuid.New()] = &models.FlightSignature{
				FlightID: stored.ID, Status: models.SignatureStatusVoided, VoidedAt: &voidedAt, VoidedReason: strp("wrong landing count"),
			}

			if err := e.svc.DeleteFlightWithReason(context.Background(), stored.ID, e.userID, tt.reason); err != nil {
				t.Fatal(err)
			}
			cc := e.flights.writes[0]
			gotReason := ""
			if cc.Reason != nil {
				gotReason = *cc.Reason
			}
			if gotReason != tt.wantReason {
				t.Errorf("reason = %q, want %q", gotReason, tt.wantReason)
			}
			if cc.IsCorrection != tt.strict {
				t.Errorf("correction = %v, want %v", cc.IsCorrection, tt.strict)
			}
		})
	}
}

// txRepos serves a logbook service's reads in place of a transaction.
type txRepos struct {
	logbook  repository.LogbookRepository
	licenses repository.LicenseRepository
}

func (r txRepos) Contacts() repository.ContactRepository { return nil }
func (r txRepos) Logbook() repository.LogbookRepository  { return r.logbook }
func (r txRepos) Licenses() repository.LicenseRepository { return r.licenses }

func TestLogbookService_DeleteCorrections(t *testing.T) {
	ctx := context.Background()
	today := time.Date(testNow.Year(), testNow.Month(), testNow.Day(), 0, 0, 0, 0, time.UTC)
	flight := func(daysAgo int) *models.Flight {
		return &models.Flight{ID: uuid.New(), Date: today.AddDate(0, 0, -daysAgo)}
	}
	fresh, late, certified := flight(0), flight(3), flight(0)
	signed := flight(0)
	sigID := uuid.New()
	signed.SignatureID = &sigID
	list := func(fs ...*models.Flight) repository.FlightLister {
		return func() ([]*models.Flight, error) { return fs, nil }
	}

	t.Run("strict account flags late, certified and signed flights", func(t *testing.T) {
		e := newLogbookEnv(t, true)
		e.lbRepo.certs = append(e.lbRepo.certs, models.LogbookCertification{UserID: e.userID, CertifiedThrough: today.AddDate(0, 0, -1)})
		certified.Date = today.AddDate(0, 0, -2)

		got, err := e.logbook.DeleteCorrections(ctx, txRepos{logbook: e.lbRepo, licenses: e.licenses}, e.userID, list(fresh, late, certified, signed))
		if err != nil {
			t.Fatal(err)
		}
		want := map[uuid.UUID]bool{late.ID: true, certified.ID: true, signed.ID: true}
		if len(got) != len(want) {
			t.Fatalf("corrections = %v", got)
		}
		for _, id := range got {
			if !want[id] {
				t.Errorf("unexpected correction %s", id)
			}
		}
	})

	t.Run("standard account lists nothing", func(t *testing.T) {
		e := newLogbookEnv(t, false)
		listed := false
		got, err := e.logbook.DeleteCorrections(ctx, txRepos{logbook: e.lbRepo, licenses: e.licenses}, e.userID,
			func() ([]*models.Flight, error) { listed = true; return nil, nil })
		if err != nil || got != nil || listed {
			t.Fatalf("got=%v listed=%v err=%v", got, listed, err)
		}
	})

	t.Run("state is read through the repositories handed in", func(t *testing.T) {
		e := newLogbookEnv(t, true)
		tx := newFakeLogbookRepo()
		tx.mode(e.userID).Strict = true

		if _, err := e.logbook.DeleteCorrections(ctx, txRepos{logbook: tx, licenses: e.licenses}, e.userID, list(late)); err != nil {
			t.Fatal(err)
		}
		if tx.getModeCalls != 1 || e.lbRepo.getModeCalls != 0 {
			t.Errorf("GetMode calls: transaction %d, base %d; want 1 and 0", tx.getModeCalls, e.lbRepo.getModeCalls)
		}
	})
}

func TestFlightService_DeleteAllFlightsCarriesTheClassifier(t *testing.T) {
	tests := []struct {
		name       string
		withLogbok bool
		want       bool
	}{
		{"with the logbook service", true, true},
		{"without it", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newCorrectionEnv(t, true)
			svc := NewFlightService(e.flights, nil)
			if tt.withLogbok {
				svc.SetLogbookService(e.logbook)
			}
			spy := &classifierSpyRepo{recordingFlightRepo: e.flights}
			svc.flightRepo = spy

			if _, err := svc.DeleteAllFlights(context.Background(), e.userID); err != nil {
				t.Fatal(err)
			}
			if spy.hasClassifier != tt.want {
				t.Errorf("classifier carried = %v, want %v", spy.hasClassifier, tt.want)
			}
			if cc, _ := repository.ChangeContextFrom(spy.ctx); cc.Source != repository.ChangeSourceUserEdit {
				t.Errorf("source = %q", cc.Source)
			}
		})
	}
}

type classifierSpyRepo struct {
	*recordingFlightRepo
	ctx           context.Context
	hasClassifier bool
}

func (r *classifierSpyRepo) DeleteAllByUserID(ctx context.Context, userID uuid.UUID) (int64, error) {
	r.ctx = ctx
	_, r.hasClassifier = repository.DeleteCorrectionsFrom(ctx)
	return r.recordingFlightRepo.DeleteAllByUserID(ctx, userID)
}

type contentSpyRepo struct {
	repository.UserContentRepository
	classifiers []bool
}

func (r *contentSpyRepo) DeleteAllContent(ctx context.Context, _ uuid.UUID) error {
	_, ok := repository.DeleteCorrectionsFrom(ctx)
	r.classifiers = append(r.classifiers, ok)
	return nil
}

func (r *contentSpyRepo) DeleteFlightsAndHistory(ctx context.Context, _ uuid.UUID) (int64, error) {
	_, ok := repository.DeleteCorrectionsFrom(ctx)
	r.classifiers = append(r.classifiers, ok)
	return 0, nil
}

func TestUserDataService_BulkDeletesCarryTheClassifier(t *testing.T) {
	e := newLogbookEnv(t, true)
	for _, withLogbook := range []bool{true, false} {
		spy := &contentSpyRepo{}
		svc := NewUserDataService(spy)
		if withLogbook {
			svc.SetLogbookService(e.logbook)
		}
		if err := svc.DeleteAll(context.Background(), e.userID); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.DeleteFlights(context.Background(), e.userID); err != nil {
			t.Fatal(err)
		}
		if len(spy.classifiers) != 2 || spy.classifiers[0] != withLogbook || spy.classifiers[1] != withLogbook {
			t.Errorf("withLogbook=%v: classifier carried = %v", withLogbook, spy.classifiers)
		}
	}
}
