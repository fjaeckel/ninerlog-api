package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// ErrInvalidLogbookImport is returned when a backup's logbook section carries
// a value the logbook cannot store.
var ErrInvalidLogbookImport = errors.New("invalid logbook data in backup")

var revisionSources = func() map[string]bool {
	m := make(map[string]bool, len(repository.ChangeSources))
	for _, s := range repository.ChangeSources {
		m[s] = true
	}
	return m
}()

// LogbookState is everything strict logbook mode stores for a user.
type LogbookState struct {
	Mode           models.LogbookMode
	Events         []models.LogbookModeEvent
	Revisions      []models.FlightRevision
	Certifications []models.LogbookCertification
}

// ExportState returns the user's logbook mode, switch events, revisions and
// certifications.
func (s *LogbookService) ExportState(ctx context.Context, userID uuid.UUID) (*LogbookState, error) {
	mode, err := s.repo.GetMode(ctx, userID)
	if err != nil {
		return nil, err
	}
	events, err := s.repo.ListModeEvents(ctx, userID)
	if err != nil {
		return nil, err
	}
	revisions, err := s.repo.ListUserRevisions(ctx, userID)
	if err != nil {
		return nil, err
	}
	certs, err := s.repo.ListCertifications(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &LogbookState{Mode: *mode, Events: events, Revisions: revisions, Certifications: certs}, nil
}

// LogbookImport is the logbook state read from a backup.
type LogbookImport struct {
	Strict         bool
	Since          *time.Time
	Events         []models.LogbookModeEvent
	Revisions      []models.FlightRevision
	Certifications []models.LogbookCertification
}

// PreparedLogbook is a LogbookImport that passed validation, with every
// revision's snapshot and crew parsed.
type PreparedLogbook struct {
	strict         bool
	since          *time.Time
	events         []models.LogbookModeEvent
	certifications []models.LogbookCertification
	revisions      []preparedRevision
}

type preparedRevision struct {
	rev      models.FlightRevision
	snapshot map[string]json.RawMessage
	crew     json.RawMessage
}

// LogbookImportResult counts what a restore stored.
type LogbookImportResult struct {
	Revisions      int
	Certifications int
}

// LogbookImportError names the backup entry a restore cannot store. It
// matches ErrInvalidLogbookImport.
type LogbookImportError struct {
	Section string
	Index   int
	Detail  string
}

func (e *LogbookImportError) Error() string {
	return fmt.Sprintf("%s: %s %d: %s", ErrInvalidLogbookImport, e.Section, e.Index, e.Detail)
}

// Unwrap returns ErrInvalidLogbookImport.
func (e *LogbookImportError) Unwrap() error { return ErrInvalidLogbookImport }

func invalidLogbookEntry(section string, index int, format string, args ...any) error {
	return &LogbookImportError{Section: section, Index: index, Detail: fmt.Sprintf(format, args...)}
}

// Prepare validates the backup's logbook and parses its snapshots. It reports
// a *LogbookImportError when an entry cannot be stored: an unknown trigger,
// kind or revision number, an unreadable snapshot or crew, or a missing
// timestamp.
func (in LogbookImport) Prepare() (*PreparedLogbook, error) {
	p := &PreparedLogbook{
		strict:         in.Strict,
		since:          in.Since,
		events:         in.Events,
		certifications: in.Certifications,
		revisions:      make([]preparedRevision, 0, len(in.Revisions)),
	}
	for i, e := range in.Events {
		switch e.Trigger {
		case models.LogbookTriggerLicence, models.LogbookTriggerManual, models.LogbookTriggerImport:
		default:
			return nil, invalidLogbookEntry("logbookMode.events", i, "trigger %q", e.Trigger)
		}
		if e.CreatedAt.IsZero() {
			return nil, invalidLogbookEntry("logbookMode.events", i, "createdAt is missing")
		}
	}
	for i, c := range in.Certifications {
		if c.CertifiedAt.IsZero() {
			return nil, invalidLogbookEntry("logbookCertifications", i, "certifiedAt is missing")
		}
	}
	for i, rev := range in.Revisions {
		if rev.Kind != models.RevisionKindUpdate && rev.Kind != models.RevisionKindDelete {
			return nil, invalidLogbookEntry("flightRevisions", i, "kind %q", rev.Kind)
		}
		if rev.RevisionNo < 1 {
			return nil, invalidLogbookEntry("flightRevisions", i, "revision number %d", rev.RevisionNo)
		}
		if rev.ChangedAt.IsZero() {
			return nil, invalidLogbookEntry("flightRevisions", i, "changedAt is missing")
		}
		if _, err := parseSnapshotSummary(rev.Snapshot); err != nil {
			return nil, invalidLogbookEntry("flightRevisions", i, "snapshot: %v", err)
		}
		if _, err := DiffFlightStates(rev.Snapshot, rev.CrewSnapshot, rev.Snapshot, rev.CrewSnapshot); err != nil {
			return nil, invalidLogbookEntry("flightRevisions", i, "snapshot: %v", err)
		}
		snapshot, err := rawObject(rev.Snapshot)
		if err != nil {
			return nil, invalidLogbookEntry("flightRevisions", i, "snapshot: %v", err)
		}
		crew, err := stripCrewContactIDs(rev.CrewSnapshot)
		if err != nil {
			return nil, invalidLogbookEntry("flightRevisions", i, "crew snapshot: %v", err)
		}
		if !revisionSources[rev.Source] {
			rev.Source = repository.ChangeSourceUnknown
		}
		rev.Snapshot, rev.CrewSnapshot = nil, nil
		p.revisions = append(p.revisions, preparedRevision{rev: rev, snapshot: snapshot, crew: crew})
	}
	return p, nil
}

// RevisionFlightIDs returns the distinct flight ids, as numbered in the
// backup, that the revisions belong to.
func (p *PreparedLogbook) RevisionFlightIDs() []uuid.UUID {
	seen := make(map[uuid.UUID]bool, len(p.revisions))
	var ids []uuid.UUID
	for _, r := range p.revisions {
		if !seen[r.rev.FlightID] {
			seen[r.rev.FlightID] = true
			ids = append(ids, r.rev.FlightID)
		}
	}
	return ids
}

// ImportLogbook restores logbook mode, revisions and certifications for the
// user through w. flightIDs maps a flight's id in the backup to its id after
// the restore and licenseIDs does the same for licences; a revision of a
// flight missing from flightIDs gets one fresh id per original flight. A
// strict backup switches strict mode on with an import event when the account
// is not strict.
func ImportLogbook(ctx context.Context, w repository.LogbookRestoreWriter, userID uuid.UUID, p *PreparedLogbook, flightIDs, licenseIDs map[uuid.UUID]uuid.UUID) (LogbookImportResult, error) {
	var result LogbookImportResult

	for _, e := range p.events {
		e.UserID = userID
		if e.LicenseID != nil {
			if mapped, ok := licenseIDs[*e.LicenseID]; ok {
				e.LicenseID = &mapped
			} else {
				e.LicenseID = nil
			}
		}
		if _, err := w.ImportModeEvent(ctx, &e); err != nil {
			return result, fmt.Errorf("restore logbook mode event: %w", err)
		}
	}
	if p.strict {
		switched, err := w.SetStrict(ctx, userID, true, models.LogbookTriggerImport, nil)
		if err != nil {
			return result, fmt.Errorf("restore logbook mode: %w", err)
		}
		if switched && p.since != nil {
			if err := w.RestoreStrictSince(ctx, userID, *p.since); err != nil {
				return result, fmt.Errorf("restore logbook mode: %w", err)
			}
		}
	}

	ids := make(map[uuid.UUID]uuid.UUID, len(flightIDs))
	for old, restored := range flightIDs {
		ids[old] = restored
	}
	revisions := make([]models.FlightRevision, 0, len(p.revisions))
	for _, pr := range p.revisions {
		rev := pr.rev
		newID, ok := ids[rev.FlightID]
		if !ok {
			newID = uuid.New()
			ids[rev.FlightID] = newID
		}
		snapshot, err := withSnapshotIDs(pr.snapshot, newID, userID)
		if err != nil {
			return result, fmt.Errorf("restore flight revision: %w", err)
		}
		rev.UserID = userID
		rev.FlightID = newID
		rev.Snapshot = snapshot
		rev.CrewSnapshot = pr.crew
		revisions = append(revisions, rev)
	}
	stored, err := w.ImportRevisions(ctx, revisions)
	if err != nil {
		return result, fmt.Errorf("restore flight revisions: %w", err)
	}
	result.Revisions = stored

	for _, c := range p.certifications {
		c.UserID = userID
		inserted, err := w.ImportCertification(ctx, &c)
		if err != nil {
			return result, fmt.Errorf("restore logbook certification: %w", err)
		}
		if inserted {
			result.Certifications++
		}
	}
	return result, nil
}

// rawObject decodes a JSON object into its raw members.
func rawObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, errors.New("snapshot is not a JSON object")
	}
	return obj, nil
}

// withSnapshotIDs returns the snapshot object pointed at the restored flight
// and user, leaving obj unchanged.
func withSnapshotIDs(obj map[string]json.RawMessage, flightID, userID uuid.UUID) (json.RawMessage, error) {
	out := make(map[string]json.RawMessage, len(obj))
	for k, v := range obj {
		out[k] = v
	}
	id, _ := json.Marshal(flightID)
	uid, _ := json.Marshal(userID)
	out["id"] = id
	out["user_id"] = uid
	return json.Marshal(out)
}

// stripCrewContactIDs drops contact links from a crew snapshot; contact ids do
// not survive a restore.
func stripCrewContactIDs(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage("[]"), nil
	}
	var crew []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &crew); err != nil {
		return nil, err
	}
	for _, m := range crew {
		delete(m, "contactId")
	}
	if crew == nil {
		crew = []map[string]json.RawMessage{}
	}
	return json.Marshal(crew)
}
