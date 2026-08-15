package portability

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/pkg/csvsafe"
)

// The open portability archive.
//
// Every vendor format above is a lossy projection: each destination models a
// subset of a logbook and drops the rest. Licenses, class ratings, medicals,
// contacts, instructor signatures and the pre-NinerLog opening balance have no
// column in any of them.
//
// This archive is the answer to that. It is a plain ZIP of UTF-8 CSV and JSON,
// documented in docs/PORTABILITY.md and versioned, holding everything the
// account contains. No tool reads it today; any tool can be made to, and it
// stays readable with nothing but a text editor long after every importer
// targeted here has changed. That is the actual portability guarantee — the
// vendor formats are the convenience.

// ArchiveFormatVersion is the version stamped into manifest.json. Bump it when
// the archive layout changes in a way a reader would notice; the manifest
// records it so a reader can tell what it is holding.
const ArchiveFormatVersion = "1.0"

// ArchiveFormatID identifies this archive layout.
const ArchiveFormatID = "ninerlog-portability-archive"

// Manifest is archive/manifest.json — enough for a reader to interpret the
// archive without guessing.
type Manifest struct {
	Format        string            `json:"format"`
	FormatVersion string            `json:"formatVersion"`
	ExportedAt    string            `json:"exportedAt"`
	Pilot         ManifestPilot     `json:"pilot"`
	Files         []ManifestFile    `json:"files"`
	Counts        map[string]int    `json:"counts"`
	Units         map[string]string `json:"units"`
	Documentation string            `json:"documentation"`
}

// ManifestPilot identifies whose logbook this is.
type ManifestPilot struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

// ManifestFile describes one member of the archive.
type ManifestFile struct {
	Path        string `json:"path"`
	Description string `json:"description"`
	Rows        int    `json:"rows,omitempty"`
}

// WriteArchive streams the complete portability archive as a ZIP.
func WriteArchive(out io.Writer, b *Bundle) error {
	zw := zip.NewWriter(out)

	var files []ManifestFile
	add := func(path, description string, rows int, write func(w io.Writer) error) error {
		f, err := zw.Create(path)
		if err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
		if err := write(f); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		files = append(files, ManifestFile{Path: path, Description: description, Rows: rows})
		return nil
	}

	if err := add("flights.csv",
		"Every logged flight, one row each. Durations are decimal hours; times are UTC HH:MM.",
		len(b.Flights), func(w io.Writer) error { return writeArchiveFlights(w, b) }); err != nil {
		return err
	}

	aircraft := resolveAircraft(b)
	if err := add("aircraft.csv",
		"The pilot's fleet, plus any aircraft that appears on a flight but was never added to the fleet.",
		len(aircraft), func(w io.Writer) error { return writeArchiveAircraft(w, aircraft) }); err != nil {
		return err
	}

	if err := add("licenses.csv",
		"Pilot licenses. One row per license; class ratings are in class-ratings.csv.",
		len(b.Licenses), func(w io.Writer) error { return writeArchiveLicenses(w, b) }); err != nil {
		return err
	}

	ratingRows := 0
	for _, l := range b.Licenses {
		ratingRows += len(l.ClassRatings)
	}
	if err := add("class-ratings.csv",
		"Class and type ratings, each linked to its license by licenseId.",
		ratingRows, func(w io.Writer) error { return writeArchiveClassRatings(w, b) }); err != nil {
		return err
	}

	if err := add("credentials.csv",
		"Medicals, language proficiency and radio certificates, with issue and expiry dates.",
		len(b.Credentials), func(w io.Writer) error { return writeArchiveCredentials(w, b) }); err != nil {
		return err
	}

	if err := add("contacts.csv",
		"People the pilot has flown with or recorded.",
		len(b.Contacts), func(w io.Writer) error { return writeArchiveContacts(w, b) }); err != nil {
		return err
	}

	if err := add("crew.csv",
		"Who was on board each flight and in what role, linked to flights.csv by flightId.",
		crewRowCount(b), func(w io.Writer) error { return writeArchiveCrew(w, b) }); err != nil {
		return err
	}

	if err := add("signatures.csv",
		"Instructor sign-off records. Signature images are in signatures/ and referenced by imageFile.",
		len(b.Signatures), func(w io.Writer) error { return writeArchiveSignatures(w, b) }); err != nil {
		return err
	}

	for _, rec := range b.Signatures {
		if rec.ImageFilename == "" || len(rec.Signature.SignatureImage) == 0 {
			continue
		}
		image := rec.Signature.SignatureImage
		if err := add(rec.ImageFilename, "Captured instructor signature image (PNG).", 0,
			func(w io.Writer) error { _, werr := w.Write(image); return werr }); err != nil {
			return err
		}
	}

	if b.Baseline != nil {
		if err := add("baseline.json",
			"Pre-NinerLog opening balance: hours flown before this logbook began. Not a flight, and not carried by any vendor format.",
			1, func(w io.Writer) error { return writeArchiveBaseline(w, b.Baseline) }); err != nil {
			return err
		}
	}

	if err := add("README.md",
		"Human-readable description of this archive and how to read it.",
		0, func(w io.Writer) error { return writeArchiveReadme(w, b) }); err != nil {
		return err
	}

	// The manifest is written last because it describes the files above it.
	manifest := Manifest{
		Format:        ArchiveFormatID,
		FormatVersion: ArchiveFormatVersion,
		ExportedAt:    b.ExportedAt.UTC().Format(time.RFC3339),
		Pilot:         ManifestPilot{Name: b.PilotName, Email: b.PilotEmail},
		Files:         files,
		Counts: map[string]int{
			"flights":      len(b.Flights),
			"aircraft":     len(aircraft),
			"licenses":     len(b.Licenses),
			"classRatings": ratingRows,
			"credentials":  len(b.Credentials),
			"contacts":     len(b.Contacts),
			"crew":         crewRowCount(b),
			"signatures":   len(b.Signatures),
		},
		Units: map[string]string{
			"durations": "decimal hours, two decimal places",
			"dates":     "ISO 8601 (YYYY-MM-DD)",
			"times":     "UTC HH:MM",
			"distance":  "nautical miles",
		},
		Documentation: "https://github.com/fjaeckel/ninerlog-api/blob/main/docs/PORTABILITY.md",
	}
	mf, err := zw.Create("manifest.json")
	if err != nil {
		return fmt.Errorf("create manifest.json: %w", err)
	}
	enc := json.NewEncoder(mf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(manifest); err != nil {
		return fmt.Errorf("encode manifest.json: %w", err)
	}

	return zw.Close()
}

// ArchiveFilename is the download name for one archive.
func ArchiveFilename(exportedAt time.Time) string {
	return fmt.Sprintf("ninerlog-logbook-%s.zip", exportedAt.Format("2006-01-02"))
}

// --- individual archive members ---

var archiveFlightColumns = []string{
	"flightId", "date",
	"aircraftRegistration", "aircraftType",
	"departureIcao", "arrivalIcao", "route",
	"offBlockTime", "takeoffTime", "landingTime", "onBlockTime",
	"totalTime", "picTime", "sicTime", "dualReceivedTime", "dualGivenTime",
	"multiPilotTime", "nightTime", "ifrTime", "actualInstrumentTime",
	"simulatedInstrumentTime", "crossCountryTime", "soloTime",
	"simulatedFlightTime", "groundTrainingTime",
	"distanceNm",
	"takeoffsDay", "takeoffsNight", "landingsDay", "landingsNight", "allLandings",
	"approaches", "approachDetail", "holds",
	"isPic", "isDual", "isFlightReview", "isIpc", "isProficiencyCheck",
	"picName", "instructorName", "instructorComments",
	"fstdType", "launchMethod", "endorsements", "remarks",
	"isSigned", "createdAt", "updatedAt",
}

func writeArchiveFlights(out io.Writer, b *Bundle) error {
	w := csvsafe.NewWriter(out)
	if err := w.Write(archiveFlightColumns); err != nil {
		return err
	}
	for _, f := range b.Flights {
		if err := w.Write([]string{
			f.ID.String(),
			f.Date.Format("2006-01-02"),
			f.AircraftReg,
			f.AircraftType,
			str(f.DepartureICAO),
			str(f.ArrivalICAO),
			str(f.Route),
			clock(f.OffBlockTime),
			clock(f.DepartureTime),
			clock(f.ArrivalTime),
			clock(f.OnBlockTime),
			hoursZero(f.TotalTime),
			hoursZero(f.PICTime),
			hoursZero(f.SICTime),
			hoursZero(f.DualTime),
			hoursZero(f.DualGivenTime),
			hoursZero(f.MultiPilotTime),
			hoursZero(f.NightTime),
			hoursZero(f.IFRTime),
			hoursZero(f.ActualInstrumentTime),
			hoursZero(f.SimulatedInstrumentTime),
			hoursZero(f.CrossCountryTime),
			hoursZero(f.SoloTime),
			hoursZero(f.SimulatedFlightTime),
			hoursZero(f.GroundTrainingTime),
			distanceCell(f.Distance),
			countZero(f.TakeoffsDay),
			countZero(f.TakeoffsNight),
			countZero(f.LandingsDay),
			countZero(f.LandingsNight),
			countZero(f.AllLandings),
			countZero(f.ApproachesCount),
			archiveApproachDetail(f),
			countZero(f.Holds),
			boolWord(f.IsPIC),
			boolWord(f.IsDual),
			boolWord(f.IsFlightReview),
			boolWord(f.IsIPC),
			boolWord(f.IsProficiencyCheck),
			str(f.PICName),
			str(f.InstructorName),
			str(f.InstructorComments),
			str(f.FSTDType),
			str(f.LaunchMethod),
			str(f.Endorsements),
			str(f.Remarks),
			boolWord(f.SignatureID != nil),
			f.CreatedAt.UTC().Format(time.RFC3339),
			f.UpdatedAt.UTC().Format(time.RFC3339),
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// archiveApproachDetail renders the structured approach list as
// "TYPE@AIRPORT/RUNWAY" entries joined by "|", so the per-approach detail
// survives in a single cell without needing a second table.
func archiveApproachDetail(f *models.Flight) string {
	if len(f.Approaches) == 0 {
		return ""
	}
	parts := make([]string, 0, len(f.Approaches))
	for _, a := range f.Approaches {
		part := a.Type
		if ap := str(a.Airport); ap != "" {
			part += "@" + ap
		}
		if rwy := str(a.Runway); rwy != "" {
			part += "/" + rwy
		}
		parts = append(parts, part)
	}
	return joinPipe(parts)
}

func writeArchiveAircraft(out io.Writer, aircraft []exportedAircraft) error {
	w := csvsafe.NewWriter(out)
	if err := w.Write([]string{
		"registration", "typeCode", "make", "model", "class",
		"category", "faaClass", "isComplex", "isHighPerformance", "isTailwheel",
		"inFleet",
	}); err != nil {
		return err
	}
	for _, a := range aircraft {
		category, faaClass := AircraftCategoryClass(a.Class)
		if err := w.Write([]string{
			a.Registration, a.TypeCode, a.Make, a.Model, a.Class,
			category, faaClass,
			boolWord(a.Complex), boolWord(a.HighPerf), boolWord(a.Tailwheel),
			boolWord(a.InFleet),
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func writeArchiveLicenses(out io.Writer, b *Bundle) error {
	w := csvsafe.NewWriter(out)
	if err := w.Write([]string{
		"licenseId", "regulatoryAuthority", "licenseType", "licenseNumber",
		"issueDate", "issuingAuthority", "requiresSeparateLogbook",
	}); err != nil {
		return err
	}
	for _, l := range b.Licenses {
		lic := l.License
		if err := w.Write([]string{
			lic.ID.String(),
			lic.RegulatoryAuthority,
			lic.LicenseType,
			lic.LicenseNumber,
			lic.IssueDate.Format("2006-01-02"),
			lic.IssuingAuthority,
			boolWord(lic.RequiresSeparateLogbook),
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func writeArchiveClassRatings(out io.Writer, b *Bundle) error {
	w := csvsafe.NewWriter(out)
	if err := w.Write([]string{
		"classRatingId", "licenseId", "classType", "issueDate", "expiryDate", "notes",
	}); err != nil {
		return err
	}
	for _, l := range b.Licenses {
		for _, r := range l.ClassRatings {
			if err := w.Write([]string{
				r.ID.String(),
				r.LicenseID.String(),
				string(r.ClassType),
				r.IssueDate.Format("2006-01-02"),
				dateCell(r.ExpiryDate),
				str(r.Notes),
			}); err != nil {
				return err
			}
		}
	}
	w.Flush()
	return w.Error()
}

func writeArchiveCredentials(out io.Writer, b *Bundle) error {
	w := csvsafe.NewWriter(out)
	if err := w.Write([]string{
		"credentialId", "credentialType", "credentialNumber",
		"issueDate", "expiryDate", "issuingAuthority", "notes",
	}); err != nil {
		return err
	}
	for _, c := range b.Credentials {
		if err := w.Write([]string{
			c.ID.String(),
			string(c.CredentialType),
			str(c.CredentialNumber),
			c.IssueDate.Format("2006-01-02"),
			dateCell(c.ExpiryDate),
			c.IssuingAuthority,
			str(c.Notes),
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func writeArchiveContacts(out io.Writer, b *Bundle) error {
	w := csvsafe.NewWriter(out)
	if err := w.Write([]string{"contactId", "name", "email", "phone", "notes"}); err != nil {
		return err
	}
	for _, c := range b.Contacts {
		if err := w.Write([]string{
			c.ID.String(), c.Name, str(c.Email), str(c.Phone), str(c.Notes),
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func writeArchiveCrew(out io.Writer, b *Bundle) error {
	w := csvsafe.NewWriter(out)
	if err := w.Write([]string{"flightId", "date", "name", "role", "contactId"}); err != nil {
		return err
	}
	for _, f := range b.Flights {
		for _, m := range f.CrewMembers {
			contactID := ""
			if m.ContactID != nil {
				contactID = m.ContactID.String()
			}
			if err := w.Write([]string{
				f.ID.String(),
				f.Date.Format("2006-01-02"),
				m.Name,
				string(m.Role),
				contactID,
			}); err != nil {
				return err
			}
		}
	}
	w.Flush()
	return w.Error()
}

func writeArchiveSignatures(out io.Writer, b *Bundle) error {
	w := csvsafe.NewWriter(out)
	if err := w.Write([]string{
		"signatureId", "flightId", "flightDate", "aircraftRegistration",
		"method", "status", "instructorName", "instructorCredentialNumber",
		"signedAt", "imageFile",
	}); err != nil {
		return err
	}
	for _, rec := range b.Signatures {
		s := rec.Signature
		flightDate, reg := "", ""
		if rec.Flight != nil {
			flightDate = rec.Flight.Date.Format("2006-01-02")
			reg = rec.Flight.AircraftReg
		}
		if err := w.Write([]string{
			s.ID.String(),
			s.FlightID.String(),
			flightDate,
			reg,
			string(s.Method),
			string(s.Status),
			str(s.InstructorName),
			str(s.InstructorCredentialNo),
			timeCell(s.SignedAt),
			rec.ImageFilename,
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func writeArchiveBaseline(out io.Writer, bl *models.FlightBaseline) error {
	payload := map[string]any{
		"description": "Flying experience recorded before this logbook began. " +
			"These hours are not flights and appear in no flights.csv row; add them " +
			"to any total computed from flights.csv.",
		"baselineDate":      bl.BaselineDate.Format("2006-01-02"),
		"totalFlights":      bl.TotalFlights,
		"totalHours":        hoursZero(bl.TotalMinutes),
		"picHours":          hoursZero(bl.PICMinutes),
		"sicHours":          hoursZero(bl.SICMinutes),
		"dualReceivedHours": hoursZero(bl.DualMinutes),
		"dualGivenHours":    hoursZero(bl.DualGivenMinutes),
		"multiPilotHours":   hoursZero(bl.MultiPilotMinutes),
		"nightHours":        hoursZero(bl.NightMinutes),
		"ifrHours":          hoursZero(bl.IFRMinutes),
		"soloHours":         hoursZero(bl.SoloMinutes),
		"crossCountryHours": hoursZero(bl.CrossCountryMinutes),
		"landingsDay":       bl.LandingsDay,
		"landingsNight":     bl.LandingsNight,
		"notes":             str(bl.Notes),
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func writeArchiveReadme(out io.Writer, b *Bundle) error {
	var buf bytes.Buffer
	buf.WriteString("# Your NinerLog logbook\n\n")
	buf.WriteString("This archive contains every record NinerLog held for this account, in plain\n")
	buf.WriteString("UTF-8 CSV and JSON. It is yours. Nothing here needs NinerLog to read it —\n")
	buf.WriteString("a text editor or any spreadsheet application will open every file.\n\n")

	fmt.Fprintf(&buf, "Exported: %s\n", b.ExportedAt.UTC().Format(time.RFC3339))
	if b.PilotName != "" {
		fmt.Fprintf(&buf, "Pilot: %s\n", b.PilotName)
	}
	fmt.Fprintf(&buf, "Archive format: %s v%s\n\n", ArchiveFormatID, ArchiveFormatVersion)

	buf.WriteString("## Files\n\n")
	buf.WriteString("| File | What it holds |\n|---|---|\n")
	buf.WriteString("| `flights.csv` | Every logged flight, one row each. |\n")
	buf.WriteString("| `aircraft.csv` | The fleet, plus aircraft reconstructed from flights. |\n")
	buf.WriteString("| `licenses.csv` | Pilot licenses. |\n")
	buf.WriteString("| `class-ratings.csv` | Ratings, linked to licenses by `licenseId`. |\n")
	buf.WriteString("| `credentials.csv` | Medicals, language proficiency, radio certificates. |\n")
	buf.WriteString("| `contacts.csv` | People recorded in the logbook. |\n")
	buf.WriteString("| `crew.csv` | Who was on board each flight, linked by `flightId`. |\n")
	buf.WriteString("| `signatures.csv` | Instructor sign-offs; images in `signatures/`. |\n")
	buf.WriteString("| `baseline.json` | Hours flown before this logbook began, if recorded. |\n")
	buf.WriteString("| `manifest.json` | Machine-readable index, row counts and units. |\n\n")

	buf.WriteString("## Conventions\n\n")
	buf.WriteString("- Durations are decimal hours with two decimal places (`1.50` = one hour thirty).\n")
	buf.WriteString("- Dates are ISO 8601 (`YYYY-MM-DD`). Times of day are UTC `HH:MM`.\n")
	buf.WriteString("- Distances are nautical miles.\n")
	buf.WriteString("- Booleans are `true` / `false`.\n")
	buf.WriteString("- IDs are stable within this archive and link the files to each other.\n")
	buf.WriteString("- A cell beginning with `'` had a leading `=`, `+`, `-` or `@`; the apostrophe\n")
	buf.WriteString("  stops spreadsheets treating logbook text as a formula and is not part of the value.\n\n")

	buf.WriteString("## Moving to another logbook\n\n")
	buf.WriteString("NinerLog can also export directly into the import format of several other\n")
	buf.WriteString("products — ForeFlight Logbook, LogTen Pro, MyFlightbook and CrewLounge\n")
	buf.WriteString("PILOTLOG. Those are one-file uploads that need no column mapping, but each\n")
	buf.WriteString("one only carries what that product models. This archive carries everything.\n\n")
	buf.WriteString("If a total in another product does not match, check `baseline.json`: hours\n")
	buf.WriteString("flown before this logbook began are recorded there and in no flight row.\n")

	_, err := out.Write(buf.Bytes())
	return err
}

// --- small shared helpers ---

func boolWord(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func dateCell(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

func timeCell(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func joinPipe(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "|"
		}
		out += p
	}
	return out
}

func crewRowCount(b *Bundle) int {
	n := 0
	for _, f := range b.Flights {
		n += len(f.CrewMembers)
	}
	return n
}
