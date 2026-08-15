package portability

import (
	"fmt"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/fjaeckel/ninerlog-api/pkg/duration"
)

// This file holds the value conversions the vendor writers share. Keeping them
// here means a rounding or unit decision is made once: if decimal hours are
// wrong in one export they are wrong in all of them, which is far easier to
// spot and fix than four subtly different implementations.

// NinerLog stores every duration as integer minutes (see the aviation-domain
// skill). Every product targeted here reads decimal hours, so the conversion
// happens on the way out and never on the way in.

// hours renders minutes as decimal hours with two places — 90 -> "1.50".
// Zero renders as an empty cell: an importer that sees "0.00" may create a
// zero-time entry where the pilot logged nothing at all.
func hours(minutes int) string {
	if minutes == 0 {
		return ""
	}
	return fmt.Sprintf("%.2f", float64(minutes)/60.0)
}

// hoursZero renders minutes as decimal hours, emitting "0.00" rather than an
// empty cell. Used for columns a product treats as required.
func hoursZero(minutes int) string {
	return fmt.Sprintf("%.2f", float64(minutes)/60.0)
}

// hhmm renders minutes as H:MM, the shape EASA-style logbooks print.
func hhmm(minutes int) string {
	if minutes == 0 {
		return ""
	}
	return duration.FormatColonHM(minutes)
}

// count renders a whole number, blank when zero, so importers do not record
// explicit zeros for landings or approaches that were simply not logged.
func count(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%d", n)
}

// countZero renders a whole number including zero.
func countZero(n int) string { return fmt.Sprintf("%d", n) }

// distanceCell renders great-circle distance in nautical miles, blank when the
// flight has none (an unknown airport, or a local flight that never left the
// circuit).
func distanceCell(nm float64) string {
	if nm <= 0 {
		return ""
	}
	return fmt.Sprintf("%.1f", nm)
}

// str dereferences an optional string.
func str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// clock trims a stored HH:MM:SS to HH:MM. NinerLog stores seconds; no target
// product records them.
func clock(s *string) string {
	if s == nil {
		return ""
	}
	v := *s
	if len(v) >= 5 {
		return v[:5]
	}
	return v
}

// clockCompact renders a stored HH:MM:SS as HHMM, which ForeFlight's template
// expects for its time-out/off/on/in columns.
func clockCompact(s *string) string {
	v := clock(s)
	return strings.ReplaceAll(v, ":", "")
}

// yesNo renders a boolean the way MyFlightbook requires — literally "Yes" or
// "No" in English, regardless of the pilot's locale.
func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// boolCell renders a boolean as the lowercase "true"/"false" ForeFlight uses,
// blank when false so unset flags do not clutter the sheet.
func boolCell(b bool) string {
	if b {
		return "true"
	}
	return ""
}

// AircraftCategoryClass maps a NinerLog aircraft class to the (category, class)
// pair the FAA-derived products use.
//
// NinerLog stores EASA class names (SEP_LAND, MEP_SEA, TMG, …) and allows
// custom values. Anything unrecognised falls back to airplane single-engine
// land, which is what the overwhelming majority of unclassified light aircraft
// are — and is a value every importer accepts, so an unknown class degrades to
// an importable row rather than a rejected file.
func AircraftCategoryClass(aircraftClass string) (category, class string) {
	switch strings.ToUpper(strings.TrimSpace(aircraftClass)) {
	case "SEP_SEA", "SES":
		return "Airplane", "Single-Engine Sea"
	case "MEP_LAND", "MEL":
		return "Airplane", "Multi-Engine Land"
	case "MEP_SEA", "MES":
		return "Airplane", "Multi-Engine Sea"
	case "TMG", "GLIDER", "SAILPLANE":
		return "Glider", "Glider"
	case "HELICOPTER", "SE_HELICOPTER", "ME_HELICOPTER":
		return "Rotorcraft", "Helicopter"
	case "GYROPLANE":
		return "Rotorcraft", "Gyroplane"
	case "BALLOON":
		return "Lighter Than Air", "Balloon"
	case "AIRSHIP":
		return "Lighter Than Air", "Airship"
	default:
		return "Airplane", "Single-Engine Land"
	}
}

// fleetIndex builds a registration -> aircraft lookup, upper-cased so a flight
// logged with a lower-case registration still finds its fleet entry.
func fleetIndex(aircraft []*models.Aircraft) map[string]*models.Aircraft {
	idx := make(map[string]*models.Aircraft, len(aircraft))
	for _, a := range aircraft {
		idx[strings.ToUpper(strings.TrimSpace(a.Registration))] = a
	}
	return idx
}

// aircraftClassFor returns the fleet class for a flight's registration, or ""
// when the aircraft is not in the fleet.
func aircraftClassFor(idx map[string]*models.Aircraft, reg string) string {
	if a, ok := idx[strings.ToUpper(strings.TrimSpace(reg))]; ok && a.AircraftClass != nil {
		return *a.AircraftClass
	}
	return ""
}

// exportedAircraft is one row of an aircraft table, resolved from the fleet
// where possible and synthesised from the flight where not.
type exportedAircraft struct {
	Registration string
	TypeCode     string
	Make         string
	Model        string
	Class        string
	Complex      bool
	HighPerf     bool
	Tailwheel    bool
	// InFleet is false for aircraft reconstructed from flight rows because the
	// pilot never added them to their fleet.
	InFleet bool
	// IsSimulator marks a training device rather than an aircraft. Declaring an
	// FNPT or FFS as an aeroplane would let the destination count simulator
	// hours as flight time, which is both wrong and the kind of error that
	// surfaces at a licence renewal rather than at import.
	IsSimulator bool
}

// simulatorRegistrations returns the registrations that only ever appear on
// FSTD rows and are not in the pilot's fleet.
//
// A pilot logs simulator sessions under a pseudo-registration ("SIM-FNPT2").
// Requiring every flight on that identifier to be an FSTD row keeps a real
// aircraft from being misclassified because of one mis-tagged entry.
func simulatorRegistrations(b *Bundle) map[string]bool {
	fleet := fleetIndex(b.Aircraft)
	candidates := map[string]bool{}
	for _, f := range b.Flights {
		key := strings.ToUpper(strings.TrimSpace(f.AircraftReg))
		if key == "" {
			continue
		}
		if _, inFleet := fleet[key]; inFleet {
			candidates[key] = false
			continue
		}
		isFSTD := f.FSTDType != nil && strings.TrimSpace(*f.FSTDType) != ""
		if prev, seen := candidates[key]; seen {
			candidates[key] = prev && isFSTD
		} else {
			candidates[key] = isFSTD
		}
	}
	return candidates
}

// resolveAircraft returns the aircraft table to export: every fleet entry,
// plus a synthesised entry for any registration that appears on a flight but
// not in the fleet.
//
// ForeFlight rejects a flight whose AircraftID has no row in the aircraft
// table, so exporting only the fleet would silently drop the pilot's oldest
// flights — exactly the flights they most need to keep. Synthesising the
// missing rows is what makes the export complete.
func resolveAircraft(b *Bundle) []exportedAircraft {
	seen := make(map[string]bool, len(b.Aircraft))
	out := make([]exportedAircraft, 0, len(b.Aircraft))

	for _, a := range b.Aircraft {
		key := strings.ToUpper(strings.TrimSpace(a.Registration))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, exportedAircraft{
			Registration: a.Registration,
			TypeCode:     a.Type,
			Make:         a.Make,
			Model:        a.Model,
			Class:        str(a.AircraftClass),
			Complex:      a.IsComplex,
			HighPerf:     a.IsHighPerformance,
			Tailwheel:    a.IsTailwheel,
			InFleet:      true,
		})
	}

	simulators := simulatorRegistrations(b)
	for _, f := range b.Flights {
		key := strings.ToUpper(strings.TrimSpace(f.AircraftReg))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, exportedAircraft{
			Registration: f.AircraftReg,
			TypeCode:     f.AircraftType,
			Model:        f.AircraftType,
			InFleet:      false,
			IsSimulator:  simulators[key],
		})
	}

	return out
}

// resolvedPICName returns the PIC to name in an export.
//
// flightrules.DisplayPICName renders "SELF" when the account holder was PIC —
// the EASA logbook convention, and what the EASA PDF and CSV print. That
// convention does not travel: a destination that treats the column as a person
// creates a crew member literally called SELF. Products that expect a name get
// the pilot's actual name; the one product whose own format uses SELF calls
// DisplayPICName directly.
func resolvedPICName(f *models.Flight, pilotName string) string {
	name := flightrules.DisplayPICName(f, pilotName)
	if strings.EqualFold(strings.TrimSpace(name), "self") && pilotName != "" {
		return pilotName
	}
	return name
}

// routeOrEndpoints returns the flight's route string, falling back to
// "DEP ARR" when no route was recorded.
//
// Products that carry no separate departure/arrival columns store the airports
// in the route field alone. Without this fallback every flight logged without
// an explicit route arrives at the destination with no airports at all.
func routeOrEndpoints(f *models.Flight) string {
	if r := strings.TrimSpace(str(f.Route)); r != "" {
		return r
	}
	dep := strings.TrimSpace(str(f.DepartureICAO))
	arr := strings.TrimSpace(str(f.ArrivalICAO))
	switch {
	case dep != "" && arr != "":
		return dep + " " + arr
	case dep != "":
		return dep
	default:
		return arr
	}
}
