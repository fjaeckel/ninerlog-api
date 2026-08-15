package portability

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"time"
)

// Target names a destination logbook product.
type Target string

const (
	// TargetForeFlight is ForeFlight Logbook (Boeing/ForeFlight, US).
	TargetForeFlight Target = "foreflight"
	// TargetLogTenPro is LogTen Pro (Coradine, US).
	TargetLogTenPro Target = "logten"
	// TargetMyFlightbook is MyFlightbook (free/open, US).
	TargetMyFlightbook Target = "myflightbook"
	// TargetCrewLounge is CrewLounge PILOTLOG, formerly mccPILOTLOG (EU).
	TargetCrewLounge Target = "crewlounge"
)

// ErrUnknownTarget is returned for a target this package cannot write.
var ErrUnknownTarget = errors.New("unknown export target")

// Descriptor describes one target for the API and the UI.
type Descriptor struct {
	Target Target
	// Product is the destination's own name for itself.
	Product string
	// ContentType is the MIME type to serve the file with.
	ContentType string
	// Extension is the file extension, without the dot.
	Extension string
	// Notes explains, in one line, what the pilot should expect — including
	// anything the destination cannot represent. The UI shows this verbatim,
	// so a pilot knows before downloading what will and will not survive.
	Notes string
	// Verified reports whether this layout has been round-tripped through a
	// live import of the destination product. Layouts built from published
	// templates but not yet round-tripped are marked false, and the UI says so
	// rather than implying a guarantee that has not been tested.
	Verified bool

	write func(w io.Writer, b *Bundle) error
}

// Filename is the download name for one export of this target.
func (d Descriptor) Filename(exportedAt time.Time) string {
	return fmt.Sprintf("ninerlog-%s-%s.%s", d.Target, exportedAt.Format("2006-01-02"), d.Extension)
}

// registry holds every supported target. Adding a product means adding one
// entry here and one writer file — nothing else in the codebase changes.
var registry = map[Target]Descriptor{
	TargetForeFlight: {
		Target:      TargetForeFlight,
		Product:     "ForeFlight Logbook",
		ContentType: "text/csv; charset=utf-8",
		Extension:   "csv",
		Notes: "Two-table ForeFlight import template. Carries the aircraft fleet, " +
			"times, landings, approaches, holds, instructor and crew names. " +
			"Aircraft flown but never added to the fleet are reconstructed from " +
			"the flights so no entry is dropped.",
		write: writeForeFlight,
	},
	TargetLogTenPro: {
		Target:      TargetLogTenPro,
		Product:     "LogTen Pro",
		ContentType: "text/csv; charset=utf-8",
		Extension:   "csv",
		Notes: "LogTen Pro import columns, tab-free CSV with its field names in " +
			"the header row. Import it through LogTen's CSV importer and accept " +
			"the auto-detected mapping.",
		write: writeLogTenPro,
	},
	TargetMyFlightbook: {
		Target:      TargetMyFlightbook,
		Product:     "MyFlightbook",
		ContentType: "text/csv; charset=utf-8",
		Extension:   "csv",
		Notes: "MyFlightbook import columns. Times are decimal hours and flags " +
			"are Yes/No, as its importer requires. Multi-pilot and EASA-specific " +
			"times ride along as named properties.",
		write: writeMyFlightbook,
	},
	TargetCrewLounge: {
		Target:      TargetCrewLounge,
		Product:     "CrewLounge PILOTLOG",
		ContentType: "text/csv; charset=utf-8",
		Extension:   "csv",
		Notes: "CrewLounge PILOTLOG / mccPILOTLOG import columns, including the " +
			"EASA multi-pilot, IFR and simulator fields the FAA-oriented formats " +
			"have nowhere to put.",
		write: writeCrewLounge,
	},
}

// Targets returns every supported target, ordered by product name so the UI
// can render the list without sorting it again.
func Targets() []Descriptor {
	out := make([]Descriptor, 0, len(registry))
	for _, d := range registry {
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Product < out[j].Product })
	return out
}

// Lookup returns the descriptor for a target.
func Lookup(t Target) (Descriptor, error) {
	d, ok := registry[t]
	if !ok {
		return Descriptor{}, fmt.Errorf("%w: %q", ErrUnknownTarget, t)
	}
	return d, nil
}

// Write renders the bundle for one target.
func Write(t Target, w io.Writer, b *Bundle) error {
	d, err := Lookup(t)
	if err != nil {
		return err
	}
	return d.write(w, b)
}
