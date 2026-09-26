package models

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func reminderDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestAddMonthsClamped(t *testing.T) {
	tests := []struct {
		name   string
		from   string
		months int
		want   string
	}{
		{"M job 3: Jahresnachprüfung 12 months", "2026-05-14", 12, "2027-05-14"},
		{"Jan 31 + 1 month clamps to Feb 28", "2026-01-31", 1, "2026-02-28"},
		{"Jan 31 + 1 month in a leap year clamps to Feb 29", "2028-01-31", 1, "2028-02-29"},
		{"Feb 29 + 12 months clamps to Feb 28", "2028-02-29", 12, "2029-02-28"},
		{"Aug 31 + 1 month clamps to Sep 30", "2026-08-31", 1, "2026-09-30"},
		{"Dec 15 + 1 month crosses year", "2026-12-15", 1, "2027-01-15"},
		{"Mar 31 + 6 months clamps to Sep 30", "2026-03-31", 6, "2026-09-30"},
		{"repack 72 months", "2026-06-01", 72, "2032-06-01"},
		{"240 months", "2026-06-30", 240, "2046-06-30"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AddMonthsClamped(reminderDate(tt.from), tt.months).Format("2006-01-02")
			if got != tt.want {
				t.Errorf("AddMonthsClamped(%s, %d) = %s, want %s", tt.from, tt.months, got, tt.want)
			}
		})
	}
}

func TestAircraftReminderStatus(t *testing.T) {
	today := reminderDate("2026-09-26")
	tests := []struct {
		name     string
		due      string
		wantDays int
		want     string
	}{
		{"overdue by one day", "2026-09-25", -1, AircraftReminderStatusOverdue},
		{"due today is due soon", "2026-09-26", 0, AircraftReminderStatusDueSoon},
		{"due in 30 days is due soon", "2026-10-26", 30, AircraftReminderStatusDueSoon},
		{"due in 31 days is ok", "2026-10-27", 31, AircraftReminderStatusOK},
		{"far future is ok", "2027-09-26", 365, AircraftReminderStatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &AircraftReminder{DueDate: reminderDate(tt.due)}
			if got := r.DaysUntilDue(today); got != tt.wantDays {
				t.Errorf("DaysUntilDue = %d, want %d", got, tt.wantDays)
			}
			if got := r.Status(today); got != tt.want {
				t.Errorf("Status = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestAircraftReminderStatus_IgnoresTimeOfDay(t *testing.T) {
	r := &AircraftReminder{DueDate: reminderDate("2026-09-27")}
	lateToday := time.Date(2026, 9, 26, 23, 59, 0, 0, time.UTC)
	if got := r.DaysUntilDue(lateToday); got != 1 {
		t.Errorf("DaysUntilDue = %d, want 1", got)
	}
}

func TestAircraftReminderValidate(t *testing.T) {
	due := reminderDate("2026-10-01")
	intPtr := func(i int) *int { return &i }
	tests := []struct {
		name    string
		r       AircraftReminder
		wantErr error
	}{
		{"annual inspection without label", AircraftReminder{Kind: ReminderKindAnnualInspection, DueDate: due, IntervalMonths: intPtr(12)}, nil},
		{"insurance with label", AircraftReminder{Kind: ReminderKindInsurance, Label: strPtr("Allianz"), DueDate: due}, nil},
		{"custom with label", AircraftReminder{Kind: ReminderKindCustom, Label: strPtr("Prop overhaul"), DueDate: due}, nil},
		{"custom without label", AircraftReminder{Kind: ReminderKindCustom, DueDate: due}, ErrInvalidAircraftReminder},
		{"custom with blank label", AircraftReminder{Kind: ReminderKindCustom, Label: strPtr("   "), DueDate: due}, ErrInvalidAircraftReminder},
		{"unknown kind", AircraftReminder{Kind: "OIL_CHANGE", DueDate: due}, ErrInvalidAircraftReminder},
		{"interval zero", AircraftReminder{Kind: ReminderKindARC, DueDate: due, IntervalMonths: intPtr(0)}, ErrInvalidAircraftReminder},
		{"interval 241", AircraftReminder{Kind: ReminderKindARC, DueDate: due, IntervalMonths: intPtr(241)}, ErrInvalidAircraftReminder},
		{"interval 240", AircraftReminder{Kind: ReminderKindARC, DueDate: due, IntervalMonths: intPtr(240)}, nil},
		{"label 101 chars", AircraftReminder{Kind: ReminderKindCustom, Label: strPtr(strings.Repeat("x", 101)), DueDate: due}, ErrFieldTooLong},
		{"label 100 chars", AircraftReminder{Kind: ReminderKindCustom, Label: strPtr(strings.Repeat("x", 100)), DueDate: due}, nil},
		{"notes 1001 chars", AircraftReminder{Kind: ReminderKindELTBattery, Notes: strPtr(strings.Repeat("n", 1001)), DueDate: due}, ErrFieldTooLong},
		{"missing due date", AircraftReminder{Kind: ReminderKindELTBattery}, ErrInvalidAircraftReminder},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.r
			err := r.Validate()
			if tt.wantErr == nil && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestAircraftReminderValidate_TrimsLabel(t *testing.T) {
	r := AircraftReminder{Kind: ReminderKindInsurance, Label: strPtr("  Hull  "), DueDate: reminderDate("2026-10-01")}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if r.Label == nil || *r.Label != "Hull" {
		t.Errorf("Label = %v, want Hull", r.Label)
	}
	blank := AircraftReminder{Kind: ReminderKindInsurance, Label: strPtr(" "), DueDate: reminderDate("2026-10-01")}
	if err := blank.Validate(); err != nil {
		t.Fatal(err)
	}
	if blank.Label != nil {
		t.Errorf("blank label should normalise to nil, got %q", *blank.Label)
	}
}
