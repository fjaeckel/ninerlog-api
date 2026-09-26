package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CustomReport is a user-saved flight filter plus a grouping and a metric.
type CustomReport struct {
	ID         uuid.UUID              `json:"id"`
	UserID     uuid.UUID              `json:"-"`
	Name       string                 `json:"name"`
	Definition CustomReportDefinition `json:"definition"`
	Position   int                    `json:"position"`
	CreatedAt  time.Time              `json:"createdAt"`
	UpdatedAt  time.Time              `json:"updatedAt"`
}

// CustomReportDefinition is the stored report document.
type CustomReportDefinition struct {
	Filter  CustomReportFilter `json:"filter"`
	Window  CustomReportWindow `json:"window"`
	GroupBy string             `json:"groupBy"`
	Metric  string             `json:"metric"`
	Limit   *int               `json:"limit,omitempty"`
}

// CustomReportFilter mirrors the GET /flights filter parameters.
type CustomReportFilter struct {
	Q                *string    `json:"q,omitempty"`
	AircraftReg      *string    `json:"aircraftReg,omitempty"`
	DepartureICAO    *string    `json:"departureIcao,omitempty"`
	ArrivalICAO      *string    `json:"arrivalIcao,omitempty"`
	Role             *string    `json:"role,omitempty"`
	LogbookLicenseID *uuid.UUID `json:"logbookLicenseId,omitempty"`
}

// CustomReportWindow is the report's date window.
type CustomReportWindow struct {
	Kind      string  `json:"kind"`
	Months    *int    `json:"months,omitempty"`
	StartDate *string `json:"startDate,omitempty"`
	EndDate   *string `json:"endDate,omitempty"`
}

// Custom report window kinds.
const (
	ReportWindowAll        = "all"
	ReportWindowLastMonths = "lastMonths"
	ReportWindowYearToDate = "yearToDate"
	ReportWindowRange      = "range"
)

// Custom report groupings.
const (
	ReportGroupMonth         = "month"
	ReportGroupYear          = "year"
	ReportGroupDayOfWeek     = "dayOfWeek"
	ReportGroupAircraftType  = "aircraftType"
	ReportGroupRegistration  = "registration"
	ReportGroupDeparture     = "departure"
	ReportGroupArrival       = "arrival"
	ReportGroupRoute         = "route"
	ReportGroupLaunchMethod  = "launchMethod"
	ReportGroupAircraftClass = "aircraftClass"
	ReportGroupULKind        = "ulKind"
)

// ReportGroupings lists every valid groupBy value.
var ReportGroupings = []string{
	ReportGroupMonth, ReportGroupYear, ReportGroupDayOfWeek, ReportGroupAircraftType,
	ReportGroupRegistration, ReportGroupDeparture, ReportGroupArrival, ReportGroupRoute,
	ReportGroupLaunchMethod, ReportGroupAircraftClass, ReportGroupULKind,
}

// ReportMetrics lists every valid metric value, in table column order.
var ReportMetrics = []string{
	"flights", "totalTime", "picTime", "dualTime", "dualGivenTime",
	"nightTime", "ifrTime", "crossCountryTime", "fstdTime", "landings",
	"launches", "outlandings", "towFlights",
}

// ReportSoaringMetrics lists the metrics that are soaring or towing counts.
var ReportSoaringMetrics = []string{"launches", "outlandings", "towFlights"}

// IsSoaringMetric reports whether metric is one of ReportSoaringMetrics.
func IsSoaringMetric(metric string) bool {
	return contains(ReportSoaringMetrics, metric)
}

// IsTimeGrouping reports whether groupBy is chronological rather than ranked.
func IsTimeGrouping(groupBy string) bool {
	return groupBy == ReportGroupMonth || groupBy == ReportGroupYear || groupBy == ReportGroupDayOfWeek
}

// IsDurationMetric reports whether metric is measured in minutes.
func IsDurationMetric(metric string) bool {
	return metric != "flights" && metric != "landings" && !IsSoaringMetric(metric)
}

// Custom report limits.
const (
	ReportDefaultLimit = 20
	ReportMaxLimit     = 100
	ReportMaxMonths    = 120
	ReportMaxQueryLen  = 1000
)

const reportDateLayout = "2006-01-02"

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func parseReportDate(field string, v *string) (*time.Time, error) {
	if v == nil || *v == "" {
		return nil, nil
	}
	t, err := time.Parse(reportDateLayout, *v)
	if err != nil {
		return nil, fmt.Errorf("%s must be a date (YYYY-MM-DD)", field)
	}
	if t.Year() < 1900 || t.Year() > 2200 {
		return nil, fmt.Errorf("%s is out of range", field)
	}
	return &t, nil
}

// RangeDates parses a range window's bounds; either may be nil.
func (w CustomReportWindow) RangeDates() (start, end *time.Time, err error) {
	if start, err = parseReportDate("startDate", w.StartDate); err != nil {
		return nil, nil, err
	}
	if end, err = parseReportDate("endDate", w.EndDate); err != nil {
		return nil, nil, err
	}
	return start, end, nil
}

// Normalize trims text filters and drops empty ones.
func (d *CustomReportDefinition) Normalize() {
	for _, p := range []**string{&d.Filter.Q, &d.Filter.AircraftReg, &d.Filter.DepartureICAO, &d.Filter.ArrivalICAO, &d.Filter.Role} {
		if *p == nil {
			continue
		}
		v := strings.TrimSpace(**p)
		if v == "" {
			*p = nil
			continue
		}
		*p = &v
	}
	if d.Window.Kind != ReportWindowLastMonths {
		d.Window.Months = nil
	}
	if d.Window.Kind != ReportWindowRange {
		d.Window.StartDate, d.Window.EndDate = nil, nil
	}
	if IsTimeGrouping(d.GroupBy) {
		d.Limit = nil
	}
}

// Validate checks the controlled vocabulary and bounds of a definition. The
// search query itself is validated by the caller.
func (d *CustomReportDefinition) Validate() error {
	if !contains(ReportGroupings, d.GroupBy) {
		return fmt.Errorf("invalid groupBy %q", d.GroupBy)
	}
	if !contains(ReportMetrics, d.Metric) {
		return fmt.Errorf("invalid metric %q", d.Metric)
	}
	if d.Limit != nil && (*d.Limit < 1 || *d.Limit > ReportMaxLimit) {
		return fmt.Errorf("limit must be between 1 and %d", ReportMaxLimit)
	}
	switch d.Window.Kind {
	case ReportWindowAll, ReportWindowYearToDate:
	case ReportWindowLastMonths:
		if d.Window.Months == nil || *d.Window.Months < 1 || *d.Window.Months > ReportMaxMonths {
			return fmt.Errorf("window months must be between 1 and %d", ReportMaxMonths)
		}
	case ReportWindowRange:
		start, end, err := d.Window.RangeDates()
		if err != nil {
			return err
		}
		if start != nil && end != nil && end.Before(*start) {
			return fmt.Errorf("endDate must not be before startDate")
		}
	default:
		return fmt.Errorf("invalid window kind %q", d.Window.Kind)
	}
	f := d.Filter
	if f.Q != nil && len(*f.Q) > ReportMaxQueryLen {
		return fmt.Errorf("search query is too long")
	}
	for _, t := range []struct {
		name string
		v    *string
		max  int
	}{{"aircraftReg", f.AircraftReg, 20}, {"departureIcao", f.DepartureICAO, 10}, {"arrivalIcao", f.ArrivalICAO, 10}} {
		if t.v == nil {
			continue
		}
		if len(*t.v) > t.max {
			return fmt.Errorf("%s is too long", t.name)
		}
		if ContainsControlChar(*t.v) {
			return fmt.Errorf("%s contains invalid characters", t.name)
		}
	}
	if f.Role != nil && *f.Role != "pic" && *f.Role != "dual" {
		return fmt.Errorf("invalid role %q", *f.Role)
	}
	return nil
}

// Value implements driver.Valuer for JSONB columns.
func (d CustomReportDefinition) Value() (driver.Value, error) {
	data, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// Scan implements sql.Scanner for JSONB columns.
func (d *CustomReportDefinition) Scan(src interface{}) error {
	switch v := src.(type) {
	case []byte:
		return json.Unmarshal(v, d)
	case string:
		return json.Unmarshal([]byte(v), d)
	case nil:
		return nil
	default:
		return fmt.Errorf("cannot scan %T into CustomReportDefinition", src)
	}
}

// CustomReportTotals aggregates one group, or every matching flight.
// Durations are minutes.
type CustomReportTotals struct {
	Flights          int `json:"flights"`
	TotalTime        int `json:"totalTime"`
	PicTime          int `json:"picTime"`
	DualTime         int `json:"dualTime"`
	DualGivenTime    int `json:"dualGivenTime"`
	NightTime        int `json:"nightTime"`
	IfrTime          int `json:"ifrTime"`
	CrossCountryTime int `json:"crossCountryTime"`
	FstdTime         int `json:"fstdTime"`
	Landings         int `json:"landings"`
	Launches         int `json:"launches"`
	Outlandings      int `json:"outlandings"`
	TowFlights       int `json:"towFlights"`
}

// Metric returns the named metric; unknown names return 0.
func (t CustomReportTotals) Metric(name string) int {
	switch name {
	case "flights":
		return t.Flights
	case "totalTime":
		return t.TotalTime
	case "picTime":
		return t.PicTime
	case "dualTime":
		return t.DualTime
	case "dualGivenTime":
		return t.DualGivenTime
	case "nightTime":
		return t.NightTime
	case "ifrTime":
		return t.IfrTime
	case "crossCountryTime":
		return t.CrossCountryTime
	case "fstdTime":
		return t.FstdTime
	case "landings":
		return t.Landings
	case "launches":
		return t.Launches
	case "outlandings":
		return t.Outlandings
	case "towFlights":
		return t.TowFlights
	}
	return 0
}

// Add accumulates o into t.
func (t *CustomReportTotals) Add(o CustomReportTotals) {
	t.Flights += o.Flights
	t.TotalTime += o.TotalTime
	t.PicTime += o.PicTime
	t.DualTime += o.DualTime
	t.DualGivenTime += o.DualGivenTime
	t.NightTime += o.NightTime
	t.IfrTime += o.IfrTime
	t.CrossCountryTime += o.CrossCountryTime
	t.FstdTime += o.FstdTime
	t.Landings += o.Landings
	t.Launches += o.Launches
	t.Outlandings += o.Outlandings
	t.TowFlights += o.TowFlights
}

// CustomReportRow is one group of a report result.
type CustomReportRow struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value int    `json:"value"`
	CustomReportTotals
}

// CustomReportResult is an evaluated report.
type CustomReportResult struct {
	GroupBy     string             `json:"groupBy"`
	Metric      string             `json:"metric"`
	StartDate   *string            `json:"startDate,omitempty"`
	EndDate     *string            `json:"endDate,omitempty"`
	Rows        []CustomReportRow  `json:"rows"`
	Totals      CustomReportTotals `json:"totals"`
	OtherGroups int                `json:"otherGroups"`
	GeneratedAt time.Time          `json:"generatedAt"`
}
