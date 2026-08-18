package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// This file defines the read-side aggregates behind the reports and analytics
// endpoints. All methods are scoped to one user. Where a method takes a
// `months` parameter, a positive value restricts the aggregate to flights
// dated within the trailing N calendar months (month-truncated, matching
// date_trunc('month', CURRENT_DATE - N months)); zero or negative covers all
// time. Durations are integer minutes throughout, per the domain rule.

// RouteCount is one departure→arrival pair with its flight count (map view).
type RouteCount struct {
	DepartureICAO string
	ArrivalICAO   string
	FlightCount   int
}

// AirportDirectionCount is a per-airport leg count in one direction
// ("dep" or "arr").
type AirportDirectionCount struct {
	ICAO      string
	Direction string
	Count     int
}

// MonthlyTrendRow is one month of the legacy trends report.
type MonthlyTrendRow struct {
	Month         string // "YYYY-MM"
	TotalFlights  int
	TotalMinutes  int
	PICMinutes    int
	DualMinutes   int
	NightMinutes  int
	IFRMinutes    int
	LandingsDay   int
	LandingsNight int
}

// AircraftTypeTrendRow is the per-type rollup of the legacy trends report.
type AircraftTypeTrendRow struct {
	AircraftType string
	TotalFlights int
	TotalMinutes int
}

// ClassStatRow is a per-aircraft-class rollup ("Unclassified" when the flight
// has no matching fleet entry).
type ClassStatRow struct {
	Class       string
	Flights     int
	Minutes     int
	PICMinutes  int
	DualMinutes int
	Landings    int
}

// CategoryStatRow is a per-capability rollup (Tailwheel / Complex / High
// Performance). Rows overlap by design: one flight can count toward several.
type CategoryStatRow struct {
	Category    string
	Flights     int
	PICMinutes  int
	DualMinutes int
}

// AnalyticsTotals is the whole-timeframe aggregate for the analytics report.
type AnalyticsTotals struct {
	TotalFlights               int
	TotalMinutes               int
	PicMinutes                 int
	SicMinutes                 int
	DualMinutes                int
	DualGivenMinutes           int
	SoloMinutes                int
	NightMinutes               int
	IfrMinutes                 int
	ActualInstrumentMinutes    int
	SimulatedInstrumentMinutes int
	CrossCountryMinutes        int
	MultiPilotMinutes          int
	SimulatedFlightMinutes     int
	GroundTrainingMinutes      int
	LandingsDay                int
	LandingsNight              int
	TakeoffsDay                int
	TakeoffsNight              int
	Approaches                 int
	Holds                      int
	DistanceNM                 float64
	DistinctRegistrations      int
	DistinctTypes              int
	FirstFlightDate            *time.Time
	LastFlightDate             *time.Time
}

// AnalyticsMonthRow is one month of the analytics time series.
type AnalyticsMonthRow struct {
	Month            string // "YYYY-MM"
	Flights          int
	TotalMinutes     int
	PicMinutes       int
	SicMinutes       int
	DualMinutes      int
	DualGivenMinutes int
	SoloMinutes      int
	NightMinutes     int
	IfrMinutes       int
	LandingsDay      int
	LandingsNight    int
	DistanceNM       float64
}

// AnalyticsYearRow is one calendar year of the analytics time series.
type AnalyticsYearRow struct {
	Year         int
	Flights      int
	TotalMinutes int
	PicMinutes   int
	DualMinutes  int
	NightMinutes int
	IfrMinutes   int
	Landings     int
	DistanceNM   float64
}

// AnalyticsAircraftRow is a per-aircraft-type or per-registration rollup.
type AnalyticsAircraftRow struct {
	Label           string
	SubLabel        *string
	Flights         int
	TotalMinutes    int
	PicMinutes      int
	DualMinutes     int
	NightMinutes    int
	IfrMinutes      int
	Landings        int
	DistanceNM      float64
	FirstFlightDate *time.Time
	LastFlightDate  *time.Time
}

// AnalyticsGroupRow is a generic labelled rollup (class, category).
type AnalyticsGroupRow struct {
	Label        string
	Flights      int
	TotalMinutes int
	PicMinutes   int
	DualMinutes  int
	Landings     int
}

// AnalyticsAirportRow counts one airport's legs. Flights counts each flight
// once even when it both departs from and arrives at the airport.
type AnalyticsAirportRow struct {
	ICAO       string
	Departures int
	Arrivals   int
	Flights    int
}

// AnalyticsRouteRow is a per-route rollup. DistanceNM is the stored per-leg
// maximum; zero when no flight on the route recorded a distance.
type AnalyticsRouteRow struct {
	DepartureICAO string
	ArrivalICAO   string
	Flights       int
	TotalMinutes  int
	DistanceNM    float64
}

// AnalyticsPersonRow is a per-person rollup (instructor or crew member).
// Role is set only for crew rows; TotalMinutes is dual time for instructors
// and block time for crew, mirroring what each view displays.
type AnalyticsPersonRow struct {
	Name           string
	Role           *string
	Flights        int
	TotalMinutes   int
	LastFlightDate *time.Time
}

// AnalyticsApproachTypeRow counts structured approaches by type.
type AnalyticsApproachTypeRow struct {
	Type  string
	Count int
}

// AnalyticsBucket is one histogram cell keyed by an integer (ISO weekday,
// month number, hour, or duration bucket index).
type AnalyticsBucket struct {
	Flights      int
	TotalMinutes int
}

// AnalyticsFlightRef identifies a single record-holding flight.
type AnalyticsFlightRef struct {
	ID            uuid.UUID
	Date          time.Time
	AircraftReg   *string
	AircraftType  *string
	DepartureICAO *string
	ArrivalICAO   *string
	TotalMinutes  int
	DistanceNM    float64
}

// ReportsRepository serves the aggregate queries behind the reports, analytics
// and map endpoints. It is read-only.
type ReportsRepository interface {
	// RouteCounts returns every flown departure→arrival pair with counts,
	// most-flown first, across all time.
	RouteCounts(ctx context.Context, userID uuid.UUID) ([]*RouteCount, error)

	// AirportCounts returns per-airport leg counts split by direction,
	// across all time.
	AirportCounts(ctx context.Context, userID uuid.UUID) ([]*AirportDirectionCount, error)

	// MonthlyTrends returns the legacy per-month trend rows, oldest first.
	MonthlyTrends(ctx context.Context, userID uuid.UUID, months int) ([]*MonthlyTrendRow, error)

	// AircraftTypeTrends returns the legacy per-type rollup, most minutes first.
	AircraftTypeTrends(ctx context.Context, userID uuid.UUID, months int) ([]*AircraftTypeTrendRow, error)

	// StatsByClass returns per-class rollups, most minutes first.
	StatsByClass(ctx context.Context, userID uuid.UUID, months int) ([]*ClassStatRow, error)

	// StatsByCategory returns the overlapping capability rollups.
	StatsByCategory(ctx context.Context, userID uuid.UUID, months int) ([]*CategoryStatRow, error)

	// Totals returns the timeframe-wide aggregate.
	Totals(ctx context.Context, userID uuid.UUID, months int) (*AnalyticsTotals, error)

	// Monthly returns the analytics month series, oldest first.
	Monthly(ctx context.Context, userID uuid.UUID, months int) ([]*AnalyticsMonthRow, error)

	// Yearly returns the analytics year series, oldest first.
	Yearly(ctx context.Context, userID uuid.UUID, months int) ([]*AnalyticsYearRow, error)

	// ByAircraft groups by aircraft type, or by registration when
	// byRegistration is true, top `limit` rows by block time.
	ByAircraft(ctx context.Context, userID uuid.UUID, months, limit int, byRegistration bool) ([]*AnalyticsAircraftRow, error)

	// ByClass groups by aircraft class.
	ByClass(ctx context.Context, userID uuid.UUID, months int) ([]*AnalyticsGroupRow, error)

	// ByCategory returns the overlapping capability rollups.
	ByCategory(ctx context.Context, userID uuid.UUID, months int) ([]*AnalyticsGroupRow, error)

	// ByAirport returns the top `limit` airports by distinct flights.
	ByAirport(ctx context.Context, userID uuid.UUID, months, limit int) ([]*AnalyticsAirportRow, error)

	// ByRoute returns the top `limit` routes by flight count.
	ByRoute(ctx context.Context, userID uuid.UUID, months, limit int) ([]*AnalyticsRouteRow, error)

	// ByInstructor returns the top `limit` instructors by dual time received.
	ByInstructor(ctx context.Context, userID uuid.UUID, months, limit int) ([]*AnalyticsPersonRow, error)

	// ByCrew returns the top `limit` (crew member, role) pairs by block time.
	ByCrew(ctx context.Context, userID uuid.UUID, months, limit int) ([]*AnalyticsPersonRow, error)

	// ApproachTypes counts structured approach entries by type, most first.
	ApproachTypes(ctx context.Context, userID uuid.UUID, months int) ([]*AnalyticsApproachTypeRow, error)

	// ApproachesCountTotal sums the flights' approaches_count column, which
	// also covers legacy flights that carry only a count.
	ApproachesCountTotal(ctx context.Context, userID uuid.UUID, months int) (int, error)

	// DayOfWeekBuckets keys by ISO weekday (1=Mon..7=Sun).
	DayOfWeekBuckets(ctx context.Context, userID uuid.UUID, months int) (map[int]AnalyticsBucket, error)

	// MonthOfYearBuckets keys by month number (1..12).
	MonthOfYearBuckets(ctx context.Context, userID uuid.UUID, months int) (map[int]AnalyticsBucket, error)

	// HourOfDayBuckets keys by the hour of the off-block (or departure) time
	// (0..23); flights with neither timestamp are excluded.
	HourOfDayBuckets(ctx context.Context, userID uuid.UUID, months int) (map[int]AnalyticsBucket, error)

	// DurationBuckets keys by duration bucket index
	// (0: <30m, 1: 30-60m, 2: 1-2h, 3: 2-3h, 4: 3-5h, 5: >5h).
	DurationBuckets(ctx context.Context, userID uuid.UUID, months int) (map[int]AnalyticsBucket, error)

	// LongestFlight returns the flight with the most block time in the
	// timeframe, or nil when there are none.
	LongestFlight(ctx context.Context, userID uuid.UUID, months int) (*AnalyticsFlightRef, error)

	// FarthestFlight returns the flight with the greatest recorded distance,
	// or nil when there are none.
	FarthestFlight(ctx context.Context, userID uuid.UUID, months int) (*AnalyticsFlightRef, error)

	// BusiestDay returns the date with the most flights and its count, or
	// (nil, 0) when the timeframe holds no flights.
	BusiestDay(ctx context.Context, userID uuid.UUID, months int) (*time.Time, int, error)

	// LastFlightDate returns the date of the most recent flight across the
	// whole logbook (deliberately ignoring the timeframe), or nil.
	LastFlightDate(ctx context.Context, userID uuid.UUID) (*time.Time, error)

	// CarriedForwardMinutes sums block time of flights dated before the
	// timeframe's first month. Zero when months <= 0 (nothing precedes an
	// all-time window).
	CarriedForwardMinutes(ctx context.Context, userID uuid.UUID, months int) (int, error)
}
