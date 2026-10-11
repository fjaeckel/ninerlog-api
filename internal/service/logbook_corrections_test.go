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
	svc.SetCrewRepository(e.crew)
	return &correctionEnv{logbookEnv: e, svc: svc}
}

// seed stores a flight dated daysAgo days before testNow, created createdAgo before testNow.
func (e *logbookEnv) seed(t *testing.T, daysAgo int, createdAgo time.Duration) *models.Flight {
	t.Helper()
	day := time.Date(testNow.Year(), testNow.Month(), testNow.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -daysAgo)
	f := &models.Flight{
		UserID: e.userID, Date: day, AircraftReg: "D-EABC", AircraftType: "C172",
		TotalTime: 60, PICTime: 60, IsPIC: true, AllLandings: 1, LandingsDay: 1, TakeoffsDay: 1,
	}
	if err := e.flights.Create(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	f.CreatedAt = testNow.Add(-createdAgo)
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
		createdAgo     time.Duration
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
			name: "edit on the flight day is free", strict: true, daysAgo: 0, createdAgo: time.Hour,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantWrite: true,
		},
		{
			name: "edit after the flight day needs a reason", strict: true, daysAgo: 1, createdAgo: 30 * time.Hour,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantErr: ErrCorrectionReasonRequired,
		},
		{
			name: "blank reason is no reason", strict: true, daysAgo: 1, createdAgo: 30 * time.Hour,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, reason: "   ", wantErr: ErrCorrectionReasonRequired,
		},
		{
			name: "late edit with a reason is a correction", strict: true, daysAgo: 3, createdAgo: 72 * time.Hour,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, reason: " typo ",
			wantCorrection: true, wantReason: "typo", wantWrite: true,
		},
		{
			name: "non-strict user is never classified", strict: false, daysAgo: 30, createdAgo: 30 * 24 * time.Hour,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantWrite: true,
		},
		{
			name: "system sources are never classified", strict: true, daysAgo: 30, createdAgo: 30 * 24 * time.Hour,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, source: repository.ChangeSourceRecalc, wantWrite: true,
		},
		{
			name: "no-op save after the window needs no reason", strict: true, daysAgo: 30, createdAgo: 30 * 24 * time.Hour,
			mutate: func(f *models.Flight) {}, wantWrite: true,
		},
		{
			name: "CH entry inside 48 hours is free even after the flight day", strict: true, countries: []string{"CH"},
			daysAgo: 1, createdAgo: 24 * time.Hour,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantWrite: true,
		},
		{
			name: "CH entry past 48 hours needs a reason", strict: true, countries: []string{"CH"},
			daysAgo: 5, createdAgo: 72 * time.Hour,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantErr: ErrCorrectionReasonRequired,
		},
		{
			name: "earliest deadline across licences wins", strict: true, countries: []string{"CH", "DE"},
			daysAgo: 1, createdAgo: 24 * time.Hour,
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantErr: ErrCorrectionReasonRequired,
		},
		{
			name: "flight inside a certified range is a correction", strict: true, daysAgo: 0, createdAgo: time.Hour,
			prepare: func(t *testing.T, e *correctionEnv, f *models.Flight) {
				e.lbRepo.certs = append(e.lbRepo.certs, models.LogbookCertification{UserID: e.userID, CertifiedThrough: f.Date})
			},
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantErr: ErrCorrectionReasonRequired,
		},
		{
			name: "moving a flight out of a certified range is a correction", strict: true, daysAgo: 0, createdAgo: time.Hour,
			prepare: func(t *testing.T, e *correctionEnv, f *models.Flight) {
				e.lbRepo.certs = append(e.lbRepo.certs, models.LogbookCertification{UserID: e.userID, CertifiedThrough: f.Date.AddDate(0, 0, -1)})
			},
			mutate: func(f *models.Flight) { f.Date = f.Date.AddDate(0, 0, -2) }, wantErr: ErrCorrectionReasonRequired,
		},
		{
			name: "flight after the certified date is free", strict: true, daysAgo: 0, createdAgo: time.Hour,
			prepare: func(t *testing.T, e *correctionEnv, f *models.Flight) {
				e.lbRepo.certs = append(e.lbRepo.certs, models.LogbookCertification{UserID: e.userID, CertifiedThrough: f.Date.AddDate(0, 0, -1)})
			},
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantWrite: true,
		},
		{
			name: "signature history makes an edit a correction", strict: true, daysAgo: 0, createdAgo: time.Hour,
			prepare: func(t *testing.T, e *correctionEnv, f *models.Flight) {
				e.sigs.byID[uuid.New()] = &models.FlightSignature{FlightID: f.ID, Status: models.SignatureStatusCompleted}
			},
			mutate: func(f *models.Flight) { f.Remarks = strp("x") }, wantErr: ErrCorrectionReasonRequired,
		},
		{
			name: "void reason is the default correction reason", strict: true, daysAgo: 0, createdAgo: time.Hour,
			prepare: func(t *testing.T, e *correctionEnv, f *models.Flight) {
				voidedAt := testNow.Add(-time.Hour)
				e.sigs.byID[uuid.New()] = &models.FlightSignature{
					FlightID: f.ID, Status: models.SignatureStatusVoided, VoidedAt: &voidedAt, VoidedReason: strp("wrong landing count"),
				}
			},
			mutate: func(f *models.Flight) { f.AllLandings = 2 }, wantCorrection: true, wantReason: "wrong landing count", wantWrite: true,
		},
		{
			name: "raising an override after the flight day is a correction", strict: true, daysAgo: 1, createdAgo: time.Hour,
			mutate:  func(f *models.Flight) { f.NightTime = 10; f.NightTimeOverride = true },
			wantErr: ErrCorrectionReasonRequired,
		},
		{
			name: "raising an override on the flight day is free", strict: true, daysAgo: 0, createdAgo: time.Hour,
			mutate: func(f *models.Flight) { f.NightTime = 10; f.NightTimeOverride = true }, wantWrite: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newCorrectionEnv(t, tt.strict, tt.countries...)
			stored := e.seed(t, tt.daysAgo, tt.createdAgo)
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
	stored := e.seed(t, 0, time.Hour)
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
	stored := e.seed(t, 3, 72*time.Hour)
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
	stored := e.seed(t, 30, 30*24*time.Hour)
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
			stored := e.seed(t, tt.daysAgo, time.Duration(tt.daysAgo)*24*time.Hour+time.Hour)

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
			stored := e.seed(t, 0, time.Hour)
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
		stored := e.seed(t, 0, time.Hour)
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
