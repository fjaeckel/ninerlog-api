package repository

import (
	"context"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// RestoreContent reports which of the collections a restore writes hold at
// least one row for the user. Notification preferences and the flight
// baseline are single-row settings and are not reported.
type RestoreContent struct {
	Flights             bool
	Aircraft            bool
	Licenses            bool
	Credentials         bool
	Contacts            bool
	CustomCurrencyRules bool
	CustomReports       bool
}

// Empty reports whether no collection holds a row.
func (c RestoreContent) Empty() bool {
	return !c.Flights && !c.Aircraft && !c.Licenses && !c.Credentials &&
		!c.Contacts && !c.CustomCurrencyRules && !c.CustomReports
}

// RestoreAttachments reports rows a backup does not carry and a wipe would
// destroy.
type RestoreAttachments struct {
	DocumentFiles bool
	SignedFlights bool
}

// Any reports whether the user holds at least one such row.
func (a RestoreAttachments) Any() bool {
	return a.DocumentFiles || a.SignedFlights
}

// FlightIDUsage reports which flight ids are in use.
type FlightIDUsage struct {
	// Taken holds ids of flights that exist, whoever owns them, and ids that
	// carry revisions of another user.
	Taken map[uuid.UUID]bool
	// Voided holds ids that carry a delete revision of the user.
	Voided map[uuid.UUID]bool
}

// RestoreStore runs a restore inside one database transaction.
type RestoreStore interface {
	// InRestoreTx runs fn in a transaction serialised per user, with the
	// change context carried by ctx applied. A nil return commits; any error
	// rolls everything fn wrote back and is returned.
	InRestoreTx(ctx context.Context, userID uuid.UUID, fn func(tx RestoreTx) error) error
}

// RestoreTx is the set of reads and writes a restore performs. Every method
// runs in the transaction opened by InRestoreTx.
type RestoreTx interface {
	// CountContent reports which collections hold rows for the user.
	CountContent(ctx context.Context, userID uuid.UUID) (RestoreContent, error)

	// CountAttachments reports whether the user holds document files or signed
	// flights.
	CountAttachments(ctx context.Context, userID uuid.UUID) (RestoreAttachments, error)

	// WipeContent deletes every collection a restore writes, together with
	// flight crew and import history. Single-row settings are kept. The
	// flights in correctionFlightIDs are deleted with the correction flag set
	// on their void records; the others carry the flag of the context.
	WipeContent(ctx context.Context, userID uuid.UUID, correctionFlightIDs []uuid.UUID) error

	// ListFlightTimings returns the user's flights with only id, date, block
	// times and departure set, and SignatureID set when the flight has ever
	// carried a completed or voided signature.
	ListFlightTimings(ctx context.Context, userID uuid.UUID) ([]*models.Flight, error)

	// Repos returns repositories bound to this transaction.
	Repos() TxRepos

	// FlightIDUsage reports how each of ids is already in use.
	FlightIDUsage(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (FlightIDUsage, error)

	InsertAircraft(ctx context.Context, aircraft *models.Aircraft) error
	InsertLicense(ctx context.Context, license *models.License) error
	InsertClassRating(ctx context.Context, rating *models.ClassRating) error
	InsertLicencePrivilege(ctx context.Context, privilege *models.LicencePrivilege) error
	InsertCredential(ctx context.Context, credential *models.Credential) error
	InsertCustomCurrencyRule(ctx context.Context, rule *models.CustomCurrencyRule) error
	InsertCustomReport(ctx context.Context, report *models.CustomReport) error

	// InsertContacts, InsertFlights and InsertCrewMembers write their rows
	// with the identifiers already set, in batched statements.
	InsertContacts(ctx context.Context, contacts []*models.Contact) error
	InsertFlights(ctx context.Context, flights []*models.Flight) error
	InsertCrewMembers(ctx context.Context, members []models.FlightCrewMember) error

	UpsertNotificationPreferences(ctx context.Context, prefs *models.NotificationPreferences) error
	UpsertFlightBaseline(ctx context.Context, baseline *models.FlightBaseline) error
	UpsertPilotProfile(ctx context.Context, profile *models.PilotProfile) error

	// LogbookRestoreWriter writes strict logbook mode, its switch events,
	// flight revisions and certifications.
	LogbookRestoreWriter
}
