package service

import (
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func TestFleetIndexResolvesReusedRegistrationByDate(t *testing.T) {
	until := time.Date(2019, 12, 31, 0, 0, 0, 0, time.UTC)
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	cessna := &models.Aircraft{Registration: "N123AB", Type: "C172", ValidTo: &until}
	piper := &models.Aircraft{Registration: "N123AB", Type: "PA16", ValidFrom: &from, IsMultiPilot: true}
	idx := NewFleetIndex([]*models.Aircraft{cessna, piper, nil})

	tests := []struct {
		name string
		reg  string
		date time.Time
		want *models.Aircraft
	}{
		{"before the change", "N123AB", time.Date(2015, 6, 1, 0, 0, 0, 0, time.UTC), cessna},
		{"last day of the first range", "n123ab", until, cessna},
		{"first day of the second range", "N123AB ", from, piper},
		{"zero date resolves today", "N123AB", time.Time{}, piper},
		{"unknown registration", "D-EABC", from, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := idx.Resolve(tt.reg, tt.date); got != tt.want {
				t.Errorf("Resolve(%q, %s) = %v, want %v", tt.reg, tt.date.Format(time.DateOnly), got, tt.want)
			}
		})
	}

	if !idx.Has("n123ab") || idx.Has("D-EABC") {
		t.Error("Has() does not reflect the fleet registrations")
	}

	facts := NewAircraftFactsIndex([]*models.Aircraft{cessna, piper})
	if f := facts.For("N123AB", from); f == nil || !f.IsMultiPilot {
		t.Errorf("For(after) = %+v, want the PA16 facts", f)
	}
	if f := facts.For("N123AB", until); f == nil || f.IsMultiPilot {
		t.Errorf("For(before) = %+v, want the C172 facts", f)
	}
}

func TestFleetIndexGapResolvesNothing(t *testing.T) {
	to := time.Date(2010, 12, 31, 0, 0, 0, 0, time.UTC)
	from := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
	idx := NewFleetIndex([]*models.Aircraft{
		{Registration: "N123AB", Type: "C172", ValidTo: &to},
		{Registration: "N123AB", Type: "PA16", ValidFrom: &from},
	})
	if got := idx.Resolve("N123AB", time.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC)); got != nil {
		t.Errorf("Resolve in gap = %+v, want nil", got)
	}
}
