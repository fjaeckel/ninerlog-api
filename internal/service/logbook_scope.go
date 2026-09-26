package service

import (
	"context"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/fjaeckel/ninerlog-api/pkg/registration"
	"github.com/google/uuid"
)

// CreditScoper returns the aircraft a class rating's recency counts.
type CreditScoper interface {
	CreditScope(license *models.License, rating *models.ClassRating, peers []*models.ClassRating) currency.CreditScope
}

// LogbookScope restricts a flight query to one licence's logbook: flights on
// aircraft of the licence's class ratings, and flights the currency engine
// credits toward them from other classes.
type LogbookScope struct {
	licenses     *LicenseService
	classRatings *ClassRatingService
	aircraft     *AircraftService
	credit       CreditScoper
}

// NewLogbookScope creates a logbook scope resolver. A nil credit scoper
// limits every logbook to the ratings' own classes.
func NewLogbookScope(licenses *LicenseService, classRatings *ClassRatingService, aircraft *AircraftService, credit CreditScoper) *LogbookScope {
	return &LogbookScope{licenses: licenses, classRatings: classRatings, aircraft: aircraft, credit: credit}
}

// LogbookMembership is how a flight belongs to a licence's logbook.
type LogbookMembership int

const (
	// LogbookExcluded: the flight is not in the logbook.
	LogbookExcluded LogbookMembership = iota
	// LogbookNative: the flight is on an aircraft of one of the licence's ratings.
	LogbookNative
	// LogbookCredited: the flight is on another class credited toward a rating.
	LogbookCredited
)

// Logbook is a licence's resolved logbook.
type Logbook struct {
	aircraft map[string]*models.Aircraft
	ratings  []logbookRating
}

type logbookRating struct {
	rating *models.ClassRating
	scope  currency.CreditScope
}

// Resolve returns the logbook of the licence, or nil when the licence has no
// class ratings. Returns ErrLicenseNotFound or ErrUnauthorizedAccess when the
// licence is not the user's.
func (s *LogbookScope) Resolve(ctx context.Context, userID, licenseID uuid.UUID) (*Logbook, error) {
	classRatings, err := s.classRatings.ListClassRatings(ctx, licenseID, userID)
	if err != nil {
		return nil, err
	}
	if len(classRatings) == 0 {
		return nil, nil
	}
	var license *models.License
	if s.credit != nil && s.licenses != nil {
		license, err = s.licenses.GetLicense(ctx, licenseID, userID)
		if err != nil {
			return nil, err
		}
	}
	aircraftList, err := s.aircraft.ListAircraft(ctx, userID)
	if err != nil {
		return nil, err
	}
	lb := &Logbook{aircraft: make(map[string]*models.Aircraft, len(aircraftList))}
	for _, ac := range aircraftList {
		lb.aircraft[registration.Canonical(ac.Registration)] = ac
	}
	for _, cr := range classRatings {
		r := logbookRating{rating: cr}
		if license != nil {
			r.scope = s.credit.CreditScope(license, cr, classRatings)
		}
		lb.ratings = append(lb.ratings, r)
	}
	return lb, nil
}

// Classify returns how f belongs to the logbook. A towed launch is credited
// only toward a rating that counts towed launches.
func (lb *Logbook) Classify(f *models.Flight) LogbookMembership {
	if f == nil {
		return LogbookExcluded
	}
	ac := lb.aircraft[registration.Canonical(f.AircraftReg)]
	if ac == nil {
		return LogbookExcluded
	}
	if lb.isNative(ac) {
		return LogbookNative
	}
	towed := models.IsTowedLaunch(f.LaunchMethod)
	for _, r := range lb.ratings {
		if r.scope.Covers(ac) && (!towed || r.scope.CountsTowed) {
			return LogbookCredited
		}
	}
	return LogbookExcluded
}

// isNative reports whether ac is of a rating's own class; an ULTRALIGHT
// rating with a kind covers only ultralights of that kind.
func (lb *Logbook) isNative(ac *models.Aircraft) bool {
	class := models.NormalizeAircraftClass(ac.AircraftClass)
	if class == "" {
		return false
	}
	for _, r := range lb.ratings {
		if r.rating.ClassType != class {
			continue
		}
		if class == models.ClassTypeUL && r.rating.ULKind != nil && !r.rating.ULKind.CoversAircraftKind(ac.ULKind) {
			continue
		}
		return true
	}
	return false
}

// creditState reports whether flights on ac are credited, and whether a towed
// launch on ac is credited too.
func (lb *Logbook) creditState(ac *models.Aircraft) (credited, towedCredited bool) {
	for _, r := range lb.ratings {
		if r.scope.Covers(ac) {
			credited = true
			towedCredited = towedCredited || r.scope.CountsTowed
		}
	}
	return credited, towedCredited
}

// Apply sets the registration filter on opts for the licence's logbook, as
// Classify. A licence without class ratings leaves opts unfiltered. Returns
// ErrLicenseNotFound or ErrUnauthorizedAccess when the licence is not the
// user's.
func (s *LogbookScope) Apply(ctx context.Context, userID, licenseID uuid.UUID, opts *repository.FlightQueryOptions) error {
	lb, err := s.Resolve(ctx, userID, licenseID)
	if err != nil || lb == nil {
		return err
	}
	regs := make([]string, 0, len(lb.aircraft))
	untowed := []string{}
	for reg, ac := range lb.aircraft {
		if lb.isNative(ac) {
			regs = append(regs, reg)
			continue
		}
		credited, towedCredited := lb.creditState(ac)
		switch {
		case credited && towedCredited:
			regs = append(regs, reg)
		case credited:
			untowed = append(untowed, reg)
		}
	}
	opts.FilterByRegistrations = true
	opts.AircraftRegistrations = regs
	opts.UntowedAircraftRegistrations = untowed
	return nil
}
