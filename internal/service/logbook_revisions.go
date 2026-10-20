package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// FieldChange is one altered field of a revision.
type FieldChange struct {
	Field    string
	OldValue any
	NewValue any
}

// CrewEntry is a crew member as shown in a crewMembers change.
type CrewEntry = models.CrewKey

// RevisionView is a stored revision with the fields it changed.
type RevisionView struct {
	models.FlightRevision
	Changes []FieldChange
}

// VoidedFlight is a deleted flight kept as a void record.
type VoidedFlight struct {
	FlightID      uuid.UUID
	RevisionID    uuid.UUID
	DeletedAt     time.Time
	Reason        *string
	Source        string
	IsCorrection  bool
	Date          time.Time
	AircraftReg   string
	AircraftType  string
	DepartureICAO *string
	ArrivalICAO   *string
	TotalTime     int
	Remarks       *string
}

// diffExcluded lists flight columns a revision diff never reports.
var diffExcluded = map[string]bool{
	"id": true, "user_id": true, "created_at": true, "updated_at": true, "signature_id": true,
}

// CamelField converts a flights column name to its Flight schema name.
func CamelField(column string) string {
	parts := strings.Split(column, "_")
	var b strings.Builder
	for i, p := range parts {
		if i == 0 {
			b.WriteString(p)
			continue
		}
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]) + p[1:])
	}
	return b.String()
}

func decodeObject(raw json.RawMessage) (map[string]any, error) {
	out := map[string]any{}
	if len(raw) == 0 {
		return out, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, errors.New("not a JSON object")
	}
	return out, nil
}

func decodeCrew(raw json.RawMessage) ([]CrewEntry, error) {
	var entries []CrewEntry
	if len(raw) == 0 {
		return entries, nil
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	models.SortCrewKeys(entries)
	return entries, nil
}

// DiffFlightStates lists the fields that differ between two stored flight
// states, with the crew compared by name and role.
func DiffFlightStates(oldSnap json.RawMessage, oldCrew json.RawMessage, newSnap json.RawMessage, newCrew json.RawMessage) ([]FieldChange, error) {
	oldObj, err := decodeObject(oldSnap)
	if err != nil {
		return nil, err
	}
	newObj, err := decodeObject(newSnap)
	if err != nil {
		return nil, err
	}

	keys := make(map[string]struct{}, len(oldObj)+len(newObj))
	for k := range oldObj {
		keys[k] = struct{}{}
	}
	for k := range newObj {
		keys[k] = struct{}{}
	}
	names := make([]string, 0, len(keys))
	for k := range keys {
		if !diffExcluded[k] {
			names = append(names, k)
		}
	}
	sort.Strings(names)

	changes := []FieldChange{}
	for _, k := range names {
		if reflect.DeepEqual(oldObj[k], newObj[k]) {
			continue
		}
		changes = append(changes, FieldChange{Field: CamelField(k), OldValue: oldObj[k], NewValue: newObj[k]})
	}

	oc, err := decodeCrew(oldCrew)
	if err != nil {
		return nil, err
	}
	nc, err := decodeCrew(newCrew)
	if err != nil {
		return nil, err
	}
	if !models.SameCrewKeys(oc, nc) {
		changes = append(changes, FieldChange{Field: "crewMembers", OldValue: orEmptyCrew(oc), NewValue: orEmptyCrew(nc)})
	}
	return changes, nil
}

func orEmptyCrew(entries []CrewEntry) []CrewEntry {
	if entries == nil {
		return []CrewEntry{}
	}
	return entries
}

// revisionViews turns one flight's revisions (oldest first) into views, each
// diffed against the next state: the following revision's snapshot, or current
// when the flight still exists. A revision that cannot be diffed keeps an
// empty change list.
func revisionViews(revs []models.FlightRevision, current *models.FlightState) []RevisionView {
	views := make([]RevisionView, len(revs))
	for i := range revs {
		views[i] = RevisionView{FlightRevision: revs[i], Changes: []FieldChange{}}
		if revs[i].Kind == models.RevisionKindDelete {
			continue
		}
		var next models.FlightState
		switch {
		case i+1 < len(revs):
			next = models.FlightState{Snapshot: revs[i+1].Snapshot, Crew: revs[i+1].CrewSnapshot}
		case current != nil:
			next = *current
		default:
			continue
		}
		changes, err := DiffFlightStates(revs[i].Snapshot, revs[i].CrewSnapshot, next.Snapshot, next.Crew)
		if err != nil {
			slog.Warn("flight revision cannot be diffed", "revisionId", revs[i].ID, "flightId", revs[i].FlightID, "error", err)
			continue
		}
		views[i].Changes = changes
	}
	return views
}

// ListRevisions returns the flight's revisions newest first, each with the
// fields it changed. The flight may be deleted as long as a void record exists.
func (s *LogbookService) ListRevisions(ctx context.Context, userID, flightID uuid.UUID) ([]RevisionView, error) {
	revs, err := s.repo.ListRevisions(ctx, userID, flightID)
	if err != nil {
		return nil, err
	}

	curSnap, curCrew, err := s.repo.CurrentFlightState(ctx, userID, flightID)
	exists := err == nil
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	if !exists && len(revs) == 0 {
		return nil, ErrFlightNotFound
	}

	var current *models.FlightState
	if exists {
		current = &models.FlightState{Snapshot: curSnap, Crew: curCrew}
	}
	views := revisionViews(revs, current)
	for i, j := 0, len(views)-1; i < j; i, j = i+1, j-1 {
		views[i], views[j] = views[j], views[i]
	}
	return views, nil
}

// snapshotSummary is the part of a stored flight snapshot the voided list and
// the exports read.
type snapshotSummary struct {
	ID            string  `json:"id"`
	Date          string  `json:"date"`
	AircraftReg   string  `json:"aircraft_reg"`
	AircraftType  string  `json:"aircraft_type"`
	DepartureICAO *string `json:"departure_icao"`
	ArrivalICAO   *string `json:"arrival_icao"`
	TotalTime     int     `json:"total_time"`
	Remarks       *string `json:"remarks"`

	date time.Time
}

// parseSnapshotSummary reads the summary fields of a flight snapshot. The
// snapshot must be a JSON object with a valid id and a YYYY-MM-DD date.
func parseSnapshotSummary(raw json.RawMessage) (snapshotSummary, error) {
	var out snapshotSummary
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return out, errors.New("snapshot is not a JSON object")
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, err
	}
	if _, err := uuid.Parse(out.ID); err != nil {
		return out, fmt.Errorf("snapshot id: %w", err)
	}
	date, err := time.Parse("2006-01-02", out.Date)
	if err != nil {
		return out, fmt.Errorf("snapshot date: %w", err)
	}
	out.date = date
	return out, nil
}

// ListVoided returns the user's deleted flights, newest first. A void record
// whose snapshot cannot be read is logged and left out.
func (s *LogbookService) ListVoided(ctx context.Context, userID uuid.UUID) ([]VoidedFlight, error) {
	revs, err := s.repo.ListVoided(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]VoidedFlight, 0, len(revs))
	for _, rev := range revs {
		snap, err := parseSnapshotSummary(rev.Snapshot)
		if err != nil {
			slog.Warn("void record has an unreadable snapshot", "revisionId", rev.ID, "flightId", rev.FlightID, "error", err)
			continue
		}
		out = append(out, VoidedFlight{
			FlightID:      rev.FlightID,
			RevisionID:    rev.ID,
			DeletedAt:     rev.ChangedAt,
			Reason:        rev.Reason,
			Source:        rev.Source,
			IsCorrection:  rev.IsCorrection,
			Date:          snap.date,
			AircraftReg:   snap.AircraftReg,
			AircraftType:  snap.AircraftType,
			DepartureICAO: snap.DepartureICAO,
			ArrivalICAO:   snap.ArrivalICAO,
			TotalTime:     snap.TotalTime,
			Remarks:       snap.Remarks,
		})
	}
	return out, nil
}
