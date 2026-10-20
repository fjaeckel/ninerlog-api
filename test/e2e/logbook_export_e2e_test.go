//go:build e2e

package e2e_test

import (
	"encoding/csv"
	"net/http"
	"strings"
	"testing"
	"time"
)

func csvRows(t *testing.T, r *Response) [][]string {
	t.Helper()
	requireStatus(t, r, http.StatusOK)
	rows, err := csv.NewReader(strings.NewReader(string(r.Body))).ReadAll()
	if err != nil {
		t.Fatalf("export is not CSV: %v", err)
	}
	return rows
}

func columnIndex(header []string, name string) int {
	for i, h := range header {
		if h == name {
			return i
		}
	}
	return -1
}

func TestStrictModeExports(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("strict-exp"), "SecurePass123!", "StrictExport")
	switchStrict(t, c, true)

	corrected := createFlightID(t, c, newFlightBody(pastDate(6), "D-ECORR"))
	untouched := createFlightID(t, c, newFlightBody(pastDate(5), "D-EKEEP"))
	voided := createFlightID(t, c, newFlightBody(pastDate(4), "D-EVOID"))

	body := newFlightBody(pastDate(6), "D-ECORR")
	body["remarks"] = "Landing count fixed"
	body["landings"] = 2
	body["correctionReason"] = "Counted the go-around twice"
	requireStatus(t, putFlight(c, corrected, body), http.StatusOK)
	requireStatus(t, c.DELETE("/flights/"+voided+"?reason=Duplicate+entry"), http.StatusNoContent)
	_ = untouched

	for _, format := range []string{"standard", "easa", "faa"} {
		t.Run("strict CSV "+format+" carries the correction columns", func(t *testing.T) {
			rows := csvRows(t, c.GET("/exports/csv?format="+format))
			header := rows[0]
			ci, ti := columnIndex(header, "corrected"), columnIndex(header, "last_corrected_at")
			if ci < 0 || ti < 0 || ti != ci+1 || ci != len(header)-2 {
				t.Fatalf("header = %v", header)
			}
			yes, no := 0, 0
			for _, row := range rows[1:] {
				switch row[ci] {
				case "true":
					yes++
					if _, err := time.Parse("02.01.2006 15:04", row[ti]); err != nil {
						t.Errorf("last_corrected_at = %q: %v", row[ti], err)
					}
				case "false":
					no++
					if row[ti] != "" {
						t.Errorf("uncorrected row has last_corrected_at = %q", row[ti])
					}
				default:
					t.Errorf("corrected = %q", row[ci])
				}
			}
			if yes != 1 || no != 1 {
				t.Errorf("corrected yes=%d no=%d, want 1 and 1 (the voided flight is not exported)", yes, no)
			}
		})
	}

	t.Run("strict totals row keeps the column count", func(t *testing.T) {
		rows := csvRows(t, c.GET("/exports/csv?totals=true"))
		if len(rows[len(rows)-1]) != len(rows[0]) {
			t.Errorf("totals row has %d cells, header %d", len(rows[len(rows)-1]), len(rows[0]))
		}
	})

	t.Run("weblogbook CSV is unchanged", func(t *testing.T) {
		rows := csvRows(t, c.GET("/exports/csv?format=weblogbook"))
		if columnIndex(rows[0], "corrected") >= 0 {
			t.Errorf("weblogbook header = %v", rows[0])
		}
	})

	strictPDF := map[string]*Response{}
	for _, format := range []string{"easa", "faa"} {
		for _, layout := range []string{"spread", "single"} {
			t.Run("strict PDF "+format+" "+layout+" carries the audit trail", func(t *testing.T) {
				r := c.GET("/exports/pdf?format=" + format + "&layout=" + layout)
				requireStatus(t, r, http.StatusOK)
				text := pdfStreamText(r.Body)
				for _, want := range []string{
					"Change Log", "Voided Entries", "Logbook History", "History recorded since",
					"Counted the go-around twice", "D-EVOID", "Duplicate entry", "\x861",
				} {
					if !strings.Contains(text, want) {
						t.Errorf("PDF lacks %q", want)
					}
				}
				strictPDF[format+layout] = r
			})
		}
	}

	t.Run("summary PDF is unchanged", func(t *testing.T) {
		r := c.GET("/exports/pdf?format=summary")
		requireStatus(t, r, http.StatusOK)
		if strings.Contains(pdfStreamText(r.Body), "Change Log") {
			t.Error("summary PDF carries the change log")
		}
	})

	t.Run("switching strict mode off returns the standard output", func(t *testing.T) {
		switchStrict(t, c, false)

		rows := csvRows(t, c.GET("/exports/csv"))
		if columnIndex(rows[0], "corrected") >= 0 || columnIndex(rows[0], "last_corrected_at") >= 0 {
			t.Errorf("standard header = %v", rows[0])
		}

		r := c.GET("/exports/pdf?format=easa&layout=spread")
		requireStatus(t, r, http.StatusOK)
		text := pdfStreamText(r.Body)
		for _, banned := range []string{"Change Log", "Voided Entries", "History recorded", "\x86"} {
			if strings.Contains(text, banned) {
				t.Errorf("standard PDF contains %q", banned)
			}
		}
		if strict := strictPDF["easaspread"]; strict != nil && len(strict.Body) <= len(r.Body) {
			t.Errorf("strict PDF (%d bytes) is not larger than the standard one (%d bytes)", len(strict.Body), len(r.Body))
		}
	})
}

func TestStandardModeExportsCarryNoAuditTrail(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("std-exp"), "SecurePass123!", "StandardExport")
	id := createFlightID(t, c, newFlightBody(pastDate(6), "D-ESTD"))
	body := newFlightBody(pastDate(6), "D-ESTD")
	body["remarks"] = "edited"
	requireStatus(t, putFlight(c, id, body), http.StatusOK)

	for _, format := range []string{"standard", "easa", "faa"} {
		rows := csvRows(t, c.GET("/exports/csv?format="+format))
		if columnIndex(rows[0], "corrected") >= 0 || columnIndex(rows[0], "last_corrected_at") >= 0 {
			t.Errorf("%s header = %v", format, rows[0])
		}
	}
	r := c.GET("/exports/pdf?format=faa")
	requireStatus(t, r, http.StatusOK)
	if strings.Contains(pdfStreamText(r.Body), "Change Log") {
		t.Error("standard PDF carries the change log")
	}
}
