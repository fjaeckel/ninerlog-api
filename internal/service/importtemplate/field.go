// Package importtemplate holds the catalogue of known pilot-logbook export
// formats and the logic that recognises one from a file's header row.
//
// A template is pure data: a set of header aliases pointing at NinerLog import
// fields, plus the signature columns that identify the source application. The
// package deliberately does not import the generated OpenAPI types — the
// handler converts Field values to generated.ImportField at the edge — so the
// catalogue stays usable from background jobs and tests.
package importtemplate

// Field is a NinerLog import target field. The string values are exactly the
// members of the OpenAPI ImportField enum; the handler casts between the two.
type Field string

const (
	FieldIgnore Field = "ignore"

	FieldDate         Field = "date"
	FieldAircraftReg  Field = "aircraftReg"
	FieldAircraftType Field = "aircraftType"

	FieldDepartureIcao Field = "departureIcao"
	FieldArrivalIcao   Field = "arrivalIcao"
	FieldRoute         Field = "route"

	FieldOffBlockTime  Field = "offBlockTime"
	FieldOnBlockTime   Field = "onBlockTime"
	FieldDepartureTime Field = "departureTime"
	FieldArrivalTime   Field = "arrivalTime"

	FieldTotalTime               Field = "totalTime"
	FieldNightTime               Field = "nightTime"
	FieldIFRTime                 Field = "ifrTime"
	FieldActualInstrumentTime    Field = "actualInstrumentTime"
	FieldSimulatedInstrumentTime Field = "simulatedInstrumentTime"
	FieldDualGivenTime           Field = "dualGivenTime"

	FieldIsPic  Field = "isPic"
	FieldIsDual Field = "isDual"

	FieldLandingsDay   Field = "landingsDay"
	FieldLandingsNight Field = "landingsNight"
	FieldLandingsTotal Field = "landingsTotal"

	FieldApproachesCount Field = "approachesCount"
	FieldHolds           Field = "holds"
	FieldIsIpc           Field = "isIpc"
	FieldIsFlightReview  Field = "isFlightReview"

	FieldRemarks            Field = "remarks"
	FieldInstructorName     Field = "instructorName"
	FieldInstructorComments Field = "instructorComments"

	FieldPerson1 Field = "person1"
	FieldPerson2 Field = "person2"
	FieldPerson3 Field = "person3"
	FieldPerson4 Field = "person4"
	FieldPerson5 Field = "person5"
	FieldPerson6 Field = "person6"
)

// Mapping is one suggested source-column → target-field pair.
type Mapping struct {
	SourceColumn string
	TargetField  Field
	// DateFormat is a Go layout hint attached to date columns. The importer
	// falls back through its own layout list when the hint does not match, so
	// a wrong guess degrades rather than fails.
	DateFormat string
}
