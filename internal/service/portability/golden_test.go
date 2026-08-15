package portability

import (
	"bytes"
	"encoding/csv"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Golden-file tests pin each vendor layout byte for byte.
//
// These layouts are contracts with software this project does not control.
// A reviewer cannot eyeball a forty-column CSV and see that a value shifted
// one column left, but a golden diff makes it obvious — and column drift is
// exactly the failure mode that silently writes a pilot's night hours into
// their cross-country column.
//
// Regenerate deliberately, never reflexively:
//
//	go test ./internal/service/portability/ -update
//
// and read the diff before committing it.

var update = flag.Bool("update", false, "rewrite golden files instead of comparing")

func goldenPath(name string) string { return filepath.Join("testdata", name) }

func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := goldenPath(name)

	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create it)", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output does not match %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

func renderTarget(t *testing.T, target Target) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(target, &buf, fixtureBundle()); err != nil {
		t.Fatalf("Write(%s): %v", target, err)
	}
	return buf.Bytes()
}

func TestGoldenForeFlight(t *testing.T) {
	assertGolden(t, "foreflight.csv", renderTarget(t, TargetForeFlight))
}

func TestGoldenLogTenPro(t *testing.T) {
	assertGolden(t, "logten.csv", renderTarget(t, TargetLogTenPro))
}

func TestGoldenMyFlightbook(t *testing.T) {
	assertGolden(t, "myflightbook.csv", renderTarget(t, TargetMyFlightbook))
}

func TestGoldenCrewLounge(t *testing.T) {
	assertGolden(t, "crewlounge.csv", renderTarget(t, TargetCrewLounge))
}

// TestEveryTargetRendersEveryFlight is the property that actually matters to a
// departing pilot: whatever the layout, no flight may be silently dropped.
//
// The fixture deliberately includes an aircraft that is not in the fleet and a
// simulator session, because those are the two rows a naive implementation
// loses.
func TestEveryTargetRendersEveryFlight(t *testing.T) {
	bundle := fixtureBundle()

	for _, d := range Targets() {
		t.Run(string(d.Target), func(t *testing.T) {
			var buf bytes.Buffer
			if err := Write(d.Target, &buf, bundle); err != nil {
				t.Fatalf("Write: %v", err)
			}

			out := buf.String()
			for _, f := range bundle.Flights {
				if !strings.Contains(out, f.AircraftReg) {
					t.Errorf("flight on %s (%s) is missing from the %s export",
						f.AircraftReg, f.Date.Format("2006-01-02"), d.Product)
				}
			}
		})
	}
}

// TestEveryTargetIsWellFormedCSV guards against a layout whose header and rows
// disagree on column count — the shape that makes an importer reject the whole
// file or, worse, accept it shifted.
//
// ForeFlight's template is intentionally ragged (a title line, two labelled
// tables), so it is checked per-table in TestForeFlightTableShape instead.
func TestEveryTargetIsWellFormedCSV(t *testing.T) {
	for _, d := range Targets() {
		if d.Target == TargetForeFlight {
			continue
		}
		t.Run(string(d.Target), func(t *testing.T) {
			var buf bytes.Buffer
			if err := Write(d.Target, &buf, fixtureBundle()); err != nil {
				t.Fatalf("Write: %v", err)
			}
			records, err := csv.NewReader(&buf).ReadAll()
			if err != nil {
				t.Fatalf("%s output is not valid CSV: %v", d.Product, err)
			}
			if len(records) < 2 {
				t.Fatalf("%s produced no data rows", d.Product)
			}
			width := len(records[0])
			for i, rec := range records {
				if len(rec) != width {
					t.Errorf("row %d has %d columns, header has %d", i, len(rec), width)
				}
			}
		})
	}
}

// TestForeFlightTableShape checks each of ForeFlight's two tables separately:
// the type row, the header row and every data row must agree on width, or
// values land in the wrong fields on import.
func TestForeFlightTableShape(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(TargetForeFlight, &buf, fixtureBundle()); err != nil {
		t.Fatalf("Write: %v", err)
	}

	r := csv.NewReader(&buf)
	r.FieldsPerRecord = -1 // the file is intentionally ragged between tables
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("output is not parseable as CSV: %v", err)
	}

	checkTable := func(label string, wantHeader []string) {
		t.Helper()
		start := -1
		for i, rec := range records {
			if len(rec) > 0 && rec[0] == label {
				start = i
				break
			}
		}
		if start < 0 {
			t.Fatalf("%q marker row not found", label)
		}
		typesRow := records[start+1]
		headerRow := records[start+2]

		if len(typesRow) != len(wantHeader) {
			t.Errorf("%s: type row has %d columns, want %d", label, len(typesRow), len(wantHeader))
		}
		if len(headerRow) != len(wantHeader) {
			t.Fatalf("%s: header row has %d columns, want %d", label, len(headerRow), len(wantHeader))
		}
		for i, want := range wantHeader {
			if headerRow[i] != want {
				t.Errorf("%s: column %d is %q, want %q", label, i, headerRow[i], want)
			}
		}

		for i := start + 3; i < len(records); i++ {
			rec := records[i]
			// A single empty cell separates the two tables.
			if len(rec) <= 1 {
				break
			}
			if len(rec) != len(wantHeader) {
				t.Errorf("%s: data row %d has %d columns, want %d", label, i, len(rec), len(wantHeader))
			}
		}
	}

	checkTable("Aircraft Table", foreFlightAircraftColumns)
	checkTable("Flights Table", foreFlightFlightColumns)
}

// TestUnfleetedAircraftIsExported covers the case that would quietly cost a
// pilot their oldest entries: ForeFlight rejects a flight whose AircraftID has
// no row in the aircraft table.
func TestUnfleetedAircraftIsExported(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(TargetForeFlight, &buf, fixtureBundle()); err != nil {
		t.Fatalf("Write: %v", err)
	}

	r := csv.NewReader(&buf)
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	inAircraftTable := false
	found := false
	for _, rec := range records {
		if len(rec) > 0 && rec[0] == "Aircraft Table" {
			inAircraftTable = true
			continue
		}
		if len(rec) > 0 && rec[0] == "Flights Table" {
			break
		}
		if inAircraftTable && len(rec) > 0 && rec[0] == "G-OLDY" {
			found = true
		}
	}
	if !found {
		t.Error("G-OLDY flies in the fixture but was never added to the fleet; " +
			"it must still appear in the aircraft table or ForeFlight drops its flight")
	}
}

// TestFormulaInjectionIsNeutralizedInEveryTarget makes the security control
// non-optional across formats. The fixture's remarks begin with "=".
func TestFormulaInjectionIsNeutralizedInEveryTarget(t *testing.T) {
	for _, d := range Targets() {
		t.Run(string(d.Target), func(t *testing.T) {
			var buf bytes.Buffer
			if err := Write(d.Target, &buf, fixtureBundle()); err != nil {
				t.Fatalf("Write: %v", err)
			}
			out := buf.String()
			for _, unsafe := range []string{",=SUM(", "\n=SUM("} {
				if strings.Contains(out, unsafe) {
					t.Errorf("%s emitted an unescaped formula cell (%q)", d.Product, unsafe)
				}
			}
			if !strings.Contains(out, "'=SUM(A1:A9)") {
				t.Errorf("%s lost the remarks value entirely instead of neutralizing it", d.Product)
			}
		})
	}
}

// TestLookupRejectsUnknownTarget pins the error the handler turns into a 400.
func TestLookupRejectsUnknownTarget(t *testing.T) {
	if _, err := Lookup(Target("logbookpro")); err == nil {
		t.Fatal("expected an error for an unsupported target")
	}
}

// TestDescriptorFilename keeps download names stable and predictable.
func TestDescriptorFilename(t *testing.T) {
	d, err := Lookup(TargetForeFlight)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	got := d.Filename(fixtureBundle().ExportedAt)
	if want := "ninerlog-foreflight-2026-03-14.csv"; got != want {
		t.Errorf("Filename() = %q, want %q", got, want)
	}
}
