package portability

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func buildArchive(t *testing.T, b *Bundle) *zip.Reader {
	t.Helper()
	var buf bytes.Buffer
	if err := WriteArchive(&buf, b); err != nil {
		t.Fatalf("WriteArchive: %v", err)
	}
	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("archive is not a readable ZIP: %v", err)
	}
	return r
}

func readMember(t *testing.T, r *zip.Reader, name string) string {
	t.Helper()
	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return string(data)
	}
	t.Fatalf("archive has no member %q", name)
	return ""
}

// TestArchiveContainsEveryDocumentedMember is the promise the archive makes:
// a pilot who downloads it has everything, not just the parts a vendor format
// happens to model.
func TestArchiveContainsEveryDocumentedMember(t *testing.T) {
	r := buildArchive(t, fixtureBundle())

	present := make(map[string]bool, len(r.File))
	for _, f := range r.File {
		present[f.Name] = true
	}

	for _, want := range []string{
		"manifest.json",
		"README.md",
		"flights.csv",
		"aircraft.csv",
		"licenses.csv",
		"class-ratings.csv",
		"credentials.csv",
		"contacts.csv",
		"crew.csv",
		"signatures.csv",
		"baseline.json",
	} {
		if !present[want] {
			t.Errorf("archive is missing %s", want)
		}
	}
}

// TestArchiveManifestDescribesReality catches a manifest that drifts from the
// files beside it — a reader trusting the row counts would otherwise silently
// process a truncated logbook.
func TestArchiveManifestDescribesReality(t *testing.T) {
	bundle := fixtureBundle()
	r := buildArchive(t, bundle)

	var m Manifest
	if err := json.Unmarshal([]byte(readMember(t, r, "manifest.json")), &m); err != nil {
		t.Fatalf("manifest.json is not valid JSON: %v", err)
	}

	if m.Format != ArchiveFormatID {
		t.Errorf("format = %q, want %q", m.Format, ArchiveFormatID)
	}
	if m.FormatVersion != ArchiveFormatVersion {
		t.Errorf("formatVersion = %q, want %q", m.FormatVersion, ArchiveFormatVersion)
	}
	if m.ExportedAt != "2026-03-14T09:30:00Z" {
		t.Errorf("exportedAt = %q, want the bundle's timestamp", m.ExportedAt)
	}
	if m.Counts["flights"] != len(bundle.Flights) {
		t.Errorf("counts.flights = %d, want %d", m.Counts["flights"], len(bundle.Flights))
	}

	// Every file the manifest lists must actually be in the archive.
	present := make(map[string]bool, len(r.File))
	for _, f := range r.File {
		present[f.Name] = true
	}
	for _, f := range m.Files {
		if !present[f.Path] {
			t.Errorf("manifest lists %q but the archive does not contain it", f.Path)
		}
	}

	// And the flight row count it advertises must match the CSV.
	records, err := csv.NewReader(strings.NewReader(readMember(t, r, "flights.csv"))).ReadAll()
	if err != nil {
		t.Fatalf("flights.csv is not valid CSV: %v", err)
	}
	if got := len(records) - 1; got != m.Counts["flights"] {
		t.Errorf("flights.csv has %d data rows, manifest claims %d", got, m.Counts["flights"])
	}
}

// TestArchiveCarriesWhatVendorFormatsCannot is the reason this format exists.
// Each of these records is dropped by at least one — usually every — vendor
// target, so losing them here would mean losing them entirely.
func TestArchiveCarriesWhatVendorFormatsCannot(t *testing.T) {
	r := buildArchive(t, fixtureBundle())

	cases := []struct{ member, want, why string }{
		{"licenses.csv", "DE.FCL.12345", "no vendor CSV carries the pilot's licence"},
		{"class-ratings.csv", "SEP_LAND", "class ratings and their expiry dates are logbook-adjacent, not flights"},
		{"credentials.csv", "BZF-I-9987", "medicals and radio certificates have no vendor column"},
		{"contacts.csv", "karl@example.test", "contact details are not crew names on a flight row"},
		{"crew.csv", "Peter Kopilot", "who was on board, per flight, with their role"},
		{"baseline.json", "310.00", "hours flown before this logbook began appear in no flight row"},
	}

	for _, tc := range cases {
		t.Run(tc.member, func(t *testing.T) {
			body := readMember(t, r, tc.member)
			if !strings.Contains(body, tc.want) {
				t.Errorf("%s does not contain %q — %s", tc.member, tc.want, tc.why)
			}
		})
	}
}

// TestArchiveBaselineIsNotSilentlyLost guards the subtlest data loss in the
// whole feature. The opening balance is not a flight, so every flight-shaped
// export drops it; a pilot reconciling totals after a migration would find
// hundreds of hours missing with no indication why.
func TestArchiveBaselineIsNotSilentlyLost(t *testing.T) {
	r := buildArchive(t, fixtureBundle())

	var baseline map[string]any
	if err := json.Unmarshal([]byte(readMember(t, r, "baseline.json")), &baseline); err != nil {
		t.Fatalf("baseline.json is not valid JSON: %v", err)
	}
	if got := baseline["totalHours"]; got != "310.00" {
		t.Errorf("totalHours = %v, want \"310.00\" (18600 minutes)", got)
	}
	if desc, _ := baseline["description"].(string); !strings.Contains(desc, "not flights") {
		t.Error("baseline.json must explain that these hours appear in no flight row")
	}

	readme := readMember(t, r, "README.md")
	if !strings.Contains(readme, "baseline.json") {
		t.Error("README.md must point a confused pilot at baseline.json when totals disagree")
	}
}

// TestArchiveOmitsBaselineWhenAbsent keeps the archive honest for a pilot who
// has no prior experience recorded.
func TestArchiveOmitsBaselineWhenAbsent(t *testing.T) {
	bundle := fixtureBundle()
	bundle.Baseline = nil
	r := buildArchive(t, bundle)

	for _, f := range r.File {
		if f.Name == "baseline.json" {
			t.Fatal("archive contains baseline.json for a pilot with no baseline")
		}
	}
}

// TestArchiveIsFormulaSafe extends the injection guard to the open format —
// these files are opened in spreadsheets at least as often as the vendor ones.
func TestArchiveIsFormulaSafe(t *testing.T) {
	r := buildArchive(t, fixtureBundle())
	flights := readMember(t, r, "flights.csv")

	for _, unsafe := range []string{",=SUM(", "\n=SUM("} {
		if strings.Contains(flights, unsafe) {
			t.Errorf("flights.csv emitted an unescaped formula cell (%q)", unsafe)
		}
	}
	if !strings.Contains(flights, "'=SUM(A1:A9)") {
		t.Error("flights.csv lost the remarks value instead of neutralizing it")
	}
}

// TestArchiveEmptyAccount checks the degenerate case end to end: a brand-new
// account must still produce a valid, readable archive rather than an error or
// a corrupt ZIP.
func TestArchiveEmptyAccount(t *testing.T) {
	r := buildArchive(t, &Bundle{ExportedAt: fixtureBundle().ExportedAt})

	records, err := csv.NewReader(strings.NewReader(readMember(t, r, "flights.csv"))).ReadAll()
	if err != nil {
		t.Fatalf("flights.csv is not valid CSV: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("expected a header row and no data, got %d rows", len(records))
	}

	var m Manifest
	if err := json.Unmarshal([]byte(readMember(t, r, "manifest.json")), &m); err != nil {
		t.Fatalf("manifest.json is not valid JSON: %v", err)
	}
	if m.Counts["flights"] != 0 {
		t.Errorf("counts.flights = %d, want 0", m.Counts["flights"])
	}
}

// TestArchiveFilename keeps the download name stable.
func TestArchiveFilename(t *testing.T) {
	got := ArchiveFilename(fixtureBundle().ExportedAt)
	if want := "ninerlog-logbook-2026-03-14.zip"; got != want {
		t.Errorf("ArchiveFilename() = %q, want %q", got, want)
	}
}
