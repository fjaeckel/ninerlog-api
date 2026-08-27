package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/airports"
	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// The analytics endpoint backs the whole Reports page from a single request.
// Every section is a small aggregate query served by the reports repository;
// the only work done here is what SQL cannot express cheaply (airport/country
// lookup against the OurAirports dataset, month streaks, and the derived
// records) plus the mapping onto the API shape.

const (
	defaultAnalyticsRowLimit = 25
	maxAnalyticsRowLimit     = 200
	maxAnalyticsMonths       = 600
	isoDate                  = "2006-01-02"
	isoMonth                 = "2006-01"
)

// analyticsAllTime reports whether a months parameter covers all time.
func analyticsAllTime(months int) bool { return months <= 0 }

// analyticsFrom returns the first date covered by the timeframe, or the zero
// time when it covers all time.
func analyticsFrom(months int, now time.Time) time.Time {
	if analyticsAllTime(months) {
		return time.Time{}
	}
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start.AddDate(0, -months, 0)
}

// analyticsCoversBaseline reports whether an initial-hours snapshot dated
// `baselineDate` falls inside the timeframe. The snapshot stands for
// everything flown on or before its cutoff, so it is excluded only when the
// window starts after it — the same rule GET /users/me/statistics applies.
func analyticsCoversBaseline(months int, baselineDate, now time.Time) bool {
	if analyticsAllTime(months) {
		return true
	}
	return !analyticsFrom(months, now).After(baselineDate)
}

// GetFlightAnalytics implements GET /reports/analytics
func (h *APIHandler) GetFlightAnalytics(c *gin.Context, params generated.GetFlightAnalyticsParams) {
	userID, err := h.getUserIDFromContext(c)
	if err != nil {
		h.sendError(c, http.StatusUnauthorized, "Unauthorized")
		return
	}

	months := 12
	if params.Months != nil {
		if *params.Months < 0 || *params.Months > maxAnalyticsMonths {
			h.sendError(c, http.StatusBadRequest, "months must be between 0 and 600")
			return
		}
		months = *params.Months
	}

	limit := defaultAnalyticsRowLimit
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > maxAnalyticsRowLimit {
			h.sendError(c, http.StatusBadRequest, "limit must be between 1 and 200")
			return
		}
		limit = *params.Limit
	}

	ctx := c.Request.Context()
	now := time.Now().UTC()

	out := generated.FlightAnalytics{
		Range: generated.AnalyticsRange{
			Months:  months,
			AllTime: analyticsAllTime(months),
			To:      strPtr(now.Format(isoDate)),
		},
	}
	if !analyticsAllTime(months) {
		out.Range.From = strPtr(analyticsFrom(months, now).Format(isoDate))
	}

	// Each loader fills one section. They run sequentially against a single
	// pooled connection; a failure in any one is a 500 for the whole page
	// rather than a silently half-rendered report.
	type loader struct {
		name string
		run  func() error
	}
	loaders := []loader{
		{"totals", func() (err error) { out.Totals, err = h.analyticsTotals(ctx, userID, months); return }},
		{"monthly", func() (err error) { out.Monthly, err = h.analyticsMonthly(ctx, userID, months); return }},
		{"yearly", func() (err error) { out.Yearly, err = h.analyticsYearly(ctx, userID, months); return }},
		{"aircraft type", func() (err error) {
			out.ByAircraftType, err = h.analyticsByAircraft(ctx, userID, months, limit, false)
			return
		}},
		{"registration", func() (err error) {
			out.ByRegistration, err = h.analyticsByAircraft(ctx, userID, months, limit, true)
			return
		}},
		{"class", func() (err error) {
			out.ByClass, err = h.analyticsGroups(ctx, userID, months, h.reportsRepo.ByClass)
			return
		}},
		{"category", func() (err error) {
			out.ByCategory, err = h.analyticsGroups(ctx, userID, months, h.reportsRepo.ByCategory)
			return
		}},
		{"airports", func() (err error) { out.ByAirport, err = h.analyticsByAirport(ctx, userID, months, limit); return }},
		{"routes", func() (err error) { out.ByRoute, err = h.analyticsByRoute(ctx, userID, months, limit); return }},
		{"instructors", func() (err error) {
			out.ByInstructor, err = h.analyticsPeople(ctx, userID, months, limit, h.reportsRepo.ByInstructor)
			return
		}},
		{"crew", func() (err error) {
			out.ByCrew, err = h.analyticsPeople(ctx, userID, months, limit, h.reportsRepo.ByCrew)
			return
		}},
		{"approaches", func() (err error) { out.ApproachTypes, err = h.analyticsApproachTypes(ctx, userID, months); return }},
		{"patterns", func() error { return h.analyticsPatterns(ctx, userID, months, &out) }},
		{"records", func() (err error) { out.Records, err = h.analyticsRecords(ctx, userID, months, now); return }},
	}
	for _, l := range loaders {
		if err := l.run(); err != nil {
			h.sendError(c, http.StatusInternalServerError, fmt.Sprintf("Failed to compute %s analytics", l.name))
			return
		}
	}

	// byCountry and the airport-derived records need the resolved airport
	// rows, so they are folded in once byAirport is populated.
	out.ByCountry = countriesFromAirports(out.ByAirport)
	applyAirportRecords(&out)

	// The initial-hours snapshot is pre-existing experience that was never
	// entered as flights. It is carried into the totals whenever the timeframe
	// reaches back to it, so this page agrees with the dashboard statistics.
	// Only `totals` can take it — no month, aircraft or airport owns it.
	baseline, err := h.analyticsBaseline(ctx, userID)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to load initial hours")
		return
	}
	if baseline != nil && analyticsCoversBaseline(months, baseline.BaselineDate, now) {
		addBaselineToTotals(&out.Totals, baseline)
		out.Baseline = baselineContribution(baseline)
	}

	// Cumulative career hours: everything logged before the timeframe plus
	// the user's initial-hours snapshot, so the curve is not truncated by the
	// selected range.
	carried, err := h.reportsRepo.CarriedForwardMinutes(ctx, userID, months)
	if err != nil {
		h.sendError(c, http.StatusInternalServerError, "Failed to compute cumulative hours")
		return
	}
	if baseline != nil {
		carried += baseline.TotalMinutes
	}
	running := carried
	for i := range out.Monthly {
		running += out.Monthly[i].TotalMinutes
		out.Monthly[i].CumulativeMinutes = running
	}

	out.Totals.DistinctAirports = len(out.ByAirport)
	out.Totals.DistinctCountries = len(out.ByCountry)

	c.JSON(http.StatusOK, out)
}

// ── Totals ───────────────────────────────────────────────────────────────

func (h *APIHandler) analyticsTotals(ctx context.Context, userID uuid.UUID, months int) (generated.AnalyticsTotals, error) {
	var t generated.AnalyticsTotals
	row, err := h.reportsRepo.Totals(ctx, userID, months)
	if err != nil {
		return t, err
	}
	t.TotalFlights = row.TotalFlights
	t.TotalMinutes = row.TotalMinutes
	t.PicMinutes = row.PicMinutes
	t.SicMinutes = row.SicMinutes
	t.DualMinutes = row.DualMinutes
	t.DualGivenMinutes = row.DualGivenMinutes
	t.PicusMinutes = row.PicusMinutes
	t.SpicMinutes = row.SpicMinutes
	t.ExaminerMinutes = row.ExaminerMinutes
	t.ReliefMinutes = row.ReliefMinutes
	t.SoloMinutes = row.SoloMinutes
	t.NightMinutes = row.NightMinutes
	t.IfrMinutes = row.IfrMinutes
	t.ActualInstrumentMinutes = row.ActualInstrumentMinutes
	t.SimulatedInstrumentMinutes = row.SimulatedInstrumentMinutes
	t.CrossCountryMinutes = row.CrossCountryMinutes
	t.MultiPilotMinutes = row.MultiPilotMinutes
	t.SimulatedFlightMinutes = row.SimulatedFlightMinutes
	t.GroundTrainingMinutes = row.GroundTrainingMinutes
	t.LandingsDay = row.LandingsDay
	t.LandingsNight = row.LandingsNight
	t.TakeoffsDay = row.TakeoffsDay
	t.TakeoffsNight = row.TakeoffsNight
	t.Approaches = row.Approaches
	t.Holds = row.Holds
	t.DistanceNm = row.DistanceNM
	t.DistinctRegistrations = row.DistinctRegistrations
	t.DistinctTypes = row.DistinctTypes
	if row.FirstFlightDate != nil {
		t.FirstFlightDate = strPtr(row.FirstFlightDate.Format(isoDate))
	}
	if row.LastFlightDate != nil {
		t.LastFlightDate = strPtr(row.LastFlightDate.Format(isoDate))
	}
	return t, nil
}

// ── Time series ──────────────────────────────────────────────────────────

func (h *APIHandler) analyticsMonthly(ctx context.Context, userID uuid.UUID, months int) ([]generated.AnalyticsMonthPoint, error) {
	rows, err := h.reportsRepo.Monthly(ctx, userID, months)
	if err != nil {
		return nil, err
	}
	out := []generated.AnalyticsMonthPoint{}
	for _, row := range rows {
		out = append(out, generated.AnalyticsMonthPoint{
			Month:            row.Month,
			Flights:          row.Flights,
			TotalMinutes:     row.TotalMinutes,
			PicMinutes:       row.PicMinutes,
			SicMinutes:       row.SicMinutes,
			DualMinutes:      row.DualMinutes,
			DualGivenMinutes: row.DualGivenMinutes,
			SoloMinutes:      row.SoloMinutes,
			NightMinutes:     row.NightMinutes,
			IfrMinutes:       row.IfrMinutes,
			LandingsDay:      row.LandingsDay,
			LandingsNight:    row.LandingsNight,
			DistanceNm:       row.DistanceNM,
		})
	}
	return out, nil
}

func (h *APIHandler) analyticsYearly(ctx context.Context, userID uuid.UUID, months int) ([]generated.AnalyticsYearPoint, error) {
	rows, err := h.reportsRepo.Yearly(ctx, userID, months)
	if err != nil {
		return nil, err
	}
	out := []generated.AnalyticsYearPoint{}
	for _, row := range rows {
		out = append(out, generated.AnalyticsYearPoint{
			Year:         row.Year,
			Flights:      row.Flights,
			TotalMinutes: row.TotalMinutes,
			PicMinutes:   row.PicMinutes,
			DualMinutes:  row.DualMinutes,
			NightMinutes: row.NightMinutes,
			IfrMinutes:   row.IfrMinutes,
			Landings:     row.Landings,
			DistanceNm:   row.DistanceNM,
		})
	}
	return out, nil
}

// ── Initial hours ────────────────────────────────────────────────────────

// analyticsBaseline loads the user's initial-hours snapshot, or nil when they
// have none.
func (h *APIHandler) analyticsBaseline(ctx context.Context, userID uuid.UUID) (*models.FlightBaseline, error) {
	if h.flightService == nil {
		return nil, nil
	}
	b, err := h.flightService.GetBaseline(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return b, nil
}

// addBaselineToTotals folds the snapshot into the aggregate totals. Only the
// columns a baseline records are touched — approaches, distance, takeoffs and
// the distinct-aircraft counts are not part of the snapshot, and the
// first-flight date deliberately keeps pointing at the first *logged* flight.
func addBaselineToTotals(t *generated.AnalyticsTotals, b *models.FlightBaseline) {
	t.TotalFlights += b.TotalFlights
	t.TotalMinutes += b.TotalMinutes
	t.PicMinutes += b.PICMinutes
	t.SicMinutes += b.SICMinutes
	t.DualMinutes += b.DualMinutes
	t.DualGivenMinutes += b.DualGivenMinutes
	t.PicusMinutes += b.PICUSMinutes
	t.SpicMinutes += b.SPICMinutes
	t.ExaminerMinutes += b.ExaminerMinutes
	t.ReliefMinutes += b.ReliefMinutes
	t.MultiPilotMinutes += b.MultiPilotMinutes
	t.SoloMinutes += b.SoloMinutes
	t.NightMinutes += b.NightMinutes
	t.IfrMinutes += b.IFRMinutes
	t.CrossCountryMinutes += b.CrossCountryMinutes
	t.LandingsDay += b.LandingsDay
	t.LandingsNight += b.LandingsNight
}

// ── Aircraft ─────────────────────────────────────────────────────────────

func (h *APIHandler) analyticsByAircraft(ctx context.Context, userID uuid.UUID, months, limit int, byRegistration bool) ([]generated.AnalyticsAircraftRow, error) {
	rows, err := h.reportsRepo.ByAircraft(ctx, userID, months, limit, byRegistration)
	if err != nil {
		return nil, err
	}
	out := []generated.AnalyticsAircraftRow{}
	for _, row := range rows {
		r := generated.AnalyticsAircraftRow{
			Label:        row.Label,
			Flights:      row.Flights,
			TotalMinutes: row.TotalMinutes,
			PicMinutes:   row.PicMinutes,
			DualMinutes:  row.DualMinutes,
			NightMinutes: row.NightMinutes,
			IfrMinutes:   row.IfrMinutes,
			Landings:     row.Landings,
			DistanceNm:   row.DistanceNM,
		}
		if row.SubLabel != nil && *row.SubLabel != row.Label {
			r.SubLabel = strPtr(*row.SubLabel)
		}
		if row.FirstFlightDate != nil {
			r.FirstFlightDate = strPtr(row.FirstFlightDate.Format(isoDate))
		}
		if row.LastFlightDate != nil {
			r.LastFlightDate = strPtr(row.LastFlightDate.Format(isoDate))
		}
		out = append(out, r)
	}
	return out, nil
}

func (h *APIHandler) analyticsGroups(ctx context.Context, userID uuid.UUID, months int, load func(context.Context, uuid.UUID, int) ([]*repository.AnalyticsGroupRow, error)) ([]generated.AnalyticsGroupRow, error) {
	rows, err := load(ctx, userID, months)
	if err != nil {
		return nil, err
	}
	out := []generated.AnalyticsGroupRow{}
	for _, row := range rows {
		out = append(out, generated.AnalyticsGroupRow{
			Label:        row.Label,
			Flights:      row.Flights,
			TotalMinutes: row.TotalMinutes,
			PicMinutes:   row.PicMinutes,
			DualMinutes:  row.DualMinutes,
			Landings:     row.Landings,
		})
	}
	return out, nil
}

// ── Places ───────────────────────────────────────────────────────────────

func (h *APIHandler) analyticsByAirport(ctx context.Context, userID uuid.UUID, months, limit int) ([]generated.AnalyticsAirportRow, error) {
	rows, err := h.reportsRepo.ByAirport(ctx, userID, months, limit)
	if err != nil {
		return nil, err
	}
	out := []generated.AnalyticsAirportRow{}
	for _, row := range rows {
		r := generated.AnalyticsAirportRow{
			Icao:       row.ICAO,
			Departures: row.Departures,
			Arrivals:   row.Arrivals,
			Flights:    row.Flights,
		}
		if ap := airports.Lookup(r.Icao); ap != nil {
			r.Name = strPtr(ap.Name)
			r.Latitude = f64Ptr(ap.Latitude)
			r.Longitude = f64Ptr(ap.Longitude)
			if ap.Country != "" {
				r.Country = strPtr(ap.Country)
			}
		}
		out = append(out, r)
	}
	return out, nil
}

func (h *APIHandler) analyticsByRoute(ctx context.Context, userID uuid.UUID, months, limit int) ([]generated.AnalyticsRouteRow, error) {
	rows, err := h.reportsRepo.ByRoute(ctx, userID, months, limit)
	if err != nil {
		return nil, err
	}
	out := []generated.AnalyticsRouteRow{}
	for _, row := range rows {
		r := generated.AnalyticsRouteRow{
			DepartureIcao: row.DepartureICAO,
			ArrivalIcao:   row.ArrivalICAO,
			Flights:       row.Flights,
			TotalMinutes:  row.TotalMinutes,
			DistanceNm:    row.DistanceNM,
		}
		// Fall back to the airport database when the leg predates distance
		// auto-calculation and has no stored value.
		if r.DistanceNm == 0 && r.DepartureIcao != r.ArrivalIcao {
			dep, arr := airports.Lookup(r.DepartureIcao), airports.Lookup(r.ArrivalIcao)
			if dep != nil && arr != nil {
				r.DistanceNm = airports.DistanceNM(dep.Latitude, dep.Longitude, arr.Latitude, arr.Longitude)
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// countriesFromAirports rolls the resolved airport rows up by country.
// Airports the database does not know are skipped rather than bucketed into a
// synthetic "unknown" country.
func countriesFromAirports(rows []generated.AnalyticsAirportRow) []generated.AnalyticsCountryRow {
	type acc struct {
		airports int
		flights  int
	}
	byCountry := map[string]*acc{}
	for _, r := range rows {
		if r.Country == nil || *r.Country == "" {
			continue
		}
		a, ok := byCountry[*r.Country]
		if !ok {
			a = &acc{}
			byCountry[*r.Country] = a
		}
		a.airports++
		a.flights += r.Flights
	}

	out := make([]generated.AnalyticsCountryRow, 0, len(byCountry))
	for code, a := range byCountry {
		out = append(out, generated.AnalyticsCountryRow{
			Country:  code,
			Airports: a.airports,
			Flights:  a.flights,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Flights != out[j].Flights {
			return out[i].Flights > out[j].Flights
		}
		return out[i].Country < out[j].Country
	})
	return out
}

// ── People ───────────────────────────────────────────────────────────────

func (h *APIHandler) analyticsPeople(ctx context.Context, userID uuid.UUID, months, limit int, load func(context.Context, uuid.UUID, int, int) ([]*repository.AnalyticsPersonRow, error)) ([]generated.AnalyticsPersonRow, error) {
	rows, err := load(ctx, userID, months, limit)
	if err != nil {
		return nil, err
	}
	out := []generated.AnalyticsPersonRow{}
	for _, row := range rows {
		r := generated.AnalyticsPersonRow{
			Name:         row.Name,
			Flights:      row.Flights,
			TotalMinutes: row.TotalMinutes,
		}
		if row.Role != nil && *row.Role != "" {
			r.Role = strPtr(*row.Role)
		}
		if len(row.Roles) > 0 {
			roles := append([]string(nil), row.Roles...)
			r.Roles = &roles
		}
		if row.ContactID != nil {
			if id, err := uuid.Parse(*row.ContactID); err == nil {
				r.ContactId = &id
			}
		}
		if row.LastFlightDate != nil {
			r.LastFlightDate = strPtr(row.LastFlightDate.Format(isoDate))
		}
		out = append(out, r)
	}
	return out, nil
}

// ── Instrument ───────────────────────────────────────────────────────────

func (h *APIHandler) analyticsApproachTypes(ctx context.Context, userID uuid.UUID, months int) ([]generated.AnalyticsApproachTypeRow, error) {
	rows, err := h.reportsRepo.ApproachTypes(ctx, userID, months)
	if err != nil {
		return nil, err
	}
	out := []generated.AnalyticsApproachTypeRow{}
	structured := 0
	for _, row := range rows {
		out = append(out, generated.AnalyticsApproachTypeRow{Type: row.Type, Count: row.Count})
		structured += row.Count
	}

	// Flights logged before structured approaches existed carry only a count.
	// Surface the remainder so the breakdown adds up to totals.approaches.
	counted, err := h.reportsRepo.ApproachesCountTotal(ctx, userID, months)
	if err != nil {
		return nil, err
	}
	if rest := counted - structured; rest > 0 {
		out = append(out, generated.AnalyticsApproachTypeRow{Type: "Unspecified", Count: rest})
	}
	return out, nil
}

// ── Patterns ─────────────────────────────────────────────────────────────

// analyticsPatterns fills the four distribution histograms. Day of week, month
// of year and duration buckets are dense (every bucket present, zeros
// included) so clients can render a fixed axis without gap-filling.
func (h *APIHandler) analyticsPatterns(ctx context.Context, userID uuid.UUID, months int, out *generated.FlightAnalytics) error {
	dow, err := h.reportsRepo.DayOfWeekBuckets(ctx, userID, months)
	if err != nil {
		return err
	}
	out.DayOfWeek = denseBuckets(dow, 1, 7, dayOfWeekLabel)

	moy, err := h.reportsRepo.MonthOfYearBuckets(ctx, userID, months)
	if err != nil {
		return err
	}
	out.MonthOfYear = denseBuckets(moy, 1, 12, monthOfYearLabel)

	hod, err := h.reportsRepo.HourOfDayBuckets(ctx, userID, months)
	if err != nil {
		return err
	}
	out.HourOfDay = denseBuckets(hod, 0, 23, func(k int) string { return fmt.Sprintf("%02d", k) })

	dur, err := h.reportsRepo.DurationBuckets(ctx, userID, months)
	if err != nil {
		return err
	}
	out.DurationBuckets = denseBuckets(dur, 0, 5, durationBucketLabel)
	return nil
}

func denseBuckets(agg map[int]repository.AnalyticsBucket, lo, hi int, label func(int) string) []generated.AnalyticsBucketRow {
	out := make([]generated.AnalyticsBucketRow, 0, hi-lo+1)
	for k := lo; k <= hi; k++ {
		a := agg[k]
		out = append(out, generated.AnalyticsBucketRow{
			Key:          k,
			Label:        label(k),
			Flights:      a.Flights,
			TotalMinutes: a.TotalMinutes,
		})
	}
	return out
}

func dayOfWeekLabel(k int) string {
	names := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	if k >= 1 && k <= 7 {
		return names[k-1]
	}
	return ""
}

func monthOfYearLabel(k int) string {
	if k >= 1 && k <= 12 {
		return time.Month(k).String()[:3]
	}
	return ""
}

func durationBucketLabel(k int) string {
	labels := []string{"<30m", "30-60m", "1-2h", "2-3h", "3-5h", ">5h"}
	if k >= 0 && k < len(labels) {
		return labels[k]
	}
	return ""
}

// ── Records ──────────────────────────────────────────────────────────────

func (h *APIHandler) analyticsRecords(ctx context.Context, userID uuid.UUID, months int, now time.Time) (generated.AnalyticsRecords, error) {
	var r generated.AnalyticsRecords

	longest, err := h.reportsRepo.LongestFlight(ctx, userID, months)
	if err != nil {
		return r, err
	}
	r.LongestFlight = analyticsFlightRef(longest)

	farthest, err := h.reportsRepo.FarthestFlight(ctx, userID, months)
	if err != nil {
		return r, err
	}
	r.LongestDistanceFlight = analyticsFlightRef(farthest)

	busiestDay, busiestDayFlights, err := h.reportsRepo.BusiestDay(ctx, userID, months)
	if err != nil {
		return r, err
	}
	if busiestDay != nil {
		r.BusiestDay = strPtr(busiestDay.Format(isoDate))
		r.BusiestDayFlights = busiestDayFlights
	}

	// Days since last flight is measured against the whole logbook.
	last, err := h.reportsRepo.LastFlightDate(ctx, userID)
	if err != nil {
		return r, err
	}
	if last != nil {
		days := int(now.Truncate(24*time.Hour).Sub(last.UTC().Truncate(24*time.Hour)).Hours() / 24)
		if days < 0 {
			days = 0
		}
		r.DaysSinceLastFlight = &days
	}
	return r, nil
}

func analyticsFlightRef(row *repository.AnalyticsFlightRef) *generated.AnalyticsFlightRef {
	if row == nil {
		return nil
	}
	ref := &generated.AnalyticsFlightRef{
		Id:           openapi_types.UUID(row.ID),
		Date:         row.Date.Format(isoDate),
		TotalMinutes: row.TotalMinutes,
		DistanceNm:   row.DistanceNM,
	}
	if row.AircraftReg != nil {
		ref.AircraftReg = strPtr(*row.AircraftReg)
	}
	if row.AircraftType != nil {
		ref.AircraftType = strPtr(*row.AircraftType)
	}
	if row.DepartureICAO != nil && *row.DepartureICAO != "" {
		ref.DepartureIcao = strPtr(strings.ToUpper(*row.DepartureICAO))
	}
	if row.ArrivalICAO != nil && *row.ArrivalICAO != "" {
		ref.ArrivalIcao = strPtr(strings.ToUpper(*row.ArrivalICAO))
	}
	return ref
}

// applyAirportRecords derives the streak, busiest-period and home-base records
// from sections already loaded, so they stay consistent with what the page
// renders rather than being recomputed from a separate query.
func applyAirportRecords(out *generated.FlightAnalytics) {
	rec := &out.Records

	// Busiest month / year, taken from the series the charts use.
	for _, m := range out.Monthly {
		if rec.BusiestMonthMinutes == nil || m.TotalMinutes > *rec.BusiestMonthMinutes {
			rec.BusiestMonth = strPtr(m.Month)
			rec.BusiestMonthMinutes = intPtr(m.TotalMinutes)
		}
	}
	for _, y := range out.Yearly {
		if rec.BusiestYearMinutes == nil || y.TotalMinutes > *rec.BusiestYearMinutes {
			year := y.Year
			rec.BusiestYear = &year
			rec.BusiestYearMinutes = intPtr(y.TotalMinutes)
		}
	}

	rec.ActiveMonths = len(out.Monthly)
	rec.LongestStreakMonths, rec.CurrentStreakMonths = monthStreaks(out.Monthly, out.Range.To)

	// Home base is the most-visited airport; byAirport is already sorted by
	// flight count, so the farthest airport is measured from there.
	if len(out.ByAirport) == 0 {
		return
	}
	home := out.ByAirport[0]
	rec.HomeBase = strPtr(home.Icao)
	if home.Latitude == nil || home.Longitude == nil {
		return
	}
	var best *generated.AnalyticsAirportRow
	bestNM := 0.0
	for i := range out.ByAirport {
		a := out.ByAirport[i]
		if a.Latitude == nil || a.Longitude == nil {
			continue
		}
		d := airports.DistanceNM(*home.Latitude, *home.Longitude, *a.Latitude, *a.Longitude)
		if d > bestNM {
			bestNM = d
			best = &out.ByAirport[i]
		}
	}
	if best != nil {
		rec.FarthestAirport = best
		rec.FarthestAirportNm = f64Ptr(bestNM)
	}
}

// monthStreaks returns the longest run of consecutive months containing at
// least one flight, and the run ending at the current month. Monthly is
// expected to be sorted oldest first and to contain only months that have
// flights, so a gap in the sequence ends a streak.
func monthStreaks(monthly []generated.AnalyticsMonthPoint, to *string) (longest, current int) {
	if len(monthly) == 0 {
		return 0, 0
	}

	parse := func(s string) (time.Time, bool) {
		t, err := time.Parse(isoMonth, s)
		return t, err == nil
	}

	run := 0
	var prev time.Time
	for _, m := range monthly {
		t, ok := parse(m.Month)
		if !ok {
			continue
		}
		if run > 0 && prev.AddDate(0, 1, 0).Equal(t) {
			run++
		} else {
			run = 1
		}
		if run > longest {
			longest = run
		}
		prev = t
	}

	// The trailing run only counts as "current" when it reaches this month or
	// the one before it — a pilot who last flew in March is not on a streak.
	if to == nil {
		return longest, 0
	}
	now, err := time.Parse(isoDate, *to)
	if err != nil {
		return longest, 0
	}
	thisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	if prev.Equal(thisMonth) || prev.AddDate(0, 1, 0).Equal(thisMonth) {
		current = run
	}
	return longest, current
}

// ── Small helpers ────────────────────────────────────────────────────────

func strPtr(s string) *string   { return &s }
func intPtr(i int) *int         { return &i }
func f64Ptr(f float64) *float64 { return &f }
