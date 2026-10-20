package repository

import "context"

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
