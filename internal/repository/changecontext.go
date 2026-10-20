package repository

import (
	"context"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// Change sources recorded on a flight revision.
const (
	ChangeSourceUserEdit       = "user_edit"
	ChangeSourceRecalc         = "recalc"
	ChangeSourceAircraftRename = "aircraft_rename"
	ChangeSourceContactRename  = "contact_rename"
	ChangeSourceImport         = "import"
	ChangeSourceUnknown        = "unknown"
)

// ChangeSources lists the sources a stored revision can carry.
var ChangeSources = []string{
	ChangeSourceUserEdit, ChangeSourceRecalc, ChangeSourceAircraftRename,
	ChangeSourceContactRename, ChangeSourceImport, ChangeSourceUnknown,
}

// ChangeContext describes why a write touches flights or their crew.
type ChangeContext struct {
	Source       string
	Reason       *string
	IsCorrection bool
	// RejectChange makes a flight write that alters the flight or its crew
	// fail with ErrChangeRejected and roll back. A write that leaves both
	// unchanged commits.
	RejectChange bool
}

type changeContextKey struct{}

// WithChangeContext returns ctx carrying cc.
func WithChangeContext(ctx context.Context, cc ChangeContext) context.Context {
	return context.WithValue(ctx, changeContextKey{}, cc)
}

// ChangeContextFrom returns the change context carried by ctx.
func ChangeContextFrom(ctx context.Context) (ChangeContext, bool) {
	cc, ok := ctx.Value(changeContextKey{}).(ChangeContext)
	return cc, ok
}

// WithChangeSource returns ctx carrying a change context with only a source.
func WithChangeSource(ctx context.Context, source string) context.Context {
	return WithChangeContext(ctx, ChangeContext{Source: source})
}

// TxRepos exposes repositories bound to the transaction of a flight write or
// a bulk wipe.
type TxRepos interface {
	Contacts() ContactRepository
	Logbook() LogbookRepository
	Licenses() LicenseRepository
}

// TxHook runs inside a flight write transaction, before the flight is written.
type TxHook func(ctx context.Context, repos TxRepos) error

type txHookKey struct{}

// WithTxHook returns ctx carrying a hook the flight repository runs inside
// its write transaction. A hook error rolls the write back.
func WithTxHook(ctx context.Context, hook TxHook) context.Context {
	return context.WithValue(ctx, txHookKey{}, hook)
}

// RunTxHook runs the hook carried by ctx, if any, against repos.
func RunTxHook(ctx context.Context, repos TxRepos) error {
	hook, ok := ctx.Value(txHookKey{}).(TxHook)
	if !ok || hook == nil {
		return nil
	}
	return hook(ctx, repos)
}

// FlightLister loads a user's flights inside the transaction of the caller.
// Each flight carries id, date, block times and departure; SignatureID is set
// when the flight has ever carried a completed or voided signature.
type FlightLister func() ([]*models.Flight, error)

// DeleteCorrections returns the ids of the flights whose deletion is a
// correction. repos are bound to the transaction that performs the delete.
type DeleteCorrections func(ctx context.Context, repos TxRepos, userID uuid.UUID, list FlightLister) ([]uuid.UUID, error)

type deleteCorrectionsKey struct{}

// WithDeleteCorrections returns ctx carrying the classifier a bulk delete of
// flights applies inside its transaction.
func WithDeleteCorrections(ctx context.Context, fn DeleteCorrections) context.Context {
	return context.WithValue(ctx, deleteCorrectionsKey{}, fn)
}

// DeleteCorrectionsFrom returns the classifier carried by ctx.
func DeleteCorrectionsFrom(ctx context.Context) (DeleteCorrections, bool) {
	fn, ok := ctx.Value(deleteCorrectionsKey{}).(DeleteCorrections)
	return fn, ok && fn != nil
}
