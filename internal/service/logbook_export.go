package service

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// ExportRevision is one recorded update of a flight with the fields it changed.
type ExportRevision struct {
	FlightID      uuid.UUID
	RevisionNo    int
	ChangedAt     time.Time
	Date          time.Time
	AircraftReg   string
	DepartureICAO *string
	ArrivalICAO   *string
	Source        string
	Reason        *string
	IsCorrection  bool
	Changes       []FieldChange
}

// ExportHistory is the audit trail a strict-mode export prints.
type ExportHistory struct {
	Since           *time.Time
	Events          []models.LogbookModeEvent
	Revisions       []ExportRevision
	Voided          []VoidedFlight
	LastCorrectedAt map[uuid.UUID]time.Time
}

// ExportHistory returns the user's audit trail for an export, or nil when the
// user is not in strict mode. Revisions are ordered by flight date, flight and
// revision number.
func (s *LogbookService) ExportHistory(ctx context.Context, userID uuid.UUID) (*ExportHistory, error) {
	mode, err := s.repo.GetMode(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !mode.Strict {
		return nil, nil
	}
	events, err := s.repo.ListModeEvents(ctx, userID)
	if err != nil {
		return nil, err
	}
	all, err := s.repo.ListUserRevisions(ctx, userID)
	if err != nil {
		return nil, err
	}

	history := &ExportHistory{
		Since:           mode.Since,
		Events:          events,
		Revisions:       []ExportRevision{},
		LastCorrectedAt: lastCorrections(all),
	}

	flightIDs := []uuid.UUID{}
	seen := map[uuid.UUID]bool{}
	for _, rev := range all {
		if rev.Kind != models.RevisionKindUpdate || seen[rev.FlightID] {
			continue
		}
		seen[rev.FlightID] = true
		flightIDs = append(flightIDs, rev.FlightID)
	}

	for _, flightID := range flightIDs {
		views, err := s.ListRevisions(ctx, userID, flightID)
		if err != nil {
			return nil, err
		}
		for _, view := range views {
			if view.Kind != models.RevisionKindUpdate {
				continue
			}
			entry, err := exportRevisionOf(view)
			if err != nil {
				return nil, err
			}
			history.Revisions = append(history.Revisions, entry)
		}
	}

	sort.SliceStable(history.Revisions, func(i, j int) bool {
		a, b := history.Revisions[i], history.Revisions[j]
		if !a.Date.Equal(b.Date) {
			return a.Date.Before(b.Date)
		}
		if a.FlightID != b.FlightID {
			return a.FlightID.String() < b.FlightID.String()
		}
		return a.RevisionNo < b.RevisionNo
	})

	history.Voided, err = s.ListVoided(ctx, userID)
	if err != nil {
		return nil, err
	}
	return history, nil
}

func exportRevisionOf(view RevisionView) (ExportRevision, error) {
	var snap struct {
		Date          string  `json:"date"`
		AircraftReg   string  `json:"aircraft_reg"`
		DepartureICAO *string `json:"departure_icao"`
		ArrivalICAO   *string `json:"arrival_icao"`
	}
	if err := json.Unmarshal(view.Snapshot, &snap); err != nil {
		return ExportRevision{}, err
	}
	date, err := time.Parse("2006-01-02", snap.Date)
	if err != nil {
		return ExportRevision{}, err
	}
	return ExportRevision{
		FlightID:      view.FlightID,
		RevisionNo:    view.RevisionNo,
		ChangedAt:     view.ChangedAt,
		Date:          date,
		AircraftReg:   snap.AircraftReg,
		DepartureICAO: snap.DepartureICAO,
		ArrivalICAO:   snap.ArrivalICAO,
		Source:        view.Source,
		Reason:        view.Reason,
		IsCorrection:  view.IsCorrection,
		Changes:       view.Changes,
	}, nil
}

// LastCorrectedAt returns, per flight, the time of its latest correction
// revision. The bool is false when the user is not in strict mode.
func (s *LogbookService) LastCorrectedAt(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]time.Time, bool, error) {
	mode, err := s.repo.GetMode(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	if !mode.Strict {
		return nil, false, nil
	}
	all, err := s.repo.ListUserRevisions(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	return lastCorrections(all), true, nil
}

func lastCorrections(revs []models.FlightRevision) map[uuid.UUID]time.Time {
	out := map[uuid.UUID]time.Time{}
	for _, rev := range revs {
		if rev.Kind == models.RevisionKindUpdate && rev.IsCorrection && rev.ChangedAt.After(out[rev.FlightID]) {
			out[rev.FlightID] = rev.ChangedAt
		}
	}
	return out
}
