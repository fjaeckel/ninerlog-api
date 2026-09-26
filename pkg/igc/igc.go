// Package igc parses FAI IGC flight recorder files and derives the facts a
// logbook needs from them. It performs no I/O; every input is bounded.
package igc

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Parser limits.
const (
	// MaxFileBytes is the largest file Parse accepts.
	MaxFileBytes = 5 * 1024 * 1024
	// MaxFixes is the largest number of B records Parse accepts.
	MaxFixes = 200_000
	// MaxLineBytes is the longest line Parse accepts.
	MaxLineBytes = 4096
	// MaxEvents is the number of E records kept; later ones are counted only.
	MaxEvents = 1000
	// MaxExtensions is the number of I-record extensions read.
	MaxExtensions = 64
	// MaxHeaderValueRunes caps every header string kept.
	MaxHeaderValueRunes = 100
)

// Parse errors. Every error Parse returns is a *ParseError wrapping one of
// these.
var (
	ErrEmpty         = errors.New("file is empty")
	ErrTooLarge      = errors.New("file exceeds the maximum size")
	ErrTooManyFixes  = errors.New("file has too many fixes")
	ErrBinaryContent = errors.New("file contains control characters and is not an IGC text file")
	ErrLineTooLong   = errors.New("line exceeds the maximum length")
	ErrNotIGC        = errors.New("file does not start with an IGC A record")
	ErrNoDate        = errors.New("file has no valid HFDTE date record")
	ErrNoFixes       = errors.New("file has no valid B-record fixes")
)

// ParseError reports why a file was rejected and, where it applies, the
// 1-based line number.
type ParseError struct {
	Err  error
	Line int
}

func (e *ParseError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("igc: line %d: %v", e.Line, e.Err)
	}
	return "igc: " + e.Err.Error()
}

func (e *ParseError) Unwrap() error { return e.Err }

func perr(err error, line int) error { return &ParseError{Err: err, Line: line} }

// Logger is the A record: the flight recorder's manufacturer and serial.
type Logger struct {
	Manufacturer string
	Serial       string
	Raw          string
}

// Header holds the H records Parse reads.
type Header struct {
	Date          time.Time
	Pilot         string
	GliderType    string
	GliderID      string
	CompetitionID string
}

// Extension is one I-record entry: a three-letter code at 1-based byte
// positions Start..End of each B record.
type Extension struct {
	Code  string
	Start int
	End   int
}

// Fix is one B record. ENL and MOP are -1 when the file does not carry them
// or the value is unreadable. Valid is false for a 'V' (2D or no GPS) fix.
type Fix struct {
	Time        time.Time
	Lat         float64
	Lon         float64
	Valid       bool
	PressureAlt int
	GNSSAlt     int
	ENL         int
	MOP         int
}

// Event is one E record.
type Event struct {
	Time time.Time
	Code string
}

// File is a parsed IGC file.
type File struct {
	Logger     Logger
	Header     Header
	Extensions []Extension
	Fixes      []Fix
	Events     []Event
	// HasENL and HasMOP report whether the I record declares the extension.
	HasENL bool
	HasMOP bool
	// SkippedFixes counts malformed or out-of-order B records.
	SkippedFixes int
	// DroppedEvents counts E records beyond MaxEvents.
	DroppedEvents int
}

type rawFix struct {
	day int
	sec int
	fix Fix
}

type rawEvent struct {
	day  int
	sec  int
	code string
}

type limits struct {
	maxBytes int
	maxFixes int
}

var defaultLimits = limits{maxBytes: MaxFileBytes, maxFixes: MaxFixes}

// Parse reads an IGC file. Lines may end in CRLF, LF or CR; unknown records
// are ignored; malformed or out-of-order B records are skipped and counted.
func Parse(data []byte) (*File, error) {
	return parse(data, defaultLimits)
}

func parse(data []byte, lim limits) (*File, error) {
	if len(data) == 0 {
		return nil, perr(ErrEmpty, 0)
	}
	if len(data) > lim.maxBytes {
		return nil, perr(ErrTooLarge, 0)
	}
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}

	f := &File{}
	var raws []rawFix
	var events []rawEvent
	enl, mop := Extension{}, Extension{}
	seenA, seenI := false, false
	bCount := 0
	day, lastSec := 0, -1

	lineNo := 0
	start := 0
	for start <= len(data) {
		end := start
		for end < len(data) && data[end] != '\n' && data[end] != '\r' {
			end++
		}
		line := data[start:end]
		lineNo++
		// A CRLF pair ends one line.
		next := end + 1
		if end < len(data) && data[end] == '\r' && next < len(data) && data[next] == '\n' {
			next++
		}
		start = next

		if len(line) > MaxLineBytes {
			return nil, perr(ErrLineTooLong, lineNo)
		}
		for _, b := range line {
			if (b < 0x20 && b != '\t') || b == 0x7f {
				return nil, perr(ErrBinaryContent, lineNo)
			}
		}
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		if !seenA {
			if line[0] != 'A' {
				return nil, perr(ErrNotIGC, lineNo)
			}
			seenA = true
			f.Logger = parseA(line)
			continue
		}

		switch line[0] {
		case 'H':
			parseH(line, &f.Header)
		case 'I':
			if !seenI {
				seenI = true
				f.Extensions = parseI(line)
				for _, e := range f.Extensions {
					switch e.Code {
					case "ENL":
						if enl.Code == "" {
							enl = e
							f.HasENL = true
						}
					case "MOP":
						if mop.Code == "" {
							mop = e
							f.HasMOP = true
						}
					}
				}
			}
		case 'B':
			bCount++
			if bCount > lim.maxFixes {
				return nil, perr(ErrTooManyFixes, lineNo)
			}
			sec, fix, ok := parseB(line, enl, mop)
			if !ok {
				f.SkippedFixes++
				continue
			}
			if lastSec >= 0 && sec+12*3600 < lastSec {
				day++
			} else if lastSec >= 0 && sec < lastSec {
				f.SkippedFixes++
				continue
			}
			lastSec = sec
			raws = append(raws, rawFix{day: day, sec: sec, fix: fix})
		case 'E':
			if len(events) >= MaxEvents {
				f.DroppedEvents++
				continue
			}
			if len(line) < 10 {
				continue
			}
			sec, ok := parseHHMMSS(line[1:7])
			if !ok {
				continue
			}
			evDay := day
			if lastSec >= 0 && sec+12*3600 < lastSec {
				evDay++
			}
			events = append(events, rawEvent{day: evDay, sec: sec, code: sanitize(string(line[7:10]))})
		}
	}

	if !seenA {
		return nil, perr(ErrEmpty, 0)
	}
	if f.Header.Date.IsZero() {
		return nil, perr(ErrNoDate, 0)
	}
	if len(raws) == 0 {
		return nil, perr(ErrNoFixes, 0)
	}

	base := f.Header.Date
	f.Fixes = make([]Fix, len(raws))
	for i, r := range raws {
		fx := r.fix
		fx.Time = base.AddDate(0, 0, r.day).Add(time.Duration(r.sec) * time.Second)
		f.Fixes[i] = fx
	}
	f.Events = make([]Event, 0, len(events))
	for _, e := range events {
		f.Events = append(f.Events, Event{
			Time: base.AddDate(0, 0, e.day).Add(time.Duration(e.sec) * time.Second),
			Code: e.code,
		})
	}
	return f, nil
}

func parseA(line []byte) Logger {
	l := Logger{Raw: sanitize(string(line[1:]))}
	if len(line) >= 4 {
		l.Manufacturer = sanitize(string(line[1:4]))
	}
	if len(line) >= 7 {
		l.Serial = sanitize(string(line[4:7]))
	}
	return l
}

// parseH reads the date, pilot, glider type, glider id and competition id.
// The first non-empty value of each wins.
func parseH(line []byte, h *Header) {
	if len(line) < 5 {
		return
	}
	code := string(line[2:5])
	value := string(line[5:])
	if i := strings.IndexByte(value, ':'); i >= 0 {
		value = value[i+1:]
	}
	switch code {
	case "DTE":
		if h.Date.IsZero() {
			if d, ok := parseDate(value); ok {
				h.Date = d
			}
		}
	case "PLT":
		setIfEmpty(&h.Pilot, value)
	case "GTY":
		setIfEmpty(&h.GliderType, value)
	case "GID":
		setIfEmpty(&h.GliderID, value)
	case "CID":
		setIfEmpty(&h.CompetitionID, value)
	}
}

func setIfEmpty(dst *string, value string) {
	if *dst != "" {
		return
	}
	*dst = sanitize(value)
}

// parseDate reads DDMMYY from the start of s ("150708" or "150708,01").
// Two-digit years below 80 are 20YY.
func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 6 {
		return time.Time{}, false
	}
	dd, ok1 := atoiDigits(s[0:2])
	mm, ok2 := atoiDigits(s[2:4])
	yy, ok3 := atoiDigits(s[4:6])
	if !ok1 || !ok2 || !ok3 {
		return time.Time{}, false
	}
	year := 2000 + yy
	if yy >= 80 {
		year = 1900 + yy
	}
	d := time.Date(year, time.Month(mm), dd, 0, 0, 0, 0, time.UTC)
	if d.Day() != dd || int(d.Month()) != mm {
		return time.Time{}, false
	}
	return d, true
}

// parseI reads "INN" followed by NN groups of SSEECCC.
func parseI(line []byte) []Extension {
	if len(line) < 3 {
		return nil
	}
	n, ok := atoiDigits(string(line[1:3]))
	if !ok {
		return nil
	}
	if n > MaxExtensions {
		n = MaxExtensions
	}
	out := make([]Extension, 0, n)
	for i := 0; i < n; i++ {
		off := 3 + i*7
		if off+7 > len(line) {
			break
		}
		s, ok1 := atoiDigits(string(line[off : off+2]))
		e, ok2 := atoiDigits(string(line[off+2 : off+4]))
		code := strings.ToUpper(string(line[off+4 : off+7]))
		if !ok1 || !ok2 || s < 36 || e < s || e > MaxLineBytes {
			continue
		}
		out = append(out, Extension{Code: sanitize(code), Start: s, End: e})
	}
	return out
}

// parseB reads a B record: time, latitude, longitude, validity, pressure and
// GNSS altitude, plus the ENL and MOP extensions when declared.
func parseB(line []byte, enl, mop Extension) (int, Fix, bool) {
	if len(line) < 35 {
		return 0, Fix{}, false
	}
	sec, ok := parseHHMMSS(line[1:7])
	if !ok {
		return 0, Fix{}, false
	}
	lat, ok := parseCoord(line[7:15], 2, 'N', 'S', 90)
	if !ok {
		return 0, Fix{}, false
	}
	lon, ok := parseCoord(line[15:24], 3, 'E', 'W', 180)
	if !ok {
		return 0, Fix{}, false
	}
	var valid bool
	switch line[24] {
	case 'A':
		valid = true
	case 'V':
		valid = false
	default:
		return 0, Fix{}, false
	}
	palt, ok1 := parseAlt(line[25:30])
	galt, ok2 := parseAlt(line[30:35])
	if !ok1 || !ok2 {
		return 0, Fix{}, false
	}
	return sec, Fix{
		Lat: lat, Lon: lon, Valid: valid,
		PressureAlt: palt, GNSSAlt: galt,
		ENL: extValue(line, enl), MOP: extValue(line, mop),
	}, true
}

func parseHHMMSS(b []byte) (int, bool) {
	if len(b) != 6 {
		return 0, false
	}
	h, ok1 := atoiDigits(string(b[0:2]))
	m, ok2 := atoiDigits(string(b[2:4]))
	s, ok3 := atoiDigits(string(b[4:6]))
	if !ok1 || !ok2 || !ok3 || h > 23 || m > 59 || s > 59 {
		return 0, false
	}
	return h*3600 + m*60 + s, true
}

// parseCoord reads DD(D)MMmmm followed by a hemisphere letter.
func parseCoord(b []byte, degDigits int, pos, neg byte, maxDeg int) (float64, bool) {
	if len(b) != degDigits+6 {
		return 0, false
	}
	deg, ok1 := atoiDigits(string(b[:degDigits]))
	minThousandths, ok2 := atoiDigits(string(b[degDigits : degDigits+5]))
	if !ok1 || !ok2 {
		return 0, false
	}
	if minThousandths >= 60000 {
		return 0, false
	}
	v := float64(deg) + float64(minThousandths)/60000.0
	if v > float64(maxDeg) {
		return 0, false
	}
	switch b[degDigits+5] {
	case pos:
		return v, true
	case neg:
		return -v, true
	}
	return 0, false
}

// parseAlt reads a five-character altitude in metres, optionally with a
// leading minus sign.
func parseAlt(b []byte) (int, bool) {
	s := string(b)
	neg := false
	if s[0] == '-' {
		neg = true
		s = s[1:]
	}
	v, ok := atoiDigits(s)
	if !ok {
		return 0, false
	}
	if neg {
		v = -v
	}
	return v, true
}

func extValue(line []byte, e Extension) int {
	if e.Code == "" || e.End > len(line) {
		return -1
	}
	v, ok := atoiDigits(string(line[e.Start-1 : e.End]))
	if !ok {
		return -1
	}
	return v
}

// atoiDigits parses a non-empty run of ASCII digits of at most 9 characters.
func atoiDigits(s string) (int, bool) {
	if s == "" || len(s) > 9 {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	v, err := strconv.Atoi(s)
	return v, err == nil
}

// sanitize trims s, drops invalid UTF-8 and non-printable runes, and caps it
// at MaxHeaderValueRunes runes.
func sanitize(s string) string {
	s = strings.ToValidUTF8(s, "")
	var b strings.Builder
	n := 0
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == utf8.RuneError {
			continue
		}
		if n >= MaxHeaderValueRunes {
			break
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}
