// Package readiness answers "may I fly on this date?" from the currency
// engine evaluated as of that date, per rating, launch method, passengers and
// medical certificate.
package readiness

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/fjaeckel/ninerlog-api/pkg/registration"
	"github.com/google/uuid"
)

// MaxDaysAhead is how far ahead a readiness date may lie.
const MaxDaysAhead = 366

var (
	// ErrInvalidDate is returned for a date before today or more than
	// MaxDaysAhead days ahead.
	ErrInvalidDate = errors.New("readiness date out of range")
	// ErrAircraftNotFound is returned when aircraftReg names none of the
	// caller's aircraft.
	ErrAircraftNotFound = errors.New("aircraft not found")
)

// Kind is what a readiness item answers for.
type Kind string

const (
	KindRating       Kind = "rating"
	KindLaunchMethod Kind = "launch_method"
	KindPassengers   Kind = "passengers"
	KindCredential   Kind = "credential"
)

// Credential statuses.
const (
	StatusValid   = "valid"
	StatusExpired = "expired"
)

// Item is one readiness answer.
type Item struct {
	Kind          Kind                    `json:"kind"`
	ClassRatingID *uuid.UUID              `json:"classRatingId,omitempty"`
	LicenseID     *uuid.UUID              `json:"licenseId,omitempty"`
	ClassType     *models.ClassType       `json:"classType,omitempty"`
	ULKind        *models.ULKind          `json:"ulKind,omitempty"`
	LaunchMethod  *string                 `json:"launchMethod,omitempty"`
	CredentialID  *uuid.UUID              `json:"credentialId,omitempty"`
	Ready         bool                    `json:"ready"`
	Status        string                  `json:"status"`
	ReasonKey     string                  `json:"reasonKey"`
	Params        *currency.MessageParams `json:"params,omitempty"`
}

// Report is the readiness answer for one date.
type Report struct {
	Date        string  `json:"date"`
	AircraftReg *string `json:"aircraftReg,omitempty"`
	Items       []Item  `json:"items"`
}

// Request selects what a report answers. A nil Date means today; an empty
// AircraftReg means every rating.
type Request struct {
	Date        *time.Time
	AircraftReg string
	Passengers  bool
}

// CurrencyEvaluator evaluates currency as of a date.
type CurrencyEvaluator interface {
	EvaluateAsOf(ctx context.Context, userID uuid.UUID, date time.Time) (*currency.CurrencyStatusResponse, error)
}

// AircraftLister lists a user's aircraft.
type AircraftLister interface {
	ListAircraft(ctx context.Context, userID uuid.UUID) ([]*models.Aircraft, error)
}

// CredentialLister lists a user's credentials.
type CredentialLister interface {
	ListCredentials(ctx context.Context, userID uuid.UUID) ([]*models.Credential, error)
}

// Service builds readiness reports.
type Service struct {
	currency    CurrencyEvaluator
	aircraft    AircraftLister
	credentials CredentialLister
	now         func() time.Time
}

// NewService creates a readiness service.
func NewService(cur CurrencyEvaluator, aircraft AircraftLister, credentials CredentialLister) *Service {
	return &Service{currency: cur, aircraft: aircraft, credentials: credentials, now: time.Now}
}

// Evaluate returns the caller's readiness on req.Date. Returns ErrInvalidDate
// or ErrAircraftNotFound.
func (s *Service) Evaluate(ctx context.Context, userID uuid.UUID, req Request) (*Report, error) {
	today := utcDate(s.now())
	date := today
	if req.Date != nil {
		date = utcDate(*req.Date)
	}
	if date.Before(today) || date.After(today.AddDate(0, 0, MaxDaysAhead)) {
		return nil, ErrInvalidDate
	}

	var ac *models.Aircraft
	if reg := strings.TrimSpace(req.AircraftReg); reg != "" {
		found, err := s.findAircraft(ctx, userID, reg)
		if err != nil {
			return nil, err
		}
		ac = found
	}

	status, err := s.currency.EvaluateAsOf(ctx, userID, date)
	if err != nil {
		return nil, fmt.Errorf("evaluate currency: %w", err)
	}
	report := &Report{Date: date.Format("2006-01-02"), Items: []Item{}}
	if ac != nil {
		reg := ac.Registration
		report.AircraftReg = &reg
	}

	var launchItems []Item
	seenMethods := map[string]bool{}
	for _, r := range status.Ratings {
		if ac != nil && !ratingCovers(r, ac) {
			continue
		}
		report.Items = append(report.Items, ratingItem(r))
		for _, lm := range r.LaunchMethodCurrency {
			if seenMethods[lm.Method] {
				continue
			}
			seenMethods[lm.Method] = true
			launchItems = append(launchItems, launchMethodItem(r, lm))
		}
	}
	report.Items = append(report.Items, launchItems...)

	if req.Passengers {
		for _, pc := range status.PassengerCurrency {
			if ac != nil && !passengerCovers(pc, ac) {
				continue
			}
			report.Items = append(report.Items, passengerItem(pc))
		}
	}

	creds, err := s.credentials.ListCredentials(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	for _, c := range latestMedicals(creds) {
		report.Items = append(report.Items, credentialItem(c, date))
	}
	return report, nil
}

// findAircraft returns the user's aircraft registered as reg.
func (s *Service) findAircraft(ctx context.Context, userID uuid.UUID, reg string) (*models.Aircraft, error) {
	list, err := s.aircraft.ListAircraft(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list aircraft: %w", err)
	}
	want := registration.Canonical(reg)
	for _, ac := range list {
		if ac.UserID == userID && registration.Canonical(ac.Registration) == want {
			return ac, nil
		}
	}
	return nil, ErrAircraftNotFound
}

// utcDate returns midnight UTC on t's UTC date.
func utcDate(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// ratingCovers reports whether a rating's own class covers ac; an ULTRALIGHT
// rating with a kind covers only ultralights of that kind.
func ratingCovers(r currency.ClassRatingCurrency, ac *models.Aircraft) bool {
	class := models.NormalizeAircraftClass(ac.AircraftClass)
	if class == "" || r.ClassType != class {
		return false
	}
	if class == models.ClassTypeUL && r.ULKind != nil {
		return r.ULKind.CoversAircraftKind(ac.ULKind)
	}
	return true
}

// passengerCovers reports whether a passenger currency entry covers ac.
func passengerCovers(pc currency.PassengerCurrency, ac *models.Aircraft) bool {
	class := models.NormalizeAircraftClass(ac.AircraftClass)
	if class == "" || pc.ClassType != class {
		return false
	}
	if class == models.ClassTypeUL && pc.ULKind != nil {
		return pc.ULKind.CoversAircraftKind(ac.ULKind)
	}
	return true
}

// ratingItem answers for a rating: ready while current or expiring. A lapsed
// rating gives the remedy of its first unmet experience requirement, or of its
// proficiency check when that is the only one.
func ratingItem(r currency.ClassRatingCurrency) Item {
	it := Item{
		Kind:          KindRating,
		ClassRatingID: ptr(r.ClassRatingID),
		LicenseID:     ptr(r.LicenseID),
		ClassType:     ptr(r.ClassType),
		ULKind:        r.ULKind,
		Ready:         r.Status == currency.StatusCurrent || r.Status == currency.StatusExpiring,
		Status:        string(r.Status),
		ReasonKey:     r.MessageKey,
		Params:        r.MessageParams,
	}
	if r.Status != currency.StatusLapsed {
		return it
	}
	var check *currency.Requirement
	for i, req := range r.Requirements {
		if req.RemedyKey == "" {
			continue
		}
		if req.RemedyKey == currency.RemedyProficiencyCheck {
			if check == nil {
				check = &r.Requirements[i]
			}
			continue
		}
		it.ReasonKey, it.Params = req.RemedyKey, req.RemedyParams
		return it
	}
	if check != nil {
		it.ReasonKey, it.Params = check.RemedyKey, check.RemedyParams
	}
	return it
}

// launchMethodItem answers for one SFCL.155 launch method.
func launchMethodItem(r currency.ClassRatingCurrency, lm currency.LaunchMethodCurrency) Item {
	it := Item{
		Kind:          KindLaunchMethod,
		ClassRatingID: ptr(r.ClassRatingID),
		LicenseID:     ptr(r.LicenseID),
		ClassType:     ptr(r.ClassType),
		LaunchMethod:  ptr(lm.Method),
		Ready:         lm.Met,
	}
	if lm.Met {
		it.Status = string(currency.StatusCurrent)
		it.ReasonKey = currency.MsgReadinessLaunchMethodCurrent
		if lm.ValidUntil != nil {
			it.Params = &currency.MessageParams{Date: lm.ValidUntil}
		}
		return it
	}
	it.Status = string(currency.StatusLapsed)
	it.ReasonKey, it.Params = lm.RemedyKey, lm.RemedyParams
	return it
}

// passengerItem answers for day passenger carriage.
func passengerItem(pc currency.PassengerCurrency) Item {
	return Item{
		Kind:      KindPassengers,
		ClassType: ptr(pc.ClassType),
		ULKind:    pc.ULKind,
		Ready:     pc.DayStatus == currency.StatusCurrent,
		Status:    string(pc.DayStatus),
		ReasonKey: pc.MessageKey,
		Params:    pc.MessageParams,
	}
}

// credentialItem answers for a medical certificate on date.
func credentialItem(c *models.Credential, date time.Time) Item {
	at := date.Add(12 * time.Hour)
	it := Item{Kind: KindCredential, CredentialID: ptr(c.ID), Ready: !c.IsExpiredAt(at)}
	if c.ExpiryDate != nil {
		d := c.ExpiryDate.Format("2006-01-02")
		it.Params = &currency.MessageParams{Date: &d}
	}
	if it.Ready {
		it.Status, it.ReasonKey = StatusValid, currency.MsgReadinessCredentialValid
	} else {
		it.Status, it.ReasonKey = StatusExpired, currency.MsgReadinessCredentialExpired
	}
	return it
}

// latestMedicals returns, per medical credential type, the one expiring last;
// one without an expiry date counts as expiring last.
func latestMedicals(creds []*models.Credential) []*models.Credential {
	var out []*models.Credential
	index := map[models.CredentialType]int{}
	for _, c := range creds {
		if c == nil || !c.IsMedical() {
			continue
		}
		i, seen := index[c.CredentialType]
		if !seen {
			index[c.CredentialType] = len(out)
			out = append(out, c)
			continue
		}
		if expiresLater(c, out[i]) {
			out[i] = c
		}
	}
	return out
}

// expiresLater reports whether a expires after b.
func expiresLater(a, b *models.Credential) bool {
	if b.ExpiryDate == nil {
		return false
	}
	return a.ExpiryDate == nil || a.ExpiryDate.After(*b.ExpiryDate)
}

func ptr[T any](v T) *T { return &v }
