package service_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

type mockReminderRepo struct {
	rows      map[uuid.UUID]*models.AircraftReminder
	aircraft  *mockAircraftRepo
	lastBound *time.Time
}

func newMockReminderRepo(ac *mockAircraftRepo) *mockReminderRepo {
	return &mockReminderRepo{rows: map[uuid.UUID]*models.AircraftReminder{}, aircraft: ac}
}

func (m *mockReminderRepo) withReg(r *models.AircraftReminder) *models.AircraftReminder {
	cp := *r
	if a, ok := m.aircraft.aircraft[r.AircraftID]; ok {
		cp.AircraftRegistration = a.Registration
	}
	return &cp
}

func (m *mockReminderRepo) Create(_ context.Context, r *models.AircraftReminder) error {
	r.ID = uuid.New()
	r.CreatedAt = time.Now()
	r.UpdatedAt = r.CreatedAt
	cp := *r
	m.rows[r.ID] = &cp
	return nil
}

func (m *mockReminderRepo) GetByID(_ context.Context, id uuid.UUID) (*models.AircraftReminder, error) {
	r, ok := m.rows[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return m.withReg(r), nil
}

func (m *mockReminderRepo) sorted(keep func(*models.AircraftReminder) bool) []*models.AircraftReminder {
	out := []*models.AircraftReminder{}
	for _, r := range m.rows {
		if keep(r) {
			out = append(out, m.withReg(r))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DueDate.Before(out[j].DueDate) })
	return out
}

func (m *mockReminderRepo) ListByAircraft(_ context.Context, aircraftID uuid.UUID) ([]*models.AircraftReminder, error) {
	return m.sorted(func(r *models.AircraftReminder) bool { return r.AircraftID == aircraftID }), nil
}

func (m *mockReminderRepo) ListByUser(_ context.Context, userID uuid.UUID, bound *time.Time) ([]*models.AircraftReminder, error) {
	m.lastBound = bound
	return m.sorted(func(r *models.AircraftReminder) bool {
		return r.UserID == userID && (bound == nil || !r.DueDate.After(*bound))
	}), nil
}

func (m *mockReminderRepo) Update(_ context.Context, r *models.AircraftReminder) error {
	if _, ok := m.rows[r.ID]; !ok {
		return repository.ErrNotFound
	}
	r.UpdatedAt = time.Now()
	cp := *r
	m.rows[r.ID] = &cp
	return nil
}

func (m *mockReminderRepo) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.rows[id]; !ok {
		return repository.ErrNotFound
	}
	delete(m.rows, id)
	return nil
}

type reminderFixture struct {
	svc      *service.AircraftReminderService
	repo     *mockReminderRepo
	mehmet   uuid.UUID
	other    uuid.UUID
	c42      uuid.UUID
	otherAC  uuid.UUID
	fixedNow time.Time
}

func newReminderFixture(t *testing.T) *reminderFixture {
	t.Helper()
	acRepo := newMockAircraftRepo()
	f := &reminderFixture{
		mehmet:   uuid.New(),
		other:    uuid.New(),
		c42:      uuid.New(),
		otherAC:  uuid.New(),
		fixedNow: time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC),
	}
	acRepo.aircraft[f.c42] = &models.Aircraft{ID: f.c42, UserID: f.mehmet, Registration: "D-MXYZ"}
	acRepo.aircraft[f.otherAC] = &models.Aircraft{ID: f.otherAC, UserID: f.other, Registration: "D-EFGH"}
	f.repo = newMockReminderRepo(acRepo)
	f.svc = service.NewAircraftReminderService(f.repo, acRepo)
	f.svc.SetClock(func() time.Time { return f.fixedNow })
	return f
}

func rdate(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func intp(i int) *int { return &i }

func TestAircraftReminderService_Create(t *testing.T) {
	label := "Hull policy"
	tests := []struct {
		name    string
		user    func(f *reminderFixture) uuid.UUID
		ac      func(f *reminderFixture) uuid.UUID
		in      service.AircraftReminderInput
		wantErr error
	}{
		{
			name: "M job 3: Jahresnachprüfung on D-MXYZ",
			user: func(f *reminderFixture) uuid.UUID { return f.mehmet },
			ac:   func(f *reminderFixture) uuid.UUID { return f.c42 },
			in:   service.AircraftReminderInput{Kind: models.ReminderKindAnnualInspection, DueDate: rdate("2026-10-15"), IntervalMonths: intp(12)},
		},
		{
			name: "insurance with label",
			user: func(f *reminderFixture) uuid.UUID { return f.mehmet },
			ac:   func(f *reminderFixture) uuid.UUID { return f.c42 },
			in:   service.AircraftReminderInput{Kind: models.ReminderKindInsurance, Label: &label, DueDate: rdate("2027-01-01")},
		},
		{
			name:    "custom without label",
			user:    func(f *reminderFixture) uuid.UUID { return f.mehmet },
			ac:      func(f *reminderFixture) uuid.UUID { return f.c42 },
			in:      service.AircraftReminderInput{Kind: models.ReminderKindCustom, DueDate: rdate("2027-01-01")},
			wantErr: models.ErrInvalidAircraftReminder,
		},
		{
			name:    "interval out of range",
			user:    func(f *reminderFixture) uuid.UUID { return f.mehmet },
			ac:      func(f *reminderFixture) uuid.UUID { return f.c42 },
			in:      service.AircraftReminderInput{Kind: models.ReminderKindARC, DueDate: rdate("2027-01-01"), IntervalMonths: intp(0)},
			wantErr: models.ErrInvalidAircraftReminder,
		},
		{
			name:    "unknown kind",
			user:    func(f *reminderFixture) uuid.UUID { return f.mehmet },
			ac:      func(f *reminderFixture) uuid.UUID { return f.c42 },
			in:      service.AircraftReminderInput{Kind: "OIL", DueDate: rdate("2027-01-01")},
			wantErr: models.ErrInvalidAircraftReminder,
		},
		{
			name:    "another user's aircraft is not found",
			user:    func(f *reminderFixture) uuid.UUID { return f.mehmet },
			ac:      func(f *reminderFixture) uuid.UUID { return f.otherAC },
			in:      service.AircraftReminderInput{Kind: models.ReminderKindARC, DueDate: rdate("2027-01-01")},
			wantErr: service.ErrAircraftNotFound,
		},
		{
			name:    "missing aircraft is not found",
			user:    func(f *reminderFixture) uuid.UUID { return f.mehmet },
			ac:      func(f *reminderFixture) uuid.UUID { return uuid.New() },
			in:      service.AircraftReminderInput{Kind: models.ReminderKindARC, DueDate: rdate("2027-01-01")},
			wantErr: service.ErrAircraftNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReminderFixture(t)
			rem, err := f.svc.Create(context.Background(), tt.user(f), tt.ac(f), tt.in)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Create() error = %v, want %v", err, tt.wantErr)
				}
				if len(f.repo.rows) != 0 {
					t.Error("nothing should be stored on error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if rem.AircraftRegistration != "D-MXYZ" {
				t.Errorf("AircraftRegistration = %q, want D-MXYZ", rem.AircraftRegistration)
			}
			if rem.UserID != f.mehmet {
				t.Error("reminder must be owned by the caller")
			}
		})
	}
}

func TestAircraftReminderService_Complete(t *testing.T) {
	tests := []struct {
		name     string
		interval *int
		due      string
		doneOn   *time.Time
		wantDue  string
		wantDone string
	}{
		{"M job 3: Jahresnachprüfung rolls 12 months from the completion date", intp(12), "2026-10-15", ptrTime(rdate("2026-10-02")), "2027-10-02", "2026-10-02"},
		{"Jan 31 + 1 month clamps to Feb 28", intp(1), "2026-01-31", ptrTime(rdate("2026-01-31")), "2026-02-28", "2026-01-31"},
		{"doneOn defaults to today", intp(6), "2026-09-30", nil, "2027-03-26", "2026-09-26"},
		{"without interval the due date is left alone", nil, "2026-10-15", ptrTime(rdate("2026-10-01")), "2026-10-15", "2026-10-01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newReminderFixture(t)
			ctx := context.Background()
			rem, err := f.svc.Create(ctx, f.mehmet, f.c42, service.AircraftReminderInput{
				Kind: models.ReminderKindAnnualInspection, DueDate: rdate(tt.due), IntervalMonths: tt.interval,
			})
			if err != nil {
				t.Fatal(err)
			}
			done, err := f.svc.Complete(ctx, f.mehmet, f.c42, rem.ID, tt.doneOn)
			if err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
			if got := done.DueDate.Format("2006-01-02"); got != tt.wantDue {
				t.Errorf("DueDate = %s, want %s", got, tt.wantDue)
			}
			if done.LastDoneOn == nil || done.LastDoneOn.Format("2006-01-02") != tt.wantDone {
				t.Errorf("LastDoneOn = %v, want %s", done.LastDoneOn, tt.wantDone)
			}
			stored := f.repo.rows[rem.ID]
			if stored.DueDate.Format("2006-01-02") != tt.wantDue {
				t.Errorf("stored DueDate = %s, want %s", stored.DueDate.Format("2006-01-02"), tt.wantDue)
			}
		})
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestAircraftReminderService_OwnershipIsNotFound(t *testing.T) {
	f := newReminderFixture(t)
	ctx := context.Background()
	mine, err := f.svc.Create(ctx, f.mehmet, f.c42, service.AircraftReminderInput{Kind: models.ReminderKindInsurance, DueDate: rdate("2027-01-01")})
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := f.svc.Create(ctx, f.other, f.otherAC, service.AircraftReminderInput{Kind: models.ReminderKindARC, DueDate: rdate("2027-01-01")})
	if err != nil {
		t.Fatal(err)
	}
	kind := models.ReminderKindARC

	tests := []struct {
		name    string
		call    func() error
		wantErr error
	}{
		{"list another user's aircraft", func() error { _, err := f.svc.List(ctx, f.mehmet, f.otherAC); return err }, service.ErrAircraftNotFound},
		{"update via another user's aircraft", func() error {
			_, err := f.svc.Update(ctx, f.mehmet, f.otherAC, theirs.ID, service.AircraftReminderPatch{Kind: &kind})
			return err
		}, service.ErrAircraftNotFound},
		{"update another user's reminder through own aircraft", func() error {
			_, err := f.svc.Update(ctx, f.mehmet, f.c42, theirs.ID, service.AircraftReminderPatch{Kind: &kind})
			return err
		}, service.ErrAircraftReminderNotFound},
		{"complete another user's reminder", func() error {
			_, err := f.svc.Complete(ctx, f.mehmet, f.c42, theirs.ID, nil)
			return err
		}, service.ErrAircraftReminderNotFound},
		{"delete another user's reminder", func() error { return f.svc.Delete(ctx, f.mehmet, f.c42, theirs.ID) }, service.ErrAircraftReminderNotFound},
		{"unknown reminder", func() error { return f.svc.Delete(ctx, f.mehmet, f.c42, uuid.New()) }, service.ErrAircraftReminderNotFound},
		{"other user cannot reach mine", func() error { return f.svc.Delete(ctx, f.other, f.c42, mine.ID) }, service.ErrAircraftNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
	if f.repo.rows[theirs.ID].Kind != models.ReminderKindARC || len(f.repo.rows) != 2 {
		t.Error("another user's reminder must be untouched")
	}
}

func TestAircraftReminderService_ReminderOnOtherAircraftIsNotFound(t *testing.T) {
	f := newReminderFixture(t)
	ctx := context.Background()
	second := uuid.New()
	f.repo.aircraft.aircraft[second] = &models.Aircraft{ID: second, UserID: f.mehmet, Registration: "D-MABC"}
	rem, err := f.svc.Create(ctx, f.mehmet, f.c42, service.AircraftReminderInput{Kind: models.ReminderKindInsurance, DueDate: rdate("2027-01-01")})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Delete(ctx, f.mehmet, second, rem.ID); !errors.Is(err, service.ErrAircraftReminderNotFound) {
		t.Fatalf("Delete via wrong aircraft = %v, want ErrAircraftReminderNotFound", err)
	}
}

func TestAircraftReminderService_Update(t *testing.T) {
	f := newReminderFixture(t)
	ctx := context.Background()
	label := "Hull"
	notes := "Policy 123"
	rem, err := f.svc.Create(ctx, f.mehmet, f.c42, service.AircraftReminderInput{
		Kind: models.ReminderKindInsurance, Label: &label, Notes: &notes, DueDate: rdate("2027-01-01"), IntervalMonths: intp(12),
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("partial update leaves omitted fields", func(t *testing.T) {
		due := rdate("2027-02-01")
		got, err := f.svc.Update(ctx, f.mehmet, f.c42, rem.ID, service.AircraftReminderPatch{DueDate: &due})
		if err != nil {
			t.Fatal(err)
		}
		if got.DueDate.Format("2006-01-02") != "2027-02-01" || got.Label == nil || *got.Label != "Hull" || got.IntervalMonths == nil {
			t.Errorf("unexpected reminder after partial update: %+v", got)
		}
	})
	t.Run("null clears nullable fields", func(t *testing.T) {
		got, err := f.svc.Update(ctx, f.mehmet, f.c42, rem.ID, service.AircraftReminderPatch{ClearNotes: true, ClearIntervalMonths: true, ClearLabel: true})
		if err != nil {
			t.Fatal(err)
		}
		if got.Notes != nil || got.IntervalMonths != nil || got.Label != nil {
			t.Errorf("fields not cleared: %+v", got)
		}
	})
	t.Run("switching to CUSTOM without label is rejected", func(t *testing.T) {
		kind := models.ReminderKindCustom
		_, err := f.svc.Update(ctx, f.mehmet, f.c42, rem.ID, service.AircraftReminderPatch{Kind: &kind})
		if !errors.Is(err, models.ErrInvalidAircraftReminder) {
			t.Fatalf("error = %v, want ErrInvalidAircraftReminder", err)
		}
		if f.repo.rows[rem.ID].Kind != models.ReminderKindInsurance {
			t.Error("rejected update must not be stored")
		}
	})
	t.Run("interval out of range is rejected", func(t *testing.T) {
		_, err := f.svc.Update(ctx, f.mehmet, f.c42, rem.ID, service.AircraftReminderPatch{IntervalMonths: intp(241)})
		if !errors.Is(err, models.ErrInvalidAircraftReminder) {
			t.Fatalf("error = %v, want ErrInvalidAircraftReminder", err)
		}
	})
}

func TestAircraftReminderService_ListAll(t *testing.T) {
	f := newReminderFixture(t)
	ctx := context.Background()
	for _, due := range []string{"2026-09-20", "2026-10-10", "2027-06-01"} {
		if _, err := f.svc.Create(ctx, f.mehmet, f.c42, service.AircraftReminderInput{Kind: models.ReminderKindELTBattery, DueDate: rdate(due)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.svc.Create(ctx, f.other, f.otherAC, service.AircraftReminderInput{Kind: models.ReminderKindARC, DueDate: rdate("2026-09-30")}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		within    *int
		wantCount int
		wantErr   error
	}{
		{"all of the caller's reminders", nil, 3, nil},
		{"within 30 days includes overdue", intp(30), 2, nil},
		{"within 0 days is overdue and today", intp(0), 1, nil},
		{"negative is rejected", intp(-1), 0, service.ErrInvalidReminderQuery},
		{"too large is rejected", intp(service.MaxReminderDueWithinDays + 1), 0, service.ErrInvalidReminderQuery},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := f.svc.ListAll(ctx, f.mehmet, tt.within)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.wantCount {
				t.Fatalf("got %d reminders, want %d", len(got), tt.wantCount)
			}
			for i := 1; i < len(got); i++ {
				if got[i].DueDate.Before(got[i-1].DueDate) {
					t.Error("reminders must be ordered by due date")
				}
			}
			for _, r := range got {
				if r.UserID != f.mehmet {
					t.Error("another user's reminder leaked")
				}
			}
		})
	}
}
