package igc

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readFixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return b
}

func TestParse_FixtureHeaders(t *testing.T) {
	tests := []struct {
		file                      string
		date                      string
		pilot, gtype, gid, cid    string
		hasENL, hasMOP            bool
		manufacturer              string
		firstFix                  string
		skipped                   int
		invalidFixes              int
		minFixes                  int
		enlStart, enlEnd          int
		mopStart, mopEnd          int
		crlf                      bool
		lastFixDayAfterHeaderDate bool
	}{
		{file: "winch.igc", date: "2026-06-15", pilot: "Lena Example", gtype: "ASK 21", gid: "D-1234", cid: "LE",
			manufacturer: "XXX", firstFix: "10:00:01", skipped: 1, invalidFixes: 1, minFixes: 500, crlf: true},
		{file: "aerotow.igc", date: "2026-07-02", pilot: "Lena Example", gtype: "LS4", gid: "D-5678", cid: "LS",
			hasENL: true, enlStart: 36, enlEnd: 38, manufacturer: "XXX", firstFix: "12:00:01", minFixes: 1000},
		{file: "out_and_return.igc", date: "2026-07-02", pilot: "Lena Example", gtype: "LS4", gid: "D-5678", cid: "OR",
			hasENL: true, enlStart: 36, enlEnd: 38, manufacturer: "XXX", firstFix: "12:00:01", minFixes: 2000},
		{file: "selflaunch.igc", date: "2026-08-10", pilot: "Petra Example", gtype: "ASG 29E", gid: "D-KXYZ", cid: "PX",
			hasENL: true, hasMOP: true, enlStart: 36, enlEnd: 38, mopStart: 39, mopEnd: 41,
			manufacturer: "XXX", firstFix: "09:30:02", minFixes: 1000, crlf: true},
		{file: "outlanding.igc", date: "2026-06-20", pilot: "Lena Example", gtype: "ASK 21", gid: "D-1234", cid: "LE",
			manufacturer: "XXX", firstFix: "13:00:01", minFixes: 1000},
		{file: "midnight.igc", date: "2025-01-15", pilot: "Mark Example", gtype: "Duo Discus", gid: "ZK-GXX", cid: "XX",
			manufacturer: "XXX", firstFix: "23:20:02", minFixes: 2000, lastFixDayAfterHeaderDate: true},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			data := readFixture(t, tt.file)
			if got := bytes.Contains(data, []byte("\r\n")); got != tt.crlf {
				t.Fatalf("fixture CRLF = %v, want %v", got, tt.crlf)
			}
			f, err := Parse(data)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			h := f.Header
			if got := h.Date.Format("2006-01-02"); got != tt.date {
				t.Errorf("date = %s, want %s", got, tt.date)
			}
			if h.Pilot != tt.pilot || h.GliderType != tt.gtype || h.GliderID != tt.gid || h.CompetitionID != tt.cid {
				t.Errorf("header = %+v", h)
			}
			if f.Logger.Manufacturer != tt.manufacturer {
				t.Errorf("manufacturer = %q", f.Logger.Manufacturer)
			}
			if f.HasENL != tt.hasENL || f.HasMOP != tt.hasMOP {
				t.Errorf("HasENL/HasMOP = %v/%v", f.HasENL, f.HasMOP)
			}
			for _, e := range f.Extensions {
				switch e.Code {
				case "ENL":
					if e.Start != tt.enlStart || e.End != tt.enlEnd {
						t.Errorf("ENL offsets = %d-%d", e.Start, e.End)
					}
				case "MOP":
					if e.Start != tt.mopStart || e.End != tt.mopEnd {
						t.Errorf("MOP offsets = %d-%d", e.Start, e.End)
					}
				}
			}
			if len(f.Fixes) < tt.minFixes {
				t.Errorf("fixes = %d, want >= %d", len(f.Fixes), tt.minFixes)
			}
			if got := f.Fixes[0].Time.Format("15:04:05"); got != tt.firstFix {
				t.Errorf("first fix = %s, want %s", got, tt.firstFix)
			}
			if f.SkippedFixes != tt.skipped {
				t.Errorf("skipped = %d, want %d", f.SkippedFixes, tt.skipped)
			}
			invalid := 0
			for i, fx := range f.Fixes {
				if !fx.Valid {
					invalid++
				}
				if i > 0 && fx.Time.Before(f.Fixes[i-1].Time) {
					t.Fatalf("fix %d goes back in time", i)
				}
				if tt.hasENL && fx.ENL < 0 {
					t.Fatalf("fix %d has no ENL", i)
				}
				if !tt.hasENL && fx.ENL != -1 {
					t.Fatalf("fix %d has ENL %d without an I record", i, fx.ENL)
				}
				if tt.hasMOP && fx.MOP < 0 {
					t.Fatalf("fix %d has no MOP", i)
				}
			}
			if invalid != tt.invalidFixes {
				t.Errorf("invalid fixes = %d, want %d", invalid, tt.invalidFixes)
			}
			last := f.Fixes[len(f.Fixes)-1].Time
			nextDay := last.Format("2006-01-02") != tt.date
			if nextDay != tt.lastFixDayAfterHeaderDate {
				t.Errorf("last fix %s, crossing midnight = %v", last, nextDay)
			}
		})
	}
}

func TestParse_Fix(t *testing.T) {
	file := "AXXX001\nHFDTE010203\nI023638ENL3941MOP\n" +
		"B1101355206343N00006198WA0058700558123456\n" +
		"B1101365206343S00006198EV-001200000000999\n"
	f, err := Parse([]byte(file))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(f.Fixes) != 2 {
		t.Fatalf("fixes = %d", len(f.Fixes))
	}
	a := f.Fixes[0]
	if want := time.Date(2003, 2, 1, 11, 1, 35, 0, time.UTC); !a.Time.Equal(want) {
		t.Errorf("time = %s", a.Time)
	}
	if d := a.Lat - (52 + 6.343/60); d > 1e-9 || d < -1e-9 {
		t.Errorf("lat = %v", a.Lat)
	}
	if d := a.Lon + 6.198/60; d > 1e-9 || d < -1e-9 {
		t.Errorf("lon = %v", a.Lon)
	}
	if !a.Valid || a.PressureAlt != 587 || a.GNSSAlt != 558 || a.ENL != 123 || a.MOP != 456 {
		t.Errorf("fix = %+v", a)
	}
	b := f.Fixes[1]
	if b.Valid || b.Lat >= 0 || b.Lon <= 0 || b.PressureAlt != -12 || b.GNSSAlt != 0 || b.ENL != 0 || b.MOP != 999 {
		t.Errorf("fix = %+v", b)
	}
}

func TestParse_DateFormats(t *testing.T) {
	tests := []struct {
		name, line, want string
	}{
		{"old format", "HFDTE150708", "2008-07-15"},
		{"new format", "HFDTEDATE:150708,01", "2008-07-15"},
		{"new format without flight number", "HFDTEDATE:010199", "1999-01-01"},
		{"pilot-entered source", "HPDTE311224", "2024-12-31"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse([]byte("AXXX\n" + tt.line + "\nB1000005000000N01000000EA0010000100\n"))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got := f.Header.Date.Format("2006-01-02"); got != tt.want {
				t.Errorf("date = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestParse_LineEndings(t *testing.T) {
	body := []string{"AXXX", "HFDTE150708", "B1000005000000N01000000EA0010000100", "B1000015000000N01000000EA0010000100"}
	for name, sep := range map[string]string{"LF": "\n", "CRLF": "\r\n", "CR": "\r"} {
		t.Run(name, func(t *testing.T) {
			f, err := Parse([]byte(strings.Join(body, sep)))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(f.Fixes) != 2 {
				t.Errorf("fixes = %d", len(f.Fixes))
			}
		})
	}
}

func TestParse_Tolerates(t *testing.T) {
	tests := []struct {
		name    string
		lines   []string
		fixes   int
		skipped int
		check   func(t *testing.T, f *File)
	}{
		{
			name: "unknown records and blank lines",
			lines: []string{"AXXX", "", "HFDTE150708", "C150708100000000000000000", "LXXXcomment",
				"B1000005000000N01000000EA0010000100", "Zsomething", "G1234", "B1000015000000N01000000EA0010000100"},
			fixes: 2,
		},
		{
			name:    "malformed fixes are skipped",
			lines:   []string{"AXXX", "HFDTE150708", "B10000050000", "B9900005000000N01000000EA0010000100", "B1000005000000N01000000XA0010000100", "B1000005000000N01000000EA0010000100"},
			fixes:   1,
			skipped: 3,
		},
		{
			name:    "out-of-order fix is skipped",
			lines:   []string{"AXXX", "HFDTE150708", "B1000105000000N01000000EA0010000100", "B1000005000000N01000000EA0010000100", "B1000205000000N01000000EA0010000100"},
			fixes:   2,
			skipped: 1,
		},
		{
			name:  "midnight rollover",
			lines: []string{"AXXX", "HFDTE150708", "B2359585000000N01000000EA0010000100", "B0000025000000N01000000EA0010000100"},
			fixes: 2,
			check: func(t *testing.T, f *File) {
				if got := f.Fixes[1].Time; !got.Equal(time.Date(2008, 7, 16, 0, 0, 2, 0, time.UTC)) {
					t.Errorf("second fix = %s", got)
				}
			},
		},
		{
			name:  "malformed I record is ignored",
			lines: []string{"AXXX", "HFDTE150708", "I02ZZ38ENL0102MOP", "B1000005000000N01000000EA0010000100123"},
			fixes: 1,
			check: func(t *testing.T, f *File) {
				if f.HasENL || f.HasMOP || f.Fixes[0].ENL != -1 {
					t.Errorf("extensions = %+v", f.Extensions)
				}
			},
		},
		{
			name:  "ENL beyond a short record reads -1",
			lines: []string{"AXXX", "HFDTE150708", "I013638ENL", "B1000005000000N01000000EA0010000100"},
			fixes: 1,
			check: func(t *testing.T, f *File) {
				if !f.HasENL || f.Fixes[0].ENL != -1 {
					t.Errorf("ENL = %d", f.Fixes[0].ENL)
				}
			},
		},
		{
			name: "header values are sanitised",
			lines: []string{"AXXX", "HFDTE150708", "HFPLTPILOTINCHARGE:  M\xfcller \xff" + strings.Repeat("x", 300),
				"HFGIDGLIDERID:D-1234", "HFGIDGLIDERID:D-9999", "B1000005000000N01000000EA0010000100"},
			fixes: 1,
			check: func(t *testing.T, f *File) {
				if !strings.HasPrefix(f.Header.Pilot, "Mller x") || len([]rune(f.Header.Pilot)) > MaxHeaderValueRunes {
					t.Errorf("pilot = %q", f.Header.Pilot)
				}
				if f.Header.GliderID != "D-1234" {
					t.Errorf("glider id = %q", f.Header.GliderID)
				}
			},
		},
		{
			name:  "UTF-8 BOM",
			lines: []string{"\xef\xbb\xbfAXXX", "HFDTE150708", "B1000005000000N01000000EA0010000100"},
			fixes: 1,
		},
		{
			name:  "events are read",
			lines: []string{"AXXX", "HFDTE150708", "B1000005000000N01000000EA0010000100", "E100001PEV"},
			fixes: 1,
			check: func(t *testing.T, f *File) {
				if len(f.Events) != 1 || f.Events[0].Code != "PEV" {
					t.Errorf("events = %+v", f.Events)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse([]byte(strings.Join(tt.lines, "\n")))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(f.Fixes) != tt.fixes || f.SkippedFixes != tt.skipped {
				t.Errorf("fixes = %d skipped = %d, want %d/%d", len(f.Fixes), f.SkippedFixes, tt.fixes, tt.skipped)
			}
			if tt.check != nil {
				tt.check(t, f)
			}
		})
	}
}

func TestParse_Rejects(t *testing.T) {
	fix := "B1000005000000N01000000EA0010000100\n"
	tests := []struct {
		name string
		data []byte
		lim  limits
		want error
		line int
	}{
		{name: "empty", data: nil, want: ErrEmpty},
		{name: "only blank lines", data: []byte("\n\r\n\n"), want: ErrEmpty},
		{name: "too large", data: bytes.Repeat([]byte("A"), MaxFileBytes+1), want: ErrTooLarge},
		{name: "too many fixes", data: []byte("AXXX\nHFDTE150708\n" + strings.Repeat(fix, 4)), lim: limits{maxBytes: MaxFileBytes, maxFixes: 3}, want: ErrTooManyFixes, line: 6},
		{name: "NUL byte", data: []byte("AXXX\nHFDTE150708\nB10\x0000\n"), want: ErrBinaryContent, line: 3},
		{name: "escape sequence", data: []byte("AXXX\x1b[2J\n"), want: ErrBinaryContent, line: 1},
		{name: "DEL", data: []byte("AXXX\nHFPLT\x7f\n"), want: ErrBinaryContent, line: 2},
		{name: "PNG header", data: []byte("\x89PNG\r\n\x1a\n\x00\x00"), want: ErrNotIGC, line: 1},
		{name: "line too long", data: []byte("AXXX\nL" + strings.Repeat("x", MaxLineBytes) + "\n"), want: ErrLineTooLong, line: 2},
		{name: "not IGC", data: []byte("Date,Aircraft\n2024-01-01,D-1234\n"), want: ErrNotIGC, line: 1},
		{name: "no date", data: []byte("AXXX\n" + fix), want: ErrNoDate},
		{name: "invalid date", data: []byte("AXXX\nHFDTE310299\n" + fix), want: ErrNoDate},
		{name: "no fixes", data: []byte("AXXX\nHFDTE150708\nB12\n"), want: ErrNoFixes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lim := tt.lim
			if lim.maxBytes == 0 {
				lim = defaultLimits
			}
			_, err := parse(tt.data, lim)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("err %T is not a *ParseError", err)
			}
			if pe.Line != tt.line {
				t.Errorf("line = %d, want %d", pe.Line, tt.line)
			}
		})
	}
}

func FuzzParse(f *testing.F) {
	for _, name := range []string{"winch.igc", "selflaunch.igc", "midnight.igc"} {
		data := readFixture(f, name)
		f.Add(data[:min(len(data), 8192)])
	}
	f.Add([]byte("AXXX\nHFDTE150708\nI023638ENL3941MOP\nB1000005000000N01000000EA0010000100123456\n"))
	f.Add([]byte("AXXX\r\nHFDTEDATE:150708,01\r\nB2359595000000S01000000WV-0010-0010\r\nB0000005000000N01000000EA0010000100\r\n"))
	f.Add([]byte("AXXX\nHFDTE150708\nI99"))
	f.Fuzz(func(t *testing.T, data []byte) {
		file, err := Parse(data)
		if err != nil {
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("error %T is not a *ParseError", err)
			}
			return
		}
		if len(file.Fixes) == 0 || len(file.Fixes) > MaxFixes {
			t.Fatalf("fixes = %d", len(file.Fixes))
		}
		if file.Header.Date.IsZero() {
			t.Fatal("parsed without a date")
		}
		for i, fx := range file.Fixes {
			if fx.Lat < -90 || fx.Lat > 90 || fx.Lon < -180 || fx.Lon > 180 {
				t.Fatalf("fix %d out of range: %+v", i, fx)
			}
			if i > 0 && fx.Time.Before(file.Fixes[i-1].Time) {
				t.Fatalf("fix %d goes back in time", i)
			}
		}
		if len(file.Events) > MaxEvents {
			t.Fatalf("events = %d", len(file.Events))
		}
		for _, s := range []string{file.Header.Pilot, file.Header.GliderType, file.Header.GliderID, file.Header.CompetitionID, file.Logger.Raw} {
			if len([]rune(s)) > MaxHeaderValueRunes {
				t.Fatalf("header value too long: %q", s)
			}
		}
		if a, err := Analyze(file); err == nil {
			if a.Duration < 0 || a.FreeDistanceKm < 0 {
				t.Fatalf("analysis = %+v", a)
			}
		}
	})
}
