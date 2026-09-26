package models

// WarningCode identifies a save-time check that fired.
type WarningCode string

const (
	WarningULNightFlight    WarningCode = "ul_night_flight"
	WarningULMTOMExceeds600 WarningCode = "ul_mtom_exceeds_600"
	WarningUL120kgClass     WarningCode = "ul_120kg_class"
)

// WarningSeverity grades a Warning.
type WarningSeverity string

const (
	WarningSeverityWarning WarningSeverity = "warning"
	WarningSeverityInfo    WarningSeverity = "info"
)

// Warning is a non-blocking finding about a saved record.
type Warning struct {
	Code     WarningCode
	Severity WarningSeverity
	Params   map[string]any
}
