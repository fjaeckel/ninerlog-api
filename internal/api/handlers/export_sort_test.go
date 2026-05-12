package handlers

import (
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

func TestSortFlightsChronological(t *testing.T) {
	day1 := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)

	mk := func(d time.Time, off string) *models.Flight {
		f := &models.Flight{ID: uuid.New(), Date: d}
		if off != "" {
			s := off
			f.OffBlockTime = &s
		}
		return f
	}

	// Build flights deliberately out of order (and with mixed off-block times
	// within the same day) — the exporter must sort first by date, then by
	// off-block time so the earliest flight of the day comes first.
	f1 := mk(day2, "14:30:00") // day2 afternoon
	f2 := mk(day1, "08:15:00") // day1 morning
	f3 := mk(day1, "16:45:00") // day1 late
	f4 := mk(day2, "07:00:00") // day2 early
	f5 := mk(day1, "")         // day1 unknown time — should sort first

	flights := []*models.Flight{f1, f2, f3, f4, f5}
	sortFlightsChronological(flights)

	got := []uuid.UUID{flights[0].ID, flights[1].ID, flights[2].ID, flights[3].ID, flights[4].ID}
	want := []uuid.UUID{f5.ID, f2.ID, f3.ID, f4.ID, f1.ID}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order mismatch at %d: got %v, want %v", i, got, want)
		}
	}
}

func TestSortFlightsChronologicalFallsBackToDepartureTime(t *testing.T) {
	day := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	dep1 := "09:00:00"
	dep2 := "12:00:00"
	a := &models.Flight{ID: uuid.New(), Date: day, DepartureTime: &dep2}
	b := &models.Flight{ID: uuid.New(), Date: day, DepartureTime: &dep1}

	flights := []*models.Flight{a, b}
	sortFlightsChronological(flights)

	if flights[0].ID != b.ID || flights[1].ID != a.ID {
		t.Fatalf("expected b before a, got %v then %v", flights[0].ID, flights[1].ID)
	}
}
