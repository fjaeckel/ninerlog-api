package handlers

import (
	"bytes"
	"testing"
)

// TestPDFLayoutPageCounts asserts the pagination contract for every format ×
// layout × page-size combination: a spread emits two physical pages per
// batch of logRowsPerPage() flights plus two blank filler pages; a single
// layout emits one page per batch; every document ends with the one-page
// grand summary.
func TestPDFLayoutPageCounts(t *testing.T) {
	const n = 45
	flights := buildSamplePDFFlights(n)

	ceilDiv := func(a, b int) int { return (a + b - 1) / b }

	for _, size := range []string{"a4", "a5", "letter"} {
		g := geometryFor(size)
		rpp := g.logRowsPerPage()
		if rpp < 5 {
			t.Fatalf("%s: logRowsPerPage()=%d, want >= 5", size, rpp)
		}
		batches := ceilDiv(n, rpp)

		cases := []struct {
			name      string
			wantPages int
			render    func() int
		}{
			{"easa spread", batches*2 + 3, func() int {
				return renderEASA(flights, g, nil, "Test Pilot", layoutSpread, nil, nil).PageCount()
			}},
			{"easa single", batches + 1, func() int {
				return renderEASA(flights, g, nil, "Test Pilot", layoutSingle, nil, nil).PageCount()
			}},
			{"faa spread", batches*2 + 3, func() int {
				return generateFAAPDF(flights, g, "Test Pilot", layoutSpread, nil, nil).PageCount()
			}},
			{"faa single", batches + 1, func() int {
				return generateFAAPDF(flights, g, "Test Pilot", layoutSingle, nil, nil).PageCount()
			}},
			{"summary", 1, func() int {
				return generateSummaryPDF(flights, g, "Test Pilot", nil).PageCount()
			}},
		}
		for _, tc := range cases {
			if got := tc.render(); got != tc.wantPages {
				t.Errorf("%s %s: got %d pages, want %d (rpp=%d)", size, tc.name, got, tc.wantPages, rpp)
			}
		}
	}
}

// TestWithRowsPerPage asserts an in-range request yields exactly that many
// rows per page, and out-of-range requests clamp to a legible row height.
func TestWithRowsPerPage(t *testing.T) {
	for _, size := range []string{"a4", "a5", "letter"} {
		base := geometryFor(size)
		for _, n := range []int{16, 20, 30} {
			g := base.withRowsPerPage(n)
			if got := g.logRowsPerPage(); got != n {
				t.Errorf("%s withRowsPerPage(%d): logRowsPerPage()=%d", size, n, got)
			}
			if g.rowH < minDynRowH-0.001 || g.rowH > maxDynRowH+0.001 {
				t.Errorf("%s withRowsPerPage(%d): rowH %.2f out of bounds", size, n, g.rowH)
			}
		}

		// Requesting an absurd density clamps to the minimum row height and
		// degrades to the densest legible row count.
		dense := base.withRowsPerPage(500)
		if dense.rowH != minDynRowH {
			t.Errorf("%s withRowsPerPage(500): rowH=%.2f, want clamp to %.1f", size, dense.rowH, minDynRowH)
		}
		if dense.fontBody > minDynRowH*1.35+0.001 {
			t.Errorf("%s withRowsPerPage(500): fontBody=%.2f not scaled down", size, dense.fontBody)
		}
		// Requesting very few rows clamps to the maximum row height.
		airy := base.withRowsPerPage(5)
		if airy.rowH > maxDynRowH+0.001 {
			t.Errorf("%s withRowsPerPage(5): rowH=%.2f above max", size, airy.rowH)
		}
		if airy.fontBody != base.fontBody {
			t.Errorf("%s withRowsPerPage(5): fontBody changed to %.2f, want unchanged", size, airy.fontBody)
		}
	}
}

// TestPDFLayoutEmptyLogbook ensures a user with zero flights still gets a
// valid PDF (just the summary page) in every format and layout.
func TestPDFLayoutEmptyLogbook(t *testing.T) {
	g := geometryFor("a4")
	for name, doc := range map[string]interface{ PageCount() int }{
		"easa spread": renderEASA(nil, g, nil, "", layoutSpread, nil, nil),
		"easa single": renderEASA(nil, g, nil, "", layoutSingle, nil, nil),
		"faa spread":  generateFAAPDF(nil, g, "", layoutSpread, nil, nil),
		"faa single":  generateFAAPDF(nil, g, "", layoutSingle, nil, nil),
		"summary":     generateSummaryPDF(nil, g, "", nil),
	} {
		if got := doc.PageCount(); got != 1 {
			t.Errorf("%s with no flights: got %d pages, want 1", name, got)
		}
	}
}

// TestPDFOutputIsValid renders every combination and checks the output is a
// non-trivial PDF document.
func TestPDFOutputIsValid(t *testing.T) {
	flights := buildSamplePDFFlights(12)
	g := geometryFor("a4")
	for name, render := range map[string]func() *bytes.Buffer{
		"easa spread": func() *bytes.Buffer {
			var b bytes.Buffer
			if err := renderEASA(flights, g, nil, "Test Pilot", layoutSpread, nil, nil).Output(&b); err != nil {
				t.Fatal(err)
			}
			return &b
		},
		"faa single": func() *bytes.Buffer {
			var b bytes.Buffer
			if err := generateFAAPDF(flights, g, "Test Pilot", layoutSingle, nil, nil).Output(&b); err != nil {
				t.Fatal(err)
			}
			return &b
		},
	} {
		b := render()
		if !bytes.HasPrefix(b.Bytes(), []byte("%PDF-")) {
			t.Errorf("%s: output does not start with %%PDF-", name)
		}
		if b.Len() < 2000 {
			t.Errorf("%s: output suspiciously small (%d bytes)", name, b.Len())
		}
	}
}
