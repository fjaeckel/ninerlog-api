package handlers

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
)

func TestNeutralizeCSVCell(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"formula equals", `=HYPERLINK("http://evil.test")`, `'=HYPERLINK("http://evil.test")`},
		{"formula plus", "+SUM(1+1)", "'+SUM(1+1)"},
		{"formula minus", "-2+3", "'-2+3"},
		{"formula at", "@evil", "'@evil"},
		{"leading tab", "\tcmd", "'\tcmd"},
		{"leading cr", "\rcmd", "'\rcmd"},

		// Ordinary logbook values must pass through untouched.
		{"empty", "", ""},
		{"registration", "D-EFGH", "D-EFGH"},
		{"icao", "EDDF", "EDDF"},
		{"duration", "1:30", "1:30"},
		{"count", "2", "2"},
		{"remarks", "Training flight, 3 circuits", "Training flight, 3 circuits"},
		{"date", "01.02.2026", "01.02.2026"},
		{"embedded equals", "PIC = self", "PIC = self"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := neutralizeCSVCell(tt.in); got != tt.want {
				t.Errorf("neutralizeCSVCell(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// csvWrite is the single chokepoint every export format goes through, so
// neutralization must happen there rather than at individual call sites.
func TestCSVWrite_NeutralizesEveryColumn(t *testing.T) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	csvWrite(w, []string{"Date", "Remarks", "Instructor", "PIC Name"})
	csvWrite(w, []string{"01.02.2026", "=cmd|calc", "+SUM(1+1)", "@evil"})
	w.Flush()

	out := buf.String()
	for _, payload := range []string{"\n=cmd", ",=cmd", "\n+SUM", ",+SUM", "\n@evil", ",@evil"} {
		if strings.Contains(out, payload) {
			t.Errorf("unescaped formula cell reached the CSV output (%q):\n%s", payload, out)
		}
	}

	// Re-parse and confirm the values survive intact, just quoted as text.
	records, err := csv.NewReader(strings.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatalf("output is not valid CSV: %v", err)
	}
	row := records[1]
	if row[0] != "01.02.2026" {
		t.Errorf("benign cell was altered: %q", row[0])
	}
	for i, want := range map[int]string{1: "'=cmd|calc", 2: "'+SUM(1+1)", 3: "'@evil"} {
		if row[i] != want {
			t.Errorf("column %d = %q, want %q", i, row[i], want)
		}
	}
}
