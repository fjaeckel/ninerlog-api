package handlers

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/go-pdf/fpdf"
)

func testBaseline() *models.FlightBaseline {
	return &models.FlightBaseline{
		BaselineDate:        time.Date(2019, 3, 12, 0, 0, 0, 0, time.UTC),
		TotalFlights:        412,
		TotalMinutes:        73_920, // 1232:00
		PICMinutes:          51_300,
		SICMinutes:          4_140,
		DualMinutes:         18_480,
		DualGivenMinutes:    2_760,
		MultiPilotMinutes:   9_600,
		NightMinutes:        6_840,
		IFRMinutes:          11_100,
		SoloMinutes:         7_260,
		CrossCountryMinutes: 29_400,
		LandingsDay:         690,
		LandingsNight:       84,
	}
}

// TestBaselineApplies asserts a nil baseline and an all-zero baseline both
// render as "no baseline", disclosure notes included.
func TestBaselineApplies(t *testing.T) {
	if baselineApplies(nil) {
		t.Error("nil baseline should not apply")
	}
	empty := &models.FlightBaseline{BaselineDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}
	if baselineApplies(empty) {
		t.Error("all-zero baseline should not apply")
	}
	if !baselineApplies(testBaseline()) {
		t.Error("populated baseline should apply")
	}
	if !baselineApplies(&models.FlightBaseline{LandingsNight: 1}) {
		t.Error("baseline with a single non-zero figure should apply")
	}
}

// TestEASATotalsAddBaseline asserts every column a baseline records is
// seeded, and the single-/multi-engine split and FSTD session time stay at
// zero.
func TestEASATotalsAddBaseline(t *testing.T) {
	b := testBaseline()
	var got easaTotals
	got.addBaseline(b)

	want := easaTotals{
		mp: 9_600, total: 73_920,
		ldgD: 690, ldgN: 84,
		night: 6_840, ifr: 11_100,
		pic: 51_300, sic: 4_140, dual: 18_480, instr: 2_760,
	}
	if got != want {
		t.Errorf("addBaseline:\n got %+v\nwant %+v", got, want)
	}
	if got.se != 0 || got.me != 0 {
		t.Error("a baseline records no single-/multi-engine split; SE/ME must stay zero")
	}
	if got.fstd != 0 {
		t.Error("a baseline records no FSTD session time; fstd must stay zero")
	}

	var none easaTotals
	none.addBaseline(nil)
	if none != (easaTotals{}) {
		t.Errorf("nil baseline must leave the balance untouched, got %+v", none)
	}
}

// TestFAATotalsAddBaseline asserts actual/simulated instrument time,
// approaches and holds stay at zero.
func TestFAATotalsAddBaseline(t *testing.T) {
	b := testBaseline()
	var got faaTotals
	got.addBaseline(b)

	want := faaTotals{
		solo: 7_260, pic: 51_300, sic: 4_140, dual: 18_480, instr: 2_760,
		xc: 29_400, night: 6_840,
		ldgD: 690, ldgN: 84, total: 73_920,
	}
	if got != want {
		t.Errorf("addBaseline:\n got %+v\nwant %+v", got, want)
	}
	if got.act != 0 || got.sim != 0 || got.appr != 0 || got.holds != 0 {
		t.Error("actual/simulated instrument, approaches and holds are not part of a baseline")
	}

	var none faaTotals
	none.addBaseline(nil)
	if none != (faaTotals{}) {
		t.Errorf("nil baseline must leave the balance untouched, got %+v", none)
	}
}

// TestComputeSummaryTotals_IncludesBaseline checks the summary page reports a
// career total: logged flights plus everything carried forward.
func TestComputeSummaryTotals_IncludesBaseline(t *testing.T) {
	flights := buildSamplePDFFlights(6)
	b := testBaseline()

	logged := computeSummaryTotals(flights, nil)
	if logged.flights != 6 {
		t.Fatalf("without a baseline: got %d flights, want 6", logged.flights)
	}

	withBaseline := computeSummaryTotals(flights, b)
	if got, want := withBaseline.flights, logged.flights+b.TotalFlights; got != want {
		t.Errorf("flights: got %d, want %d", got, want)
	}
	if got, want := withBaseline.total, logged.total+b.TotalMinutes; got != want {
		t.Errorf("total: got %d, want %d", got, want)
	}
	if got, want := withBaseline.pic, logged.pic+b.PICMinutes; got != want {
		t.Errorf("pic: got %d, want %d", got, want)
	}
	if got, want := withBaseline.ldgNight, logged.ldgNight+b.LandingsNight; got != want {
		t.Errorf("night landings: got %d, want %d", got, want)
	}

	// An all-zero baseline must not shift a single figure.
	empty := computeSummaryTotals(flights, &models.FlightBaseline{})
	if empty != logged {
		t.Errorf("all-zero baseline changed the totals:\n got %+v\nwant %+v", empty, logged)
	}
}

// TestBaselineNotes checks the disclosures that tell a reader the balances are
// not purely logged time, and that they stay empty when nothing is carried
// forward.
func TestBaselineNotes(t *testing.T) {
	b := testBaseline()

	footer := baselineFooterNote(b)
	if !strings.Contains(footer, "1232:00") {
		t.Errorf("footer note should state the carried-forward time, got %q", footer)
	}
	if !strings.Contains(footer, "12 Mar 2019") {
		t.Errorf("footer note should state the baseline date, got %q", footer)
	}

	summary := baselineSummaryNote(b)
	for _, want := range []string{"1232:00", "412 flights", "12 March 2019", "single-/multi-engine"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary note missing %q, got %q", want, summary)
		}
	}

	noCount := testBaseline()
	noCount.TotalFlights = 0
	if strings.Contains(baselineSummaryNote(noCount), "0 flights") {
		t.Errorf("summary note should omit the flight count when it is zero, got %q", baselineSummaryNote(noCount))
	}

	if baselineFooterNote(nil) != "" || baselineSummaryNote(nil) != "" {
		t.Error("no baseline means no disclosure notes")
	}
}

// renderedText returns the uncompressed PDF stream as a string so tests can
// assert that a figure actually reached the page.
func renderedText(t *testing.T, doc *fpdf.Fpdf) string {
	t.Helper()
	doc.SetCompression(false)
	var buf bytes.Buffer
	if err := doc.Output(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestBaselineReachesLogbookBalances asserts a carried-forward co-pilot
// figure appears in the totals block of the logbook pages, not only the
// summary page.
func TestBaselineReachesLogbookBalances(t *testing.T) {
	flights := buildSamplePDFFlights(4)
	g := geometryFor("a4")

	// 61 h 11 m of co-pilot time: a value no sample flight produces.
	b := &models.FlightBaseline{
		BaselineDate: time.Date(2019, 3, 12, 0, 0, 0, 0, time.UTC),
		SICMinutes:   3_671,
	}

	// EASA prints h:mm on both the sheets and the summary page. The summary
	// accounts for exactly one occurrence, so two or more proves the figure
	// reached a sheet's "previous pages" and "total time" rows.
	for _, layout := range []string{layoutSpread, layoutSingle} {
		got := strings.Count(renderedText(t, renderEASA(flights, g, nil, "Test Pilot", layout, b)), "61:11")
		if got < 2 {
			t.Errorf("easa %s: found %d occurrences of the carried-forward co-pilot time, want >= 2", layout, got)
		}
		if control := strings.Count(renderedText(t, renderEASA(flights, g, nil, "Test Pilot", layout, nil)), "61:11"); control != 0 {
			t.Errorf("easa %s: control render without a baseline already contains the figure %d times", layout, control)
		}
	}

	// FAA sheets print decimal hours, which the summary page never does, so
	// "61.2" can only have come from a logbook page.
	for _, layout := range []string{layoutSpread, layoutSingle} {
		if got := strings.Count(renderedText(t, generateFAAPDF(flights, g, "Test Pilot", layout, b)), "61.2"); got == 0 {
			t.Errorf("faa %s: carried-forward co-pilot time never reached a logbook page", layout)
		}
		if control := strings.Count(renderedText(t, generateFAAPDF(flights, g, "Test Pilot", layout, nil)), "61.2"); control != 0 {
			t.Errorf("faa %s: control render without a baseline already contains the figure %d times", layout, control)
		}
	}
}

// TestBaselineDoesNotChangePagination guards the printing contract: carrying
// prior experience forward seeds existing rows, it never adds a row or a page.
func TestBaselineDoesNotChangePagination(t *testing.T) {
	flights := buildSamplePDFFlights(45)
	b := testBaseline()

	for _, size := range []string{"a4", "a5", "letter"} {
		g := geometryFor(size)
		cases := map[string]func(*models.FlightBaseline) int{
			"easa spread": func(b *models.FlightBaseline) int {
				return renderEASA(flights, g, nil, "Test Pilot", layoutSpread, b).PageCount()
			},
			"easa single": func(b *models.FlightBaseline) int {
				return renderEASA(flights, g, nil, "Test Pilot", layoutSingle, b).PageCount()
			},
			"faa spread": func(b *models.FlightBaseline) int {
				return generateFAAPDF(flights, g, "Test Pilot", layoutSpread, b).PageCount()
			},
			"faa single": func(b *models.FlightBaseline) int {
				return generateFAAPDF(flights, g, "Test Pilot", layoutSingle, b).PageCount()
			},
			"summary": func(b *models.FlightBaseline) int {
				return generateSummaryPDF(flights, g, "Test Pilot", b).PageCount()
			},
		}
		for name, render := range cases {
			if got, want := render(b), render(nil); got != want {
				t.Errorf("%s %s: %d pages with a baseline, %d without", size, name, got, want)
			}
		}
	}
}

// TestBaselineEmptyLogbook asserts a baseline with no logged flights still
// renders a valid one-page summary.
func TestBaselineEmptyLogbook(t *testing.T) {
	g := geometryFor("a4")
	b := testBaseline()

	for name, doc := range map[string]*fpdf.Fpdf{
		"easa spread": renderEASA(nil, g, nil, "", layoutSpread, b),
		"easa single": renderEASA(nil, g, nil, "", layoutSingle, b),
		"faa spread":  generateFAAPDF(nil, g, "", layoutSpread, b),
		"faa single":  generateFAAPDF(nil, g, "", layoutSingle, b),
		"summary":     generateSummaryPDF(nil, g, "", b),
	} {
		if got := doc.PageCount(); got != 1 {
			t.Errorf("%s with no flights: got %d pages, want 1", name, got)
		}
		if out := renderedText(t, doc); !strings.Contains(out, "1232:00") {
			t.Errorf("%s: summary page omits the carried-forward total", name)
		}
	}
}
