package models

import (
	"testing"
	"time"
)

func TestExpiry_LastValidDay(t *testing.T) {
	expiry := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	berlin := time.FixedZone("CEST", 2*3600)
	newYork := time.FixedZone("EDT", -4*3600)

	tests := []struct {
		name         string
		now          time.Time
		wantExpired  bool
		wantDaysLeft int
		wantSoon7    bool
	}{
		{"day before, late evening UTC", time.Date(2026, 10, 18, 23, 59, 0, 0, time.UTC), false, 1, true},
		{"expiry date, first second", time.Date(2026, 10, 19, 0, 0, 1, 0, time.UTC), false, 0, true},
		{"expiry date, last minute", time.Date(2026, 10, 19, 23, 59, 0, 0, time.UTC), false, 0, true},
		{"day after, midnight", time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), true, -1, false},
		{"non-UTC clock still on expiry date in UTC", time.Date(2026, 10, 20, 1, 30, 0, 0, berlin), false, 0, true},
		{"non-UTC clock already past expiry in UTC", time.Date(2026, 10, 19, 21, 0, 0, 0, newYork), true, -1, false},
		{"eight days ahead", time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC), false, 8, false},
		{"seven days ahead", time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC), false, 7, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &ClassRating{ExpiryDate: &expiry}
			c := &Credential{ExpiryDate: &expiry}
			if got := cr.IsExpiredAt(tt.now); got != tt.wantExpired {
				t.Errorf("ClassRating.IsExpiredAt = %v, want %v", got, tt.wantExpired)
			}
			if got := c.IsExpiredAt(tt.now); got != tt.wantExpired {
				t.Errorf("Credential.IsExpiredAt = %v, want %v", got, tt.wantExpired)
			}
			if got := DaysUntilExpiry(expiry, tt.now); got != tt.wantDaysLeft {
				t.Errorf("DaysUntilExpiry = %d, want %d", got, tt.wantDaysLeft)
			}
			if got := cr.IsExpiringSoonAt(tt.now, 7); got != tt.wantSoon7 {
				t.Errorf("ClassRating.IsExpiringSoonAt(7) = %v, want %v", got, tt.wantSoon7)
			}
			if got := c.IsExpiringSoonAt(tt.now, 7); got != tt.wantSoon7 {
				t.Errorf("Credential.IsExpiringSoonAt(7) = %v, want %v", got, tt.wantSoon7)
			}
		})
	}
}

func TestExpiry_NilNeverExpires(t *testing.T) {
	now := time.Now()
	if (&ClassRating{}).IsExpiredAt(now) || (&Credential{}).IsExpiredAt(now) {
		t.Error("nil expiry reported expired")
	}
	if (&ClassRating{}).IsExpiringSoonAt(now, 30) || (&Credential{}).IsExpiringSoonAt(now, 30) {
		t.Error("nil expiry reported expiring soon")
	}
}

func TestExpiry_TodayIsValid(t *testing.T) {
	today := CalendarDate(time.Now())
	if (&ClassRating{ExpiryDate: &today}).IsExpired() {
		t.Error("rating expiring today reported expired")
	}
	if (&Credential{ExpiryDate: &today}).IsExpired() {
		t.Error("credential expiring today reported expired")
	}
	yesterday := today.AddDate(0, 0, -1)
	if !(&ClassRating{ExpiryDate: &yesterday}).IsExpired() {
		t.Error("rating that expired yesterday reported valid")
	}
}
