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

var revisionSources = map[string]bool{
	"user_edit": true, "recalc": true, "aircraft_rename": true, "import": true, "unknown": true,
}

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
	// FlightIDs maps a flight's id in the backup to its id after the restore.
	FlightIDs map[uuid.UUID]uuid.UUID
	// LicenseIDs maps a licence's id in the backup to its id after the restore.
	LicenseIDs map[uuid.UUID]uuid.UUID
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

// Validate reports a *LogbookImportError when the backup carries a value the
// logbook cannot store.
func (in LogbookImport) Validate() error {
	for i, e := range in.Events {
		switch e.Trigger {
		case models.LogbookTriggerLicence, models.LogbookTriggerManual, models.LogbookTriggerImport:
		default:
			return invalidLogbookEntry("logbookMode.events", i, "trigger %q", e.Trigger)
		}
	}
	for i, rev := range in.Revisions {
		if rev.Kind != models.RevisionKindUpdate && rev.Kind != models.RevisionKindDelete {
			return invalidLogbookEntry("flightRevisions", i, "kind %q", rev.Kind)
		}
		if rev.RevisionNo < 1 {
			return invalidLogbookEntry("flightRevisions", i, "revision number %d", rev.RevisionNo)
		}
		if _, err := rewriteSnapshotIDs(rev.Snapshot, uuid.Nil, uuid.Nil); err != nil {
			return invalidLogbookEntry("flightRevisions", i, "snapshot: %v", err)
		}
		if _, err := stripCrewContactIDs(rev.CrewSnapshot); err != nil {
			return invalidLogbookEntry("flightRevisions", i, "crew snapshot: %v", err)
		}
	}
	return nil
}

// ImportLogbook restores logbook mode, revisions and certifications for the
// user through w. Revisions of flights absent from the restore keep one fresh
// id per original flight. A strict backup switches strict mode on with an
// import event when the account is not strict.
func ImportLogbook(ctx context.Context, w repository.LogbookRestoreWriter, userID uuid.UUID, in LogbookImport) (LogbookImportResult, error) {
	var result LogbookImportResult
	if err := in.Validate(); err != nil {
		return result, err
	}

	for _, e := range in.Events {
		e.UserID = userID
		if e.LicenseID != nil {
			if mapped, ok := in.LicenseIDs[*e.LicenseID]; ok {
				e.LicenseID = &mapped
			} else {
				e.LicenseID = nil
			}
		}
		if _, err := w.ImportModeEvent(ctx, &e); err != nil {
			return result, fmt.Errorf("restore logbook mode event: %w", err)
		}
	}
	if in.Strict {
		switched, err := w.SetStrict(ctx, userID, true, models.LogbookTriggerImport, nil)
		if err != nil {
			return result, fmt.Errorf("restore logbook mode: %w", err)
		}
		if switched && in.Since != nil {
			if err := w.RestoreStrictSince(ctx, userID, *in.Since); err != nil {
				return result, fmt.Errorf("restore logbook mode: %w", err)
			}
		}
	}

	flightIDs := make(map[uuid.UUID]uuid.UUID, len(in.FlightIDs))
	for old, restored := range in.FlightIDs {
		flightIDs[old] = restored
	}
	revisions := make([]models.FlightRevision, 0, len(in.Revisions))
	for i, rev := range in.Revisions {
		if !revisionSources[rev.Source] {
			rev.Source = "unknown"
		}
		newID, ok := flightIDs[rev.FlightID]
		if !ok {
			newID = uuid.New()
			flightIDs[rev.FlightID] = newID
		}
		snapshot, err := rewriteSnapshotIDs(rev.Snapshot, newID, userID)
		if err != nil {
			return result, invalidLogbookEntry("flightRevisions", i, "snapshot: %v", err)
		}
		crew, err := stripCrewContactIDs(rev.CrewSnapshot)
		if err != nil {
			return result, invalidLogbookEntry("flightRevisions", i, "crew snapshot: %v", err)
		}
		revisions = append(revisions, models.FlightRevision{
			UserID:       userID,
			FlightID:     newID,
			RevisionNo:   rev.RevisionNo,
			Kind:         rev.Kind,
			Snapshot:     snapshot,
			CrewSnapshot: crew,
			Source:       rev.Source,
			Reason:       rev.Reason,
			IsCorrection: rev.IsCorrection,
			ChangedAt:    rev.ChangedAt,
		})
	}
	stored, err := w.ImportRevisions(ctx, revisions)
	if err != nil {
		return result, fmt.Errorf("restore flight revisions: %w", err)
	}
	result.Revisions = stored

	for _, c := range in.Certifications {
		c.UserID = userID
		if c.CertifiedAt.IsZero() {
			c.CertifiedAt = time.Now()
		}
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

// rewriteSnapshotIDs points a stored flight snapshot at the restored flight
// and user.
func rewriteSnapshotIDs(raw json.RawMessage, flightID, userID uuid.UUID) (json.RawMessage, error) {
	obj := map[string]json.RawMessage{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, err
		}
	}
	id, _ := json.Marshal(flightID)
	uid, _ := json.Marshal(userID)
	obj["id"] = id
	obj["user_id"] = uid
	return json.Marshal(obj)
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
