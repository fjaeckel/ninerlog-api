package handlers

import (
	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/service/importtemplate"
	"github.com/fjaeckel/ninerlog-api/pkg/duration"
)

// importDurationFields are the import targets parsed as durations.
var importDurationFields = map[generated.ImportField]bool{
	generated.ImportFieldTotalTime:               true,
	generated.ImportFieldIsDual:                  true,
	generated.ImportFieldNightTime:               true,
	generated.ImportFieldCrossCountryTime:        true,
	generated.ImportFieldIfrTime:                 true,
	generated.ImportFieldActualInstrumentTime:    true,
	generated.ImportFieldSimulatedInstrumentTime: true,
	generated.ImportField("simulatedFlightTime"): true,
	generated.ImportFieldDualGivenTime:           true,
}

// importDuration parses a duration cell in the mapping's duration format.
func importDuration(val string, m generated.ImportColumnMapping) (int, error) {
	unit := duration.UnitAuto
	if m.DurationFormat != nil {
		unit = duration.Unit(*m.DurationFormat)
	}
	return duration.ParseDurationAs(normalizeDecimalSeparator(val), unit)
}

// explicitDurationFormat reports whether m carries a non-auto duration format.
func explicitDurationFormat(m generated.ImportColumnMapping) bool {
	return m.DurationFormat != nil && *m.DurationFormat != generated.ImportColumnMappingDurationFormatAuto
}

// columnDurationUnit infers the duration unit of column col across rows.
func columnDurationUnit(rows []map[string]string, col string) duration.Unit {
	values := make([]string, len(rows))
	for i, row := range rows {
		values[i] = row[col]
	}
	return duration.InferUnit(values)
}

// resolveDurationFormats sets every auto-format duration column in mappings,
// and every auxiliary duration column the mappings leave unassigned, to
// `decimal` when its cells across rows read as decimal hours.
func resolveDurationFormats(mappings map[string]generated.ImportColumnMapping, aux map[string]importtemplate.AuxField, rows []map[string]string) {
	decimal := generated.ImportColumnMappingDurationFormatDecimal
	for col, m := range mappings {
		if !importDurationFields[m.TargetField] || explicitDurationFormat(m) {
			continue
		}
		if columnDurationUnit(rows, col) == duration.UnitDecimalHours {
			m.DurationFormat = &decimal
			mappings[col] = m
		}
	}
	for col, field := range aux {
		if field != importtemplate.AuxSICTime && field != importtemplate.AuxMultiPilotTime {
			continue
		}
		m, ok := mappings[col]
		if ok && (m.TargetField != generated.ImportFieldIgnore || explicitDurationFormat(m)) {
			continue
		}
		if columnDurationUnit(rows, col) != duration.UnitDecimalHours {
			continue
		}
		if !ok {
			m = generated.ImportColumnMapping{SourceColumn: col, TargetField: generated.ImportFieldIgnore}
		}
		m.DurationFormat = &decimal
		mappings[col] = m
	}
}

// suggestDurationFormats sets `decimal` on each suggested duration mapping
// whose column reads as decimal hours across rows.
func suggestDurationFormats(suggested []generated.ImportColumnMapping, rows []map[string]string) {
	for i, m := range suggested {
		if !importDurationFields[m.TargetField] || m.DurationFormat != nil {
			continue
		}
		if columnDurationUnit(rows, m.SourceColumn) == duration.UnitDecimalHours {
			decimal := generated.ImportColumnMappingDurationFormatDecimal
			suggested[i].DurationFormat = &decimal
		}
	}
}
