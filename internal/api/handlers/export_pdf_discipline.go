package handlers

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/flightrules"
	"github.com/fjaeckel/ninerlog-api/pkg/registration"
	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"
)

// ─────────────────────────────────────────────────────────────────────────────
// Discipline layouts — sailplane (AMC1 SFCL.050) and ultralight
// ─────────────────────────────────────────────────────────────────────────────

const (
	sailplaneRegulation  = "EASA Part-SFCL · AMC1 SFCL.050"
	ultralightRegulation = "Ultralight Pilot Logbook"
)

// disciplineSheet is a one-page-per-batch logbook layout.
type disciplineSheet struct {
	groups    []colGroup
	sub       []string
	baseW     []float64
	align     []string
	labelSpan int
	cells     func(f *models.Flight) []string
	totCells  func(t disciplineTotals) []string
}

// disciplineTotals accumulates the numeric columns of the discipline layouts.
type disciplineTotals struct {
	total, launches, landings int
	pic, dual, instr          int
}

func (t *disciplineTotals) add(f *models.Flight) {
	t.total += f.TotalTime
	t.launches += f.Launches
	t.landings += f.LandingsDay + f.LandingsNight
	t.pic += flightrules.PICColumnTime(f)
	t.dual += f.DualTime
	t.instr += f.DualGivenTime
}

func (t *disciplineTotals) addAll(o disciplineTotals) {
	t.total += o.total
	t.launches += o.launches
	t.landings += o.landings
	t.pic += o.pic
	t.dual += o.dual
	t.instr += o.instr
}

// addBaseline opens the balance with the pilot's prior experience; launches
// stay untouched.
func (t *disciplineTotals) addBaseline(b *models.FlightBaseline) {
	if !baselineApplies(b) {
		return
	}
	t.total += b.TotalMinutes
	t.landings += b.LandingsDay + b.LandingsNight
	t.pic += b.PICMinutes + b.PICUSMinutes + b.SPICMinutes
	t.dual += b.DualMinutes
	t.instr += b.DualGivenMinutes
}

// renderDisciplineSheets draws one page per batch of flights with the
// three-row totals block and signature strip.
func renderDisciplineSheets(d *pdfDoc, flights []*models.Flight, s disciplineSheet, b *models.FlightBaseline) {
	g, pdf := d.g, d.pdf
	colW := scaleWidths(s.baseW, g.usableWidth())

	rpp := g.logRowsPerPage()
	totalPages := (len(flights) + rpp - 1) / rpp
	pageNum := 0

	var cum disciplineTotals
	cum.addBaseline(b)

	for startIdx := 0; startIdx < len(flights); startIdx += rpp {
		endIdx := min(startIdx+rpp, len(flights))
		pageNum++

		d.startPage(fmt.Sprintf("Logbook Page %d of %d", pageNum, totalPages))
		d.drawHeader(colW, s.groups, s.sub)

		var pt disciplineTotals
		pdf.SetFont("Helvetica", "", g.fontBody)
		for i, f := range flights[startIdx:endIdx] {
			d.drawDataRow(colW, s.cells(f), s.align, i, f.ID)
			pt.add(f)
		}

		cumAfter := cum
		cumAfter.addAll(pt)
		d.drawTotalsRow(colW, s.labelSpan, "TOTAL THIS PAGE", s.totCells(pt), s.align, false)
		d.drawTotalsRow(colW, s.labelSpan, "TOTAL FROM PREVIOUS PAGES", s.totCells(cum), s.align, false)
		cum = cumAfter
		d.drawTotalsRow(colW, s.labelSpan, "TOTAL TIME", s.totCells(cum), s.align, true)

		d.drawSignatureBlock()
	}
}

// airborneClocks returns the take-off and landing clock times, or the
// logbook clocks when either is missing.
func airborneClocks(f *models.Flight) (takeoff, landing string) {
	c := models.ClocksOf(f)
	if c.Takeoff != nil && c.Landing != nil &&
		strings.TrimSpace(*c.Takeoff) != "" && strings.TrimSpace(*c.Landing) != "" {
		return strings.TrimSpace(*c.Takeoff), strings.TrimSpace(*c.Landing)
	}
	return c.LogbookClocks()
}

func fmtCount(v int) string {
	if v == 0 {
		return ""
	}
	return fmt.Sprintf("%d", v)
}

// ── Sailplane ────────────────────────────────────────────────────────────────

var launchMethodLabels = map[string]string{
	models.LaunchMethodWinch:      "Winch",
	models.LaunchMethodAerotow:    "Aerotow",
	models.LaunchMethodSelfLaunch: "Self-launch",
	models.LaunchMethodCar:        "Car",
	models.LaunchMethodBungee:     "Bungee",
}

// launchMethodLabel returns the printed label of a launch method.
func launchMethodLabel(m *string) string {
	if m == nil {
		return ""
	}
	v := strings.TrimSpace(*m)
	if l, ok := launchMethodLabels[v]; ok {
		return l
	}
	return v
}

var sailplaneSheet = disciplineSheet{
	groups: []colGroup{
		{"", 1}, {"AIRCRAFT", 2}, {"TAKE-OFF", 2}, {"LANDING", 2},
		{"FLIGHT TIME", 1}, {"LAUNCH", 2}, {"PILOT FUNCTION TIME", 3},
		{"REMARKS AND ENDORSEMENTS", 1},
	},
	sub: []string{
		"DATE", "TYPE", "REG", "PLACE", "TIME", "PLACE", "TIME",
		"", "METHOD", "LAUNCHES", "PIC", "DUAL", "FI(S)", "",
	},
	baseW: []float64{14, 18, 16, 22, 10, 22, 10, 14, 16, 14, 13, 13, 13, 70},
	align: []string{"C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "L"},
	// Label cells cover date through landing time.
	labelSpan: 7,
	cells: func(f *models.Flight) []string {
		to, ldg := airborneClocks(f)
		return []string{
			f.Date.Format("02.01.06"),
			f.AircraftType, f.AircraftReg,
			safeStr(f.DepartureICAO), fmtTime(&to),
			safeStr(f.ArrivalICAO), fmtTime(&ldg),
			fmtDec(f.TotalTime),
			launchMethodLabel(f.LaunchMethod), fmtCount(f.Launches),
			fmtDec(flightrules.PICColumnTime(f)), fmtDec(f.DualTime), fmtDec(f.DualGivenTime),
			truncRunes(flightrules.SailplaneRemarks(f), 60),
		}
	},
	totCells: func(t disciplineTotals) []string {
		return []string{
			fmtDec(t.total), "", fmt.Sprintf("%d", t.launches),
			fmtDec(t.pic), fmtDec(t.dual), fmtDec(t.instr), "",
		}
	},
}

// renderSailplane renders the AMC1 SFCL.050 sailplane logbook.
func renderSailplane(flights []*models.Flight, g pageGeometry, userName string, b *models.FlightBaseline, sigs map[uuid.UUID]*models.FlightSignature) *fpdf.Fpdf {
	d := newDoc(g, sailplaneRegulation, userName, certEASA)
	d.note = baselineFooterNote(b)
	d.sigs = sigs
	renderDisciplineSheets(d, flights, sailplaneSheet, b)
	addSailplaneSummaryPage(d, flights, b)
	return d.pdf
}

func addSailplaneSummaryPage(d *pdfDoc, flights []*models.Flight, b *models.FlightBaseline) {
	var t disciplineTotals
	t.addBaseline(b)
	byMethod := map[string]int{}
	flightCount, outlandings := 0, 0
	for _, f := range flights {
		t.add(f)
		flightCount++
		if f.IsOutlanding {
			outlandings++
		}
		if f.LaunchMethod != nil && f.Launches > 0 {
			byMethod[strings.TrimSpace(*f.LaunchMethod)] += f.Launches
		}
	}
	if baselineApplies(b) {
		flightCount += b.TotalFlights
	}
	rows := []summaryRow{
		{"Total Flights", fmt.Sprintf("%d", flightCount)},
		{"Total Flight Time", fmtDecTotal(t.total)},
		{"Total Launches", fmt.Sprintf("%d", t.launches)},
	}
	for _, m := range models.ValidLaunchMethods() {
		if n := byMethod[m]; n > 0 {
			rows = append(rows, summaryRow{"Launches " + emdash() + " " + launchMethodLabels[m], fmt.Sprintf("%d", n)})
		}
	}
	rows = append(rows,
		summaryRow{"PIC Time", fmtDec(t.pic)},
		summaryRow{"Dual Received", fmtDec(t.dual)},
		summaryRow{"FI(S) Instruction Given", fmtDec(t.instr)},
	)
	if outlandings > 0 {
		rows = append(rows, summaryRow{"Outlandings", fmt.Sprintf("%d", outlandings)})
	}
	note := baselineSummaryNote(b)
	if note != "" {
		note += " Launch counts cover logged flights only."
	}
	drawSummaryPage(d, withBroughtForward(rows, b), note)
}

// ── Ultralight ───────────────────────────────────────────────────────────────

var ulKindLabels = map[models.ULKind]string{
	models.ULKindThreeAxis:            "Three-axis",
	models.ULKindThreeAxisMotorglider: "Three-axis motorglider",
	models.ULKindWeightShift:          "Weight-shift",
	models.ULKindGyroplane:            "Gyroplane",
	models.ULKindHelicopter:           "Helicopter",
	models.ULKindPoweredParaglider:    "Powered paraglider",
	models.ULKindSailplane:            "Sailplane",
}

// ulKindCell returns the UL kind column of a flight: the kind label of an
// ultralight, "UL" for one with no kind, the aircraft class of any other fleet
// aircraft, and "" for a registration not in the fleet.
func ulKindCell(ac *models.Aircraft) string {
	if ac == nil {
		return ""
	}
	if models.IsULClass(ac.AircraftClass) {
		if ac.ULKind != nil {
			if l, ok := ulKindLabels[*ac.ULKind]; ok {
				return l
			}
		}
		return "UL"
	}
	return strings.ReplaceAll(string(models.NormalizeAircraftClass(ac.AircraftClass)), "_", " ")
}

func ultralightSheet(fleet map[string]*models.Aircraft) disciplineSheet {
	return disciplineSheet{
		groups: []colGroup{
			{"", 1}, {"AIRCRAFT", 3}, {"DEPARTURE", 2}, {"ARRIVAL", 2},
			{"FLIGHT TIME", 1}, {"LANDINGS", 1}, {"PILOT FUNCTION TIME", 3},
			{"REMARKS AND ENDORSEMENTS", 1},
		},
		sub: []string{
			"DATE", "REG", "TYPE", "UL KIND", "PLACE", "TIME", "PLACE", "TIME",
			"", "", "PIC", "DUAL", "INSTR", "",
		},
		baseW: []float64{14, 18, 18, 24, 22, 10, 22, 10, 14, 12, 14, 14, 14, 70},
		align: []string{"C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "C", "L"},
		// Label cells cover date through arrival time.
		labelSpan: 8,
		cells: func(f *models.Flight) []string {
			dep, arr := logbookClocks(f)
			return []string{
				f.Date.Format("02.01.06"),
				f.AircraftReg, f.AircraftType,
				ulKindCell(fleet[registration.Canonical(f.AircraftReg)]),
				safeStr(f.DepartureICAO), fmtTime(dep),
				safeStr(f.ArrivalICAO), fmtTime(arr),
				fmtDec(f.TotalTime),
				fmtCount(f.LandingsDay + f.LandingsNight),
				fmtDec(flightrules.PICColumnTime(f)), fmtDec(f.DualTime), fmtDec(f.DualGivenTime),
				truncRunes(flightrules.CombinedRemarks(f), 60),
			}
		},
		totCells: func(t disciplineTotals) []string {
			return []string{
				fmtDec(t.total), fmt.Sprintf("%d", t.landings),
				fmtDec(t.pic), fmtDec(t.dual), fmtDec(t.instr), "",
			}
		},
	}
}

// renderUltralight renders the ultralight logbook. fleet maps canonical
// registrations to the user's aircraft.
func renderUltralight(flights []*models.Flight, g pageGeometry, fleet map[string]*models.Aircraft, userName string, b *models.FlightBaseline, sigs map[uuid.UUID]*models.FlightSignature) *fpdf.Fpdf {
	d := newDoc(g, ultralightRegulation, userName, certEASA)
	d.note = baselineFooterNote(b)
	d.sigs = sigs
	renderDisciplineSheets(d, flights, ultralightSheet(fleet), b)
	addUltralightSummaryPage(d, flights, fleet, b)
	return d.pdf
}

func addUltralightSummaryPage(d *pdfDoc, flights []*models.Flight, fleet map[string]*models.Aircraft, b *models.FlightBaseline) {
	var t disciplineTotals
	t.addBaseline(b)
	byKind := map[string]int{}
	flightCount := 0
	for _, f := range flights {
		t.add(f)
		flightCount++
		if k := ulKindCell(fleet[registration.Canonical(f.AircraftReg)]); k != "" {
			byKind[k] += f.TotalTime
		}
	}
	if baselineApplies(b) {
		flightCount += b.TotalFlights
	}
	rows := []summaryRow{
		{"Total Flights", fmt.Sprintf("%d", flightCount)},
		{"Total Flight Time", fmtDecTotal(t.total)},
	}
	kinds := make([]string, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool {
		if byKind[kinds[i]] != byKind[kinds[j]] {
			return byKind[kinds[i]] > byKind[kinds[j]]
		}
		return kinds[i] < kinds[j]
	})
	for _, k := range kinds {
		if byKind[k] > 0 {
			rows = append(rows, summaryRow{"Flight Time " + emdash() + " " + k, fmtDec(byKind[k])})
		}
	}
	rows = append(rows,
		summaryRow{"PIC Time", fmtDec(t.pic)},
		summaryRow{"Dual Received", fmtDec(t.dual)},
		summaryRow{"Instruction Given", fmtDec(t.instr)},
		summaryRow{"Total Landings", fmt.Sprintf("%d", t.landings)},
	)
	drawSummaryPage(d, withBroughtForward(rows, b), baselineSummaryNote(b))
}

// fleetByRegistration indexes aircraft by canonical registration.
func fleetByRegistration(list []*models.Aircraft) map[string]*models.Aircraft {
	out := make(map[string]*models.Aircraft, len(list))
	for _, ac := range list {
		out[registration.Canonical(ac.Registration)] = ac
	}
	return out
}
