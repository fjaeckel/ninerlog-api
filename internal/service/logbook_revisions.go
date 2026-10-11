package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
type CrewEntry struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

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
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Role != entries[j].Role {
			return entries[i].Role < entries[j].Role
		}
		return entries[i].Name < entries[j].Name
	})
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
	if !reflect.DeepEqual(crewKeys(oc), crewKeys(nc)) {
		changes = append(changes, FieldChange{Field: "crewMembers", OldValue: crewKeys(oc), NewValue: crewKeys(nc)})
	}
	return changes, nil
}

func crewKeys(entries []CrewEntry) []CrewEntry {
	out := make([]CrewEntry, 0, len(entries))
	out = append(out, entries...)
	return out
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

	views := make([]RevisionView, len(revs))
	for i := range revs {
		views[i] = RevisionView{FlightRevision: revs[i], Changes: []FieldChange{}}
		if revs[i].Kind == models.RevisionKindDelete {
			continue
		}
		nextSnap, nextCrew := curSnap, curCrew
		hasNext := exists
		if i+1 < len(revs) {
			nextSnap, nextCrew, hasNext = revs[i+1].Snapshot, revs[i+1].CrewSnapshot, true
		}
		if !hasNext {
			continue
		}
		changes, err := DiffFlightStates(revs[i].Snapshot, revs[i].CrewSnapshot, nextSnap, nextCrew)
		if err != nil {
			return nil, err
		}
		views[i].Changes = changes
	}

	for i, j := 0, len(views)-1; i < j; i, j = i+1, j-1 {
		views[i], views[j] = views[j], views[i]
	}
	return views, nil
}

// ListVoided returns the user's deleted flights, newest first.
func (s *LogbookService) ListVoided(ctx context.Context, userID uuid.UUID) ([]VoidedFlight, error) {
	revs, err := s.repo.ListVoided(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]VoidedFlight, 0, len(revs))
	for _, rev := range revs {
		var snap struct {
			Date          string  `json:"date"`
			AircraftReg   string  `json:"aircraft_reg"`
			AircraftType  string  `json:"aircraft_type"`
			DepartureICAO *string `json:"departure_icao"`
			ArrivalICAO   *string `json:"arrival_icao"`
			TotalTime     int     `json:"total_time"`
			Remarks       *string `json:"remarks"`
		}
		if err := json.Unmarshal(rev.Snapshot, &snap); err != nil {
			return nil, err
		}
		date, err := time.Parse("2006-01-02", snap.Date)
		if err != nil {
			return nil, err
		}
		out = append(out, VoidedFlight{
			FlightID:      rev.FlightID,
			RevisionID:    rev.ID,
			DeletedAt:     rev.ChangedAt,
			Reason:        rev.Reason,
			Source:        rev.Source,
			IsCorrection:  rev.IsCorrection,
			Date:          date,
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
