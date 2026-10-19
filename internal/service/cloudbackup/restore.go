package cloudbackup

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

// RestoreMode selects how a restore treats data already in the account.
type RestoreMode string

const (
	// RestoreModeRestore requires an empty account.
	RestoreModeRestore RestoreMode = "restore"
	// RestoreModeReplace wipes the account's collections before restoring.
	RestoreModeReplace RestoreMode = "replace"
)

// Valid reports whether m is a known mode.
func (m RestoreMode) Valid() bool {
	return m == RestoreModeRestore || m == RestoreModeReplace
}

// Limits on a single restore.
const (
	MaxRestoreFlights  = 100000
	MaxRestoreContacts = 100000
	MaxRestoreEntities = 2000

	maxRestoreWarnings = 1000
)

var (
	// ErrAccountNotEmpty is returned by a restore-mode restore into an
	// account that already holds data; nothing is written.
	ErrAccountNotEmpty = errors.New("account already holds data")

	// ErrReplaceWouldDropAttachments is returned by a replace-mode restore
	// while the account holds document files or signed flights; nothing is
	// written.
	ErrReplaceWouldDropAttachments = errors.New("replace would delete document files or signed flights")

	// ErrUnsupportedBackupVersion is returned for a backup written by a newer
	// major payload version than this build reads.
	ErrUnsupportedBackupVersion = errors.New("backup version is newer than supported")
)

// InvalidBackupError reports a backup that cannot be restored. Message is
// safe to show a client; Detail is for logs only.
type InvalidBackupError struct {
	Message string
	Detail  string
}

func (e *InvalidBackupError) Error() string {
	if e.Detail != "" {
		return e.Message + ": " + e.Detail
	}
	return e.Message
}

func invalidEntry(section string, index int, detail error) *InvalidBackupError {
	e := &InvalidBackupError{Message: fmt.Sprintf("Invalid entry %d in backup section %q", index, section)}
	if detail != nil {
		e.Detail = detail.Error()
	}
	return e
}

func tooMany(section string, n, limit int) *InvalidBackupError {
	return &InvalidBackupError{Message: fmt.Sprintf("Backup contains too many %s (%d, max %d)", section, n, limit)}
}

// RestoreWarning flags a row that was restored although it no longer passes
// validation.
type RestoreWarning struct {
	Section string `json:"section"`
	Index   int    `json:"index"`
	Message string `json:"message"`
}

// RestoreResult summarises a committed restore.
type RestoreResult struct {
	Mode                            RestoreMode      `json:"mode"`
	AircraftImported                int              `json:"aircraftImported"`
	AircraftSkipped                 int              `json:"aircraftSkipped"`
	LicensesImported                int              `json:"licensesImported"`
	ClassRatingsImported            int              `json:"classRatingsImported"`
	CredentialsImported             int              `json:"credentialsImported"`
	FlightsImported                 int              `json:"flightsImported"`
	CrewMembersImported             int              `json:"crewMembersImported"`
	ContactsImported                int              `json:"contactsImported"`
	ContactsSkipped                 int              `json:"contactsSkipped"`
	ContactsCreated                 int              `json:"contactsCreated"`
	CustomCurrencyRulesImported     int              `json:"customCurrencyRulesImported"`
	CustomReportsImported           int              `json:"customReportsImported"`
	NotificationPreferencesImported bool             `json:"notificationPreferencesImported"`
	FlightBaselineImported          bool             `json:"flightBaselineImported"`
	Warnings                        []RestoreWarning `json:"warnings"`
}

// Restorer restores a Payload into a user's account in one transaction.
type Restorer struct {
	store repository.RestoreStore
}

// NewRestorer creates a Restorer over the given store.
func NewRestorer(store repository.RestoreStore) *Restorer {
	return &Restorer{store: store}
}

// Restore validates the payload, then writes every section in a single
// transaction. An empty mode means RestoreModeRestore.
//
// Errors: *InvalidBackupError for a payload that cannot be restored,
// ErrUnsupportedBackupVersion, ErrAccountNotEmpty and
// ErrReplaceWouldDropAttachments for refused restores; anything else is an
// infrastructure failure and nothing was written.
func (r *Restorer) Restore(ctx context.Context, userID uuid.UUID, p *Payload, mode RestoreMode) (*RestoreResult, error) {
	if mode == "" {
		mode = RestoreModeRestore
	}
	if !mode.Valid() {
		return nil, &InvalidBackupError{Message: "Unknown restore mode"}
	}
	if err := checkEnvelope(p); err != nil {
		return nil, err
	}
	plan, err := buildRestorePlan(userID, p)
	if err != nil {
		return nil, err
	}

	var result *RestoreResult
	err = r.store.InRestoreTx(ctx, userID, func(tx repository.RestoreTx) error {
		var err error
		result, err = plan.apply(ctx, tx, mode)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// checkEnvelope validates format, version and section sizes.
func checkEnvelope(p *Payload) error {
	if p.Format != PayloadFormat {
		return &InvalidBackupError{Message: "Unsupported backup format (expected '" + PayloadFormat + "')"}
	}
	if err := checkVersion(p.Version); err != nil {
		return err
	}
	for _, s := range []struct {
		name     string
		n, limit int
	}{
		{"flights", len(p.Flights), MaxRestoreFlights},
		{"contacts", len(p.Contacts), MaxRestoreContacts},
		{"aircraft", len(p.Aircraft), MaxRestoreEntities},
		{"licenses", len(p.Licenses), MaxRestoreEntities},
		{"credentials", len(p.Credentials), MaxRestoreEntities},
		{"custom currency rules", len(p.CustomCurrencyRules), MaxRestoreEntities},
		{"custom reports", len(p.CustomReports), MaxRestoreEntities},
	} {
		if s.n > s.limit {
			return tooMany(s.name, s.n, s.limit)
		}
	}
	return nil
}

// checkVersion accepts an absent version and any major version up to
// SupportedMajorVersion.
func checkVersion(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	major, _, _ := strings.Cut(v, ".")
	n, err := strconv.Atoi(major)
	if err != nil || n < 0 {
		return &InvalidBackupError{Message: "Unreadable backup version", Detail: v}
	}
	if n > SupportedMajorVersion {
		return ErrUnsupportedBackupVersion
	}
	return nil
}

// apply runs the refusal checks, the optional wipe and every insert in tx.
func (pl *restorePlan) apply(ctx context.Context, tx repository.RestoreTx, mode RestoreMode) (*RestoreResult, error) {
	switch mode {
	case RestoreModeRestore:
		content, err := tx.CountContent(ctx, pl.userID)
		if err != nil {
			return nil, fmt.Errorf("count content: %w", err)
		}
		if !content.Empty() {
			return nil, ErrAccountNotEmpty
		}
	case RestoreModeReplace:
		attachments, err := tx.CountAttachments(ctx, pl.userID)
		if err != nil {
			return nil, fmt.Errorf("count attachments: %w", err)
		}
		if attachments.Any() {
			return nil, ErrReplaceWouldDropAttachments
		}
		if err := tx.WipeContent(ctx, pl.userID); err != nil {
			return nil, fmt.Errorf("wipe content: %w", err)
		}
	}

	for i, a := range pl.aircraft {
		if err := tx.InsertAircraft(ctx, a); err != nil {
			return nil, fmt.Errorf("insert aircraft %d: %w", i, err)
		}
	}

	licenseIDs := make(map[uuid.UUID]uuid.UUID, len(pl.licenses))
	ratings := 0
	for i, l := range pl.licenses {
		if err := tx.InsertLicense(ctx, l.license); err != nil {
			return nil, fmt.Errorf("insert license %d: %w", i, err)
		}
		licenseIDs[l.oldID] = l.license.ID
		for j, cr := range l.ratings {
			cr.LicenseID = l.license.ID
			if err := tx.InsertClassRating(ctx, cr); err != nil {
				return nil, fmt.Errorf("insert class rating %d of license %d: %w", j, i, err)
			}
			ratings++
		}
	}

	for i, c := range pl.credentials {
		if err := tx.InsertCredential(ctx, c); err != nil {
			return nil, fmt.Errorf("insert credential %d: %w", i, err)
		}
	}

	if err := tx.InsertContacts(ctx, pl.contacts); err != nil {
		return nil, fmt.Errorf("insert contacts: %w", err)
	}
	if err := tx.InsertFlights(ctx, pl.flights); err != nil {
		return nil, fmt.Errorf("insert flights: %w", err)
	}
	if err := tx.InsertCrewMembers(ctx, pl.crew); err != nil {
		return nil, fmt.Errorf("insert crew members: %w", err)
	}

	for i, rule := range pl.rules {
		if err := tx.InsertCustomCurrencyRule(ctx, rule); err != nil {
			return nil, fmt.Errorf("insert custom currency rule %d: %w", i, err)
		}
	}

	for i, rep := range pl.reports {
		if id := rep.Definition.Filter.LogbookLicenseID; id != nil {
			if newID, ok := licenseIDs[*id]; ok {
				rep.Definition.Filter.LogbookLicenseID = &newID
			} else {
				rep.Definition.Filter.LogbookLicenseID = nil
			}
		}
		if err := tx.InsertCustomReport(ctx, rep); err != nil {
			return nil, fmt.Errorf("insert custom report %d: %w", i, err)
		}
	}

	if pl.prefs != nil {
		if err := tx.UpsertNotificationPreferences(ctx, pl.prefs); err != nil {
			return nil, fmt.Errorf("upsert notification preferences: %w", err)
		}
	}
	if pl.baseline != nil {
		if err := tx.UpsertFlightBaseline(ctx, pl.baseline); err != nil {
			return nil, fmt.Errorf("upsert flight baseline: %w", err)
		}
	}

	return &RestoreResult{
		Mode:                            mode,
		AircraftImported:                len(pl.aircraft),
		AircraftSkipped:                 pl.aircraftSkipped,
		LicensesImported:                len(pl.licenses),
		ClassRatingsImported:            ratings,
		CredentialsImported:             len(pl.credentials),
		FlightsImported:                 len(pl.flights),
		CrewMembersImported:             len(pl.crew),
		ContactsImported:                pl.contactsImported,
		ContactsSkipped:                 pl.contactsSkipped,
		ContactsCreated:                 pl.contactsCreated,
		CustomCurrencyRulesImported:     len(pl.rules),
		CustomReportsImported:           len(pl.reports),
		NotificationPreferencesImported: pl.prefs != nil,
		FlightBaselineImported:          pl.baseline != nil,
		Warnings:                        pl.warnings.list(),
	}, nil
}
