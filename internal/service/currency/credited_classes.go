package currency

import (
	"context"
	"sort"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// easaAeroplaneClasses are the aeroplane and TMG classes pooled for EASA FCL.140.A.
var easaAeroplaneClasses = []models.ClassType{
	models.ClassTypeSEPLand, models.ClassTypeSEPSea,
	models.ClassTypeMEPLand, models.ClassTypeMEPSea,
	models.ClassTypeSETLand, models.ClassTypeSETSea,
	models.ClassTypeTMG,
}

// easaLAPLClasses returns the FCL.140.A(a) pool: every aeroplane class and TMG.
func easaLAPLClasses(_ *models.ClassRating, _ []*models.ClassRating) []models.ClassType {
	return easaAeroplaneClasses
}

// easaSEPTMGClasses returns SEP_LAND and TMG together when the license holds
// both ratings (FCL.740.A(b)(1)), otherwise the rating's own class.
func easaSEPTMGClasses(rating *models.ClassRating, peers []*models.ClassRating) []models.ClassType {
	ct := rating.ClassType
	if (ct == models.ClassTypeSEPLand || ct == models.ClassTypeTMG) &&
		hasClass(peers, models.ClassTypeSEPLand) && hasClass(peers, models.ClassTypeTMG) {
		return []models.ClassType{models.ClassTypeSEPLand, models.ClassTypeTMG}
	}
	return []models.ClassType{ct}
}

// hasClass reports whether any rating in ratings has the given class.
func hasClass(ratings []*models.ClassRating, ct models.ClassType) bool {
	for _, r := range ratings {
		if r != nil && r.ClassType == ct {
			return true
		}
	}
	return false
}

// easaAnnexICredit returns the ultralights credited toward the rule's classes
// under FCL.035(a)(4): THREE_AXIS as SEP_LAND and THREE_AXIS_MOTORGLIDER as
// TMG, time and landings only.
func easaAnnexICredit(_ *models.ClassRating, classes []models.ClassType) *ulCredit {
	var kinds []models.ULKind
	for _, k := range models.ValidAircraftULKinds() {
		if ct, ok := k.PartFCLClass(); ok && containsClass(classes, ct) {
			kinds = append(kinds, k)
		}
	}
	if len(kinds) == 0 {
		return nil
	}
	return &ulCredit{sel: ULSelector{Kinds: kinds}}
}

// containsClass reports whether classes contains ct.
func containsClass(classes []models.ClassType, ct models.ClassType) bool {
	for _, c := range classes {
		if c == ct {
			return true
		}
	}
	return false
}

// faaClassPools are the aircraft classes of one FAA airplane class (§61.5(c)):
// single- or multi-engine, land or sea, whatever the powerplant.
var faaClassPools = [][]models.ClassType{
	{models.ClassTypeSEPLand, models.ClassTypeSETLand, models.AircraftClassSEJLand},
	{models.ClassTypeSEPSea, models.ClassTypeSETSea},
	{models.ClassTypeMEPLand, models.AircraftClassMETLand, models.AircraftClassMEJLand},
	{models.ClassTypeMEPSea, models.AircraftClassMETSea},
}

// faaClasses returns the FAA airplane class pool containing ct, or ct alone.
func faaClasses(ct models.ClassType) []models.ClassType {
	for _, pool := range faaClassPools {
		if containsClass(pool, ct) {
			return pool
		}
	}
	return []models.ClassType{ct}
}

// faaRatingClasses is faaClasses for a rating's class.
func faaRatingClasses(rating *models.ClassRating, _ []*models.ClassRating) []models.ClassType {
	return faaClasses(rating.ClassType)
}

// landingDaysByClasses merges GetLandingDaysByAircraftClass over classes into
// one entry per date, newest first.
func landingDaysByClasses(ctx context.Context, dp FlightDataProvider, userID uuid.UUID, classes []models.ClassType, includeTowed, picOnly bool, since time.Time) ([]LandingDay, error) {
	if len(classes) == 1 {
		return dp.GetLandingDaysByAircraftClass(ctx, userID, classes[0], includeTowed, picOnly, since)
	}
	byDate := map[time.Time]*LandingDay{}
	for _, ct := range classes {
		days, err := dp.GetLandingDaysByAircraftClass(ctx, userID, ct, includeTowed, picOnly, since)
		if err != nil {
			return nil, err
		}
		for _, d := range days {
			if e, ok := byDate[d.Date]; ok {
				e.DayLandings += d.DayLandings
				e.NightLandings += d.NightLandings
				continue
			}
			c := d
			byDate[d.Date] = &c
		}
	}
	out := make([]LandingDay, 0, len(byDate))
	for _, d := range byDate {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out, nil
}
