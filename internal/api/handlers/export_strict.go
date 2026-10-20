package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

// shortHashLen is the number of hex characters of a signature's fields hash
// printed in an export.
const shortHashLen = 12

// loadStrictExport returns the audit trail for an export of flights, or nil
// when the user is in standard mode or the logbook service is not wired.
func (h *APIHandler) loadStrictExport(ctx context.Context, userID uuid.UUID, flights []*models.Flight) (*strictExport, error) {
	if h.logbookService == nil {
		return nil, nil
	}
	history, err := h.logbookService.ExportHistory(ctx, userID)
	if err != nil {
		return nil, err
	}
	return newStrictExport(history, flights), nil
}

// csvCorrections adds the strict-mode columns to a CSV export. A nil
// *csvCorrections adds nothing.
type csvCorrections struct {
	lastCorrectedAt map[uuid.UUID]time.Time
	prefs           exportPrefs
}

// loadCSVCorrections returns the strict-mode CSV columns for the user, or nil
// in standard mode or when the logbook service is not wired.
func (h *APIHandler) loadCSVCorrections(ctx context.Context, userID uuid.UUID, prefs exportPrefs) (*csvCorrections, error) {
	if h.logbookService == nil {
		return nil, nil
	}
	last, strict, err := h.logbookService.LastCorrectedAt(ctx, userID)
	if err != nil || !strict {
		return nil, err
	}
	return &csvCorrections{lastCorrectedAt: last, prefs: prefs}, nil
}

func (c *csvCorrections) header(h []string) []string {
	if c == nil {
		return h
	}
	return append(h, "corrected", "last_corrected_at")
}

func (c *csvCorrections) row(r []string, f *models.Flight) []string {
	if c == nil {
		return r
	}
	at, ok := c.lastCorrectedAt[f.ID]
	if !ok {
		return append(r, "false", "")
	}
	at = at.UTC()
	return append(r, "true", c.prefs.formatDate(at)+" "+at.Format("15:04"))
}

func (c *csvCorrections) totals(r []string) []string {
	if c == nil {
		return r
	}
	return append(r, "", "")
}

// strictExport carries the audit trail of a strict-mode user through the PDF
// and CSV renderers. A nil *strictExport is a standard-mode export.
type strictExport struct {
	history  *service.ExportHistory
	revision []service.ExportRevision
	refs     map[uuid.UUID]int
}

// newStrictExport prepares h for the flights being exported, which must
// already be in print order. It returns nil for a nil history.
func newStrictExport(h *service.ExportHistory, flights []*models.Flight) *strictExport {
	if h == nil {
		return nil
	}
	position := make(map[uuid.UUID]int, len(flights))
	for i, f := range flights {
		position[f.ID] = i
	}
	posOf := func(id uuid.UUID) int {
		if p, ok := position[id]; ok {
			return p
		}
		return len(flights)
	}

	revs := append([]service.ExportRevision(nil), h.Revisions...)
	sort.SliceStable(revs, func(i, j int) bool {
		a, b := revs[i], revs[j]
		if !a.Date.Equal(b.Date) {
			return a.Date.Before(b.Date)
		}
		if pa, pb := posOf(a.FlightID), posOf(b.FlightID); pa != pb {
			return pa < pb
		}
		if a.FlightID != b.FlightID {
			return a.FlightID.String() < b.FlightID.String()
		}
		return a.RevisionNo < b.RevisionNo
	})

	refs := map[uuid.UUID]int{}
	n := 0
	for _, f := range flights {
		if _, corrected := h.LastCorrectedAt[f.ID]; corrected {
			n++
			refs[f.ID] = n
		}
	}
	return &strictExport{history: h, revision: revs, refs: refs}
}

// marker returns the appendix marker of a corrected flight, or "".
func (s *strictExport) marker(id uuid.UUID) string {
	if s == nil {
		return ""
	}
	n, ok := s.refs[id]
	if !ok {
		return ""
	}
	return fmt.Sprintf("†%d", n)
}

// remarks returns combined cut to max runes with the flight's marker appended;
// the marker is never cut.
func (s *strictExport) remarks(id uuid.UUID, combined string, max int) string {
	m := s.marker(id)
	if m == "" {
		return truncRunes(combined, max)
	}
	room := max - len([]rune(m)) - 1
	body := strings.TrimRight(truncRunes(combined, room), " ")
	if room <= 0 || body == "" {
		return m
	}
	return body + " " + m
}

// footerNote is the strict-mode line added to every page footer.
func (s *strictExport) footerNote() string {
	if s == nil || s.history.Since == nil {
		return ""
	}
	return "History recorded since " + s.history.Since.UTC().Format("02 Jan 2006")
}

// joinNotes joins the non-empty footer notes.
func joinNotes(notes ...string) string {
	out := make([]string, 0, len(notes))
	for _, n := range notes {
		if n != "" {
			out = append(out, n)
		}
	}
	return strings.Join(out, " · ")
}

// shortSignatureHash returns the leading characters of a signature's fields
// hash, or "" when it has none.
func shortSignatureHash(sig *models.FlightSignature) string {
	if sig == nil || sig.SignedFieldsHash == nil {
		return ""
	}
	h := strings.TrimSpace(*sig.SignedFieldsHash)
	if len(h) > shortHashLen {
		h = h[:shortHashLen]
	}
	return h
}

// ─────────────────────────────────────────────────────────────────────────────
// Formatting
// ─────────────────────────────────────────────────────────────────────────────

const (
	appendixDate     = "2006-01-02"
	appendixDateTime = "2006-01-02 15:04"
)

var fieldAcronyms = map[string]string{
	"icao": "ICAO", "ifr": "IFR", "pic": "PIC", "sic": "SIC", "ipc": "IPC",
	"fstd": "FSTD", "picus": "PICUS", "spic": "SPIC", "id": "ID",
}

// fieldLabel renders a camelCase flight field name as words.
func fieldLabel(field string) string {
	var words []string
	start := 0
	for i, r := range field {
		if i > 0 && r >= 'A' && r <= 'Z' {
			words = append(words, field[start:i])
			start = i
		}
	}
	words = append(words, field[start:])
	for i, w := range words {
		w = strings.ToLower(w)
		if a, ok := fieldAcronyms[w]; ok {
			w = a
		} else if i == 0 && w != "" {
			w = strings.ToUpper(w[:1]) + w[1:]
		}
		words[i] = w
	}
	return strings.Join(words, " ")
}

// changeValue renders one side of a field change.
func changeValue(field string, v any) string {
	const maxLen = 70
	switch t := v.(type) {
	case nil:
		return "-"
	case json.Number:
		if strings.HasSuffix(field, "Time") {
			if n, err := t.Int64(); err == nil {
				return fmtDecTotal(int(n))
			}
		}
		return t.String()
	case string:
		if t == "" {
			return "-"
		}
		return truncRunes(t, maxLen)
	case bool:
		return yesNo(t)
	case []service.CrewEntry:
		if len(t) == 0 {
			return "-"
		}
		parts := make([]string, len(t))
		for i, c := range t {
			parts[i] = fmt.Sprintf("%s (%s)", c.Name, c.Role)
		}
		return truncRunes(strings.Join(parts, ", "), maxLen)
	default:
		return truncRunes(fmt.Sprint(t), maxLen)
	}
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func routeLabel(dep, arr *string) string {
	d, a := strings.TrimSpace(safeStr(dep)), strings.TrimSpace(safeStr(arr))
	if d == "" && a == "" {
		return "-"
	}
	return d + "-" + a
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func sourceLabel(source string) string {
	switch source {
	case repository.ChangeSourceUserEdit:
		return "User edit"
	case repository.ChangeSourceRecalc:
		return "System: recalculation"
	case repository.ChangeSourceAircraftRename:
		return "System: aircraft re-registration"
	case repository.ChangeSourceContactRename:
		return "System: contact rename"
	case repository.ChangeSourceImport:
		return "System: import"
	default:
		return "System: unknown"
	}
}

func triggerLabel(trigger string) string {
	switch trigger {
	case models.LogbookTriggerLicence:
		return "Licence"
	case models.LogbookTriggerManual:
		return "Manual"
	case models.LogbookTriggerImport:
		return "Import"
	default:
		return trigger
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Appendix
// ─────────────────────────────────────────────────────────────────────────────

var (
	modeBaseW    = []float64{40, 20, 40}
	modeAlign    = []string{"C", "C", "L"}
	changeBaseW  = []float64{9, 26, 17, 17, 24, 27, 78, 44, 9, 34}
	changeAlign  = []string{"C", "C", "C", "C", "C", "L", "L", "L", "C", "L"}
	voidedBaseW  = []float64{28, 18, 22, 28, 14, 120, 14}
	voidedAlign  = []string{"C", "C", "C", "C", "C", "L", "C"}
	sigHashBaseW = []float64{18, 22, 28, 60, 36, 28, 34}
	sigHashAlign = []string{"C", "C", "C", "L", "L", "C", "C"}
)

// drawStrictAppendix appends the logbook history, change log, voided entries
// and signature hash pages.
func (d *pdfDoc) drawStrictAppendix(flights []*models.Flight) {
	s := d.strict
	if s == nil {
		return
	}
	h := s.history

	lead := "History recorded since (not recorded)"
	if h.Since != nil {
		lead = "History recorded since " + h.Since.UTC().Format(appendixDateTime) + " UTC"
	}
	var modeRows [][]string
	for _, e := range h.Events {
		state := "off"
		if e.Strict {
			state = "on"
		}
		modeRows = append(modeRows, []string{e.CreatedAt.UTC().Format(appendixDateTime), state, triggerLabel(e.Trigger)})
	}
	d.drawTable("Logbook History", []string{lead, "Strict logbook mode switches"},
		[]string{"SWITCHED (UTC)", "STRICT MODE", "TRIGGER"}, modeBaseW, modeAlign, modeRows,
		"No mode switches recorded.")

	var changeRows [][]string
	for _, rev := range s.revision {
		ref := "-"
		if m := s.marker(rev.FlightID); m != "" {
			ref = m
		}
		base := []string{
			ref, rev.ChangedAt.UTC().Format(appendixDateTime), rev.Date.Format(appendixDate),
			orDash(rev.AircraftReg), routeLabel(rev.DepartureICAO, rev.ArrivalICAO),
		}
		tail := []string{orDash(safeStr(rev.Reason)), yesNo(rev.IsCorrection), sourceLabel(rev.Source)}
		if len(rev.Changes) == 0 {
			changeRows = append(changeRows, append(append(append([]string{}, base...), "-", "-"), tail...))
			continue
		}
		for _, c := range rev.Changes {
			change := changeValue(c.Field, c.OldValue) + " -> " + changeValue(c.Field, c.NewValue)
			changeRows = append(changeRows, append(append(append([]string{}, base...), fieldLabel(c.Field), change), tail...))
		}
	}
	d.drawTable("Change Log", []string{"Change log: every recorded update of a logbook entry. A dagger marks entries with a correction."},
		[]string{"REF", "CHANGED (UTC)", "FLIGHT DATE", "REG", "ROUTE", "FIELD", "OLD -> NEW", "REASON", "CORR.", "SOURCE"},
		changeBaseW, changeAlign, changeRows, "No changes recorded.")

	var voidedRows [][]string
	for _, v := range h.Voided {
		voidedRows = append(voidedRows, []string{
			v.DeletedAt.UTC().Format(appendixDateTime), v.Date.Format(appendixDate),
			orDash(v.AircraftReg), routeLabel(v.DepartureICAO, v.ArrivalICAO),
			fmtDecTotal(v.TotalTime), orDash(safeStr(v.Reason)), yesNo(v.IsCorrection),
		})
	}
	d.drawTable("Voided Entries", []string{"Voided entries: logbook entries deleted from the logbook."},
		[]string{"DELETED (UTC)", "FLIGHT DATE", "REGISTRATION", "ROUTE", "TOTAL", "REASON", "CORR."},
		voidedBaseW, voidedAlign, voidedRows, "No voided entries.")

	var sigRows [][]string
	for _, f := range flights {
		sig := d.sigs[f.ID]
		hash := shortSignatureHash(sig)
		if hash == "" {
			continue
		}
		signed := "-"
		if sig.SignedAt != nil {
			signed = sig.SignedAt.UTC().Format(appendixDate)
		}
		sigRows = append(sigRows, []string{
			f.Date.Format(appendixDate), orDash(f.AircraftReg), routeLabel(f.DepartureICAO, f.ArrivalICAO),
			orDash(strings.TrimSpace(safeStr(sig.InstructorName))), orDash(strings.TrimSpace(safeStr(sig.InstructorCredentialNo))),
			signed, hash,
		})
	}
	if len(sigRows) > 0 {
		d.drawTable("Signature Hashes", []string{"Signature hashes: leading characters of the SHA-256 over the entry as signed."},
			[]string{"FLIGHT DATE", "REG", "ROUTE", "INSTRUCTOR", "CREDENTIAL", "SIGNED", "HASH"},
			sigHashBaseW, sigHashAlign, sigRows, "")
	}
}

// drawTable draws a titled table over as many pages as its rows need. The
// lead lines print above the column header of the first page.
func (d *pdfDoc) drawTable(title string, lead, header []string, baseW []float64, align []string, rows [][]string, empty string) {
	saved := d.g
	defer func() { d.g = saved }()
	if ref := geometryFor(saved.sizeName); saved.rowH < ref.rowH {
		d.g.rowH, d.g.fontBody = ref.rowH, ref.fontBody
	}
	g, pdf := d.g, d.pdf
	widths := scaleWidths(baseW, g.usableWidth())
	groups := make([]colGroup, len(header))
	for i := range groups {
		groups[i] = colGroup{span: 1}
	}

	next := 0
	for page := 0; ; page++ {
		ctx := title
		if page > 0 {
			ctx += " (cont.)"
		}
		d.startPage(ctx)
		if page == 0 {
			pdf.SetFont("Helvetica", "B", g.fontBody+1)
			setText(pdf, colBand)
			for _, line := range lead {
				pdf.CellFormat(g.usableWidth(), g.rowH, d.tr(line), "", 1, "L", false, 0, "")
			}
			setText(pdf, [3]int{0, 0, 0})
			pdf.Ln(g.rowH / 2)
		}
		d.drawHeader(widths, groups, header)

		pdf.SetFont("Helvetica", "", g.fontBody)
		capacity := int((g.gridBottom() - pdf.GetY()) / g.rowH)
		if capacity < 1 {
			capacity = 1
		}
		for i := 0; i < capacity && next < len(rows); i++ {
			d.drawDataRow(widths, rows[next], align, next, uuid.Nil)
			next++
		}
		if len(rows) == 0 && empty != "" {
			pdf.SetFont("Helvetica", "I", g.fontBody)
			setText(pdf, colMuted)
			pdf.CellFormat(g.usableWidth(), g.rowH*1.5, d.tr(empty), "", 1, "L", false, 0, "")
			setText(pdf, [3]int{0, 0, 0})
		}
		if next >= len(rows) {
			return
		}
	}
}
