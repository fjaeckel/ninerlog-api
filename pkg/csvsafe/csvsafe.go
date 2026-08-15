// Package csvsafe writes CSV whose cells cannot be re-interpreted as
// spreadsheet formulas by the application that opens them.
//
// Logbook data is user-controlled (remarks, endorsements, PIC and instructor
// names, registrations) and can also arrive from a third party via CSV import.
// Exports exist specifically to be handed to somebody else — an examiner, an
// authority, or another logbook product — so a cell like
// `=HYPERLINK("http://evil.test")` or `@SUM(...)` would execute on the
// recipient's machine when they open the file (CWE-1236).
//
// Every export format in this codebase funnels through Writer, so the guard
// holds by construction and a newly added column cannot reintroduce the
// injection.
package csvsafe

import (
	"encoding/csv"
	"io"
	"strings"
)

// FormulaLeaders are the characters that make Excel, LibreOffice Calc and
// Google Sheets treat a cell as a formula rather than as text. A tab or
// carriage return can also lead into one once the sheet re-parses the cell.
const FormulaLeaders = "=+-@\t\r"

// Cell defuses a single value. Prefixing with an apostrophe is the
// conventional defence: spreadsheet applications treat the value as literal
// text and do not display the apostrophe itself.
func Cell(s string) string {
	if s == "" || !strings.ContainsRune(FormulaLeaders, rune(s[0])) {
		return s
	}
	return "'" + s
}

// Record returns a copy of record with every field passed through Cell.
// The input slice is not modified.
func Record(record []string) []string {
	safe := make([]string, len(record))
	for i, field := range record {
		safe[i] = Cell(field)
	}
	return safe
}

// Writer wraps encoding/csv and neutralizes every field it writes.
//
// Like csv.Writer it buffers errors internally: once a write fails, later
// writes are no-ops and the error surfaces from Error or Flush.
type Writer struct {
	w *csv.Writer
}

// NewWriter returns a Writer emitting comma-separated records to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: csv.NewWriter(w)}
}

// NewWriterWithComma returns a Writer using a custom field delimiter. Some
// logbook products expect semicolon-separated files in locales where the
// comma is the decimal separator.
func NewWriterWithComma(w io.Writer, comma rune) *Writer {
	cw := csv.NewWriter(w)
	cw.Comma = comma
	return &Writer{w: cw}
}

// UseCRLF makes the writer terminate records with \r\n. Several desktop
// logbook importers are strict about this.
func (w *Writer) UseCRLF(v bool) { w.w.UseCRLF = v }

// Write neutralizes and writes a single record.
func (w *Writer) Write(record []string) error {
	return w.w.Write(Record(record))
}

// Flush writes any buffered data to the underlying writer.
func (w *Writer) Flush() { w.w.Flush() }

// Error reports the first error encountered by a previous Write or Flush.
func (w *Writer) Error() error { return w.w.Error() }
