package handlers

import (
	"fmt"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// ─────────────────────────────────────────────────────────────────────────────
// Prior experience (flight baseline)
// ─────────────────────────────────────────────────────────────────────────────

// A flight baseline is the flying a pilot had accumulated before this
// logbook. Each PDF renderer seeds its cumulative totals from the baseline
// before the first sheet is drawn. A baseline does not record the
// single-/multi-engine split, FSTD session time, the FAA actual-vs-simulated
// instrument split, approaches, or holds, so those columns open at zero and
// count logged flights only; the summary page and every footer carry a
// disclosure of the limitation.

// baselineApplies reports whether b carries anything worth carrying forward.
// A nil baseline — or one whose every figure is zero — renders exactly like no
// baseline at all, footer disclosure included.
func baselineApplies(b *models.FlightBaseline) bool {
	if b == nil {
		return false
	}
	for _, v := range []int{
		b.TotalFlights, b.TotalMinutes, b.PICMinutes, b.SICMinutes,
		b.DualMinutes, b.DualGivenMinutes, b.MultiPilotMinutes,
		b.NightMinutes, b.IFRMinutes, b.SoloMinutes, b.CrossCountryMinutes,
		b.LandingsDay, b.LandingsNight,
	} {
		if v != 0 {
			return true
		}
	}
	return false
}

// baselineFooterNote is the one-line disclosure printed in the footer of every
// page of an export whose balances open with carried-forward hours. Empty when
// no baseline applies.
func baselineFooterNote(b *models.FlightBaseline) string {
	if !baselineApplies(b) {
		return ""
	}
	return fmt.Sprintf("Totals include %s brought forward (as of %s)",
		fmtDecTotal(b.TotalMinutes), b.BaselineDate.Format("02 Jan 2006"))
}

// baselineSummaryNote is the full disclosure printed under the totals table on
// the summary page: where the carried-forward block came from, and which
// columns it cannot contribute to. Empty when no baseline applies.
func baselineSummaryNote(b *models.FlightBaseline) string {
	if !baselineApplies(b) {
		return ""
	}
	s := fmt.Sprintf("These totals include %s brought forward", fmtDecTotal(b.TotalMinutes))
	if b.TotalFlights > 0 {
		s += fmt.Sprintf(" over %d flights", b.TotalFlights)
	}
	s += fmt.Sprintf(" from flying logged before this logbook, as recorded on %s. ",
		b.BaselineDate.Format("02 January 2006"))
	s += "A carried-forward record holds no single-/multi-engine split, no FSTD session time " +
		"and no approach or hold counts, so those columns on the logbook pages and in this " +
		"table cover logged flights only."
	return s
}
