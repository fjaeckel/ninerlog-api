// Package customreport saves, evaluates and orders user-defined flight reports.
package customreport

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fjaeckel/ninerlog-api/internal/flightsearch"
	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/pkg/registration"
	"github.com/google/uuid"
)

// ValidationError is a user-fixable problem with a report; handlers map it to 400.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func invalid(format string, a ...any) *ValidationError {
	return &ValidationError{Msg: fmt.Sprintf(format, a...)}
}

// IsValidationError reports whether err is (or wraps) a ValidationError.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

// Limits on report metadata.
const (
	MaxNameLen        = 120
	MaxReportsPerUser = 100
	// maxTimeBuckets caps gap-filled month or year rows.
	maxTimeBuckets = 1200
)

// LogbookScoper restricts a flight query to a licence's logbook.
type LogbookScoper interface {
	Apply(ctx context.Context, userID, licenseID uuid.UUID, opts *repository.FlightQueryOptions) error
}

// Service manages custom reports.
type Service struct {
	repo  repository.CustomReportRepository
	scope LogbookScoper
	now   func() time.Time
}

// NewService creates a custom report service.
func NewService(repo repository.CustomReportRepository, scope LogbookScoper) *Service {
	return &Service{repo: repo, scope: scope, now: time.Now}
}

// Input carries the writable fields of a report.
type Input struct {
	Name       string
	Definition models.CustomReportDefinition
}

// List returns the user's reports in display order.
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]*models.CustomReport, error) {
	return s.repo.ListByUserID(ctx, userID)
}

// Count returns how many reports the user holds.
func (s *Service) Count(ctx context.Context, userID uuid.UUID) (int, error) {
	return s.repo.CountByUserID(ctx, userID)
}

// Get returns one of the user's reports, or repository.ErrNotFound.
func (s *Service) Get(ctx context.Context, userID, id uuid.UUID) (*models.CustomReport, error) {
	rep, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if rep.UserID != userID {
		return nil, repository.ErrNotFound
	}
	return rep, nil
}

// Create validates and stores a report after the user's existing ones.
func (s *Service) Create(ctx context.Context, userID uuid.UUID, in Input) (*models.CustomReport, error) {
	if err := s.validateInput(ctx, userID, &in); err != nil {
		return nil, err
	}
	n, err := s.repo.CountByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if n >= MaxReportsPerUser {
		return nil, invalid("an account can hold at most %d custom reports", MaxReportsPerUser)
	}
	rep := &models.CustomReport{UserID: userID, Name: in.Name, Definition: in.Definition}
	if err := s.repo.Create(ctx, rep); err != nil {
		return nil, err
	}
	return rep, nil
}

// Update replaces a report's name and definition.
func (s *Service) Update(ctx context.Context, userID, id uuid.UUID, in Input) (*models.CustomReport, error) {
	rep, err := s.Get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if err := s.validateInput(ctx, userID, &in); err != nil {
		return nil, err
	}
	rep.Name = in.Name
	rep.Definition = in.Definition
	if err := s.repo.Update(ctx, rep); err != nil {
		return nil, err
	}
	return rep, nil
}

// Delete removes one of the user's reports.
func (s *Service) Delete(ctx context.Context, userID, id uuid.UUID) error {
	if _, err := s.Get(ctx, userID, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

// Reorder sets the display order; ids must list each of the user's reports once.
func (s *Service) Reorder(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) ([]*models.CustomReport, error) {
	existing, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	owned := make(map[uuid.UUID]bool, len(existing))
	for _, r := range existing {
		owned[r.ID] = true
	}
	if len(ids) != len(existing) {
		return nil, invalid("reportIds must list every custom report exactly once")
	}
	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if !owned[id] || seen[id] {
			return nil, invalid("reportIds must list every custom report exactly once")
		}
		seen[id] = true
	}
	if err := s.repo.SetPositions(ctx, userID, ids); err != nil {
		return nil, err
	}
	return s.repo.ListByUserID(ctx, userID)
}

// Result evaluates a saved report.
func (s *Service) Result(ctx context.Context, userID, id uuid.UUID) (*models.CustomReport, *models.CustomReportResult, error) {
	rep, err := s.Get(ctx, userID, id)
	if err != nil {
		return nil, nil, err
	}
	res, err := s.Evaluate(ctx, userID, rep.Definition)
	if err != nil {
		return nil, nil, err
	}
	return rep, res, nil
}

// Evaluate runs a definition against the user's flights without storing it.
func (s *Service) Evaluate(ctx context.Context, userID uuid.UUID, def models.CustomReportDefinition) (*models.CustomReportResult, error) {
	def.Normalize()
	if err := def.Validate(); err != nil {
		return nil, invalid("%s", err.Error())
	}
	now := s.now().UTC()
	start, end, err := resolveWindow(def.Window, now)
	if err != nil {
		return nil, invalid("%s", err.Error())
	}
	opts, err := s.queryOptions(ctx, userID, def, start, end)
	if err != nil {
		return nil, err
	}
	groups, err := s.repo.Aggregate(ctx, userID, opts, def.GroupBy)
	if err != nil {
		return nil, err
	}
	return buildResult(def, groups, start, end, now), nil
}

func (s *Service) validateInput(ctx context.Context, userID uuid.UUID, in *Input) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return invalid("a report needs a name")
	}
	if utf8.RuneCountInString(in.Name) > MaxNameLen {
		return invalid("name is too long")
	}
	if models.ContainsControlChar(in.Name) {
		return invalid("name contains invalid characters")
	}
	in.Definition.Normalize()
	if err := in.Definition.Validate(); err != nil {
		return invalid("%s", err.Error())
	}
	if _, err := s.queryOptions(ctx, userID, in.Definition, nil, nil); err != nil {
		return err
	}
	return nil
}

// queryOptions translates a validated definition into a flight query.
func (s *Service) queryOptions(ctx context.Context, userID uuid.UUID, def models.CustomReportDefinition, start, end *time.Time) (*repository.FlightQueryOptions, error) {
	f := def.Filter
	opts := &repository.FlightQueryOptions{StartDate: start, EndDate: end}
	if f.Q != nil {
		q, err := flightsearch.Parse(*f.Q)
		if err != nil {
			return nil, invalid("invalid search query: %s", err.Error())
		}
		opts.Query = q
	}
	if f.AircraftReg != nil {
		reg := registration.Canonical(*f.AircraftReg)
		opts.AircraftReg = &reg
	}
	opts.DepartureICAO = f.DepartureICAO
	opts.ArrivalICAO = f.ArrivalICAO
	if f.Role != nil {
		t := true
		if *f.Role == "pic" {
			opts.IsPIC = &t
		} else {
			opts.IsDual = &t
		}
	}
	if f.LogbookLicenseID != nil {
		if err := s.scope.Apply(ctx, userID, *f.LogbookLicenseID, opts); err != nil {
			if errors.Is(err, service.ErrLicenseNotFound) || errors.Is(err, service.ErrUnauthorizedAccess) {
				return nil, invalid("the logbook licence of this report does not exist")
			}
			return nil, err
		}
	}
	return opts, nil
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// resolveWindow returns the inclusive date bounds of a window at now.
func resolveWindow(w models.CustomReportWindow, now time.Time) (start, end *time.Time, err error) {
	today := dateOnly(now)
	switch w.Kind {
	case models.ReportWindowLastMonths:
		s := time.Date(today.Year(), today.Month()-time.Month(*w.Months-1), 1, 0, 0, 0, 0, time.UTC)
		return &s, &today, nil
	case models.ReportWindowYearToDate:
		s := time.Date(today.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		return &s, &today, nil
	case models.ReportWindowRange:
		return w.RangeDates()
	}
	return nil, nil, nil
}

var weekdays = []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

var launchMethodLabels = map[string]string{
	models.LaunchMethodWinch:      "Winch",
	models.LaunchMethodAerotow:    "Aerotow",
	models.LaunchMethodSelfLaunch: "Self-launch",
	models.LaunchMethodCar:        "Car tow",
	models.LaunchMethodBungee:     "Bungee",
}

var ulKindLabels = map[string]string{
	string(models.ULKindThreeAxis):            "Three-axis",
	string(models.ULKindThreeAxisMotorglider): "Three-axis motor glider",
	string(models.ULKindWeightShift):          "Weight-shift",
	string(models.ULKindGyroplane):            "Gyroplane",
	string(models.ULKindHelicopter):           "Helicopter",
	string(models.ULKindPoweredParaglider):    "Powered paraglider",
	string(models.ULKindSailplane):            "Sailplane",
}

// Label returns the English display label of a group key.
func Label(groupBy, key string) string {
	if key == "" {
		return "(none)"
	}
	switch groupBy {
	case models.ReportGroupMonth:
		if t, err := time.Parse("2006-01", key); err == nil {
			return t.Format("Jan 2006")
		}
	case models.ReportGroupDayOfWeek:
		var d int
		if _, err := fmt.Sscanf(key, "%d", &d); err == nil && d >= 1 && d <= 7 {
			return weekdays[d-1]
		}
	case models.ReportGroupLaunchMethod:
		if l, ok := launchMethodLabels[key]; ok {
			return l
		}
	case models.ReportGroupULKind:
		if l, ok := ulKindLabels[key]; ok {
			return l
		}
	}
	return key
}

func buildResult(def models.CustomReportDefinition, groups []repository.CustomReportGroup, start, end *time.Time, now time.Time) *models.CustomReportResult {
	res := &models.CustomReportResult{
		GroupBy:     def.GroupBy,
		Metric:      def.Metric,
		Rows:        []models.CustomReportRow{},
		GeneratedAt: now,
	}
	if start != nil {
		v := start.Format("2006-01-02")
		res.StartDate = &v
	}
	if end != nil {
		v := end.Format("2006-01-02")
		res.EndDate = &v
	}

	byKey := make(map[string]models.CustomReportTotals, len(groups))
	for _, g := range groups {
		byKey[g.Key] = g.Totals
		res.Totals.Add(g.Totals)
	}

	var keys []string
	if models.IsTimeGrouping(def.GroupBy) {
		keys = timeKeys(def.GroupBy, groups, start, end, now)
	} else {
		keys = make([]string, 0, len(groups))
		for _, g := range groups {
			keys = append(keys, g.Key)
		}
		sort.SliceStable(keys, func(i, j int) bool {
			vi, vj := byKey[keys[i]].Metric(def.Metric), byKey[keys[j]].Metric(def.Metric)
			if vi != vj {
				return vi > vj
			}
			return keys[i] < keys[j]
		})
		limit := models.ReportDefaultLimit
		if def.Limit != nil {
			limit = *def.Limit
		}
		if len(keys) > limit {
			res.OtherGroups = len(keys) - limit
			keys = keys[:limit]
		}
	}

	for _, k := range keys {
		t := byKey[k]
		res.Rows = append(res.Rows, models.CustomReportRow{
			Key: k, Label: Label(def.GroupBy, k), Value: t.Metric(def.Metric), CustomReportTotals: t,
		})
	}
	return res
}

// timeKeys returns chronological, gap-filled keys for a time grouping.
func timeKeys(groupBy string, groups []repository.CustomReportGroup, start, end *time.Time, now time.Time) []string {
	if groupBy == models.ReportGroupDayOfWeek {
		return []string{"1", "2", "3", "4", "5", "6", "7"}
	}
	layout, step := "2006-01", func(t time.Time) time.Time { return t.AddDate(0, 1, 0) }
	if groupBy == models.ReportGroupYear {
		layout, step = "2006", func(t time.Time) time.Time { return t.AddDate(1, 0, 0) }
	}
	truncate := func(t time.Time) time.Time {
		if groupBy == models.ReportGroupYear {
			return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		}
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	}

	var lo, hi *time.Time
	for _, g := range groups {
		t, err := time.Parse(layout, g.Key)
		if err != nil {
			continue
		}
		if lo == nil || t.Before(*lo) {
			lo = &t
		}
		if hi == nil || t.After(*hi) {
			hi = &t
		}
	}
	if start != nil {
		t := truncate(*start)
		lo = &t
	}
	if end != nil {
		t := truncate(*end)
		// A future end is capped at the later of the last flight and today.
		limit := truncate(now)
		if hi != nil && hi.After(limit) {
			limit = *hi
		}
		if t.After(limit) {
			t = limit
		}
		hi = &t
	}
	if lo == nil || hi == nil || lo.After(*hi) {
		return []string{}
	}

	keys := []string{}
	for t := *lo; !t.After(*hi); t = step(t) {
		keys = append(keys, t.Format(layout))
	}
	if len(keys) > maxTimeBuckets {
		keys = keys[len(keys)-maxTimeBuckets:]
	}
	return keys
}
