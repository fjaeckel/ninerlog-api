package currency

import (
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// gplPassengerPICMinutes is the PIC time on gyroplanes after licence issue a
// GPL holder needs before carrying passengers (FCL.205.G(a)(2)).
const gplPassengerPICMinutes = 600

// gplAnnexICreditMinMTOMKg is the minimum MTOM of an Annex I gyroplane
// credited under FCL.035(a)(5).
const gplAnnexICreditMinMTOMKg = 450

// gplAnnexICredit credits UL gyroplanes of at least 450 kg toward GPL recency
// time and landings, not the refresher (FCL.035(a)(5)).
func gplAnnexICredit(_ *models.ClassRating, _ []models.ClassType, _ []*models.ClassRating) *ulCredit {
	return &ulCredit{sel: ULSelector{Kinds: []models.ULKind{models.ULKindGyroplane}, MinMTOMKg: gplAnnexICreditMinMTOMKg}}
}

// easaGPLRule — EASA FCL.240.G recency for gyroplanes (GPL):
//   - 12 hours flight time as PIC, dual or supervised solo on gyroplanes
//   - 12 takeoffs & landings
//   - 1 hour refresher training with an instructor
//   - OR a GPL proficiency check
//
// Lookback: rolling 2 years from NOW. UL gyroplanes of at least 450 kg count
// toward time and landings (FCL.035(a)(5)).
var easaGPLRule = ratingRule{
	displayKey:  "easa_gpl",
	description: "Requires 12h flight time + 12 takeoffs & landings + 1h refresher training with instructor on gyroplanes within the last 2 years, or a GPL proficiency check (EASA FCL.240.G(a)); ultralight gyroplanes of at least 450 kg count toward time and landings, not the refresher (FCL.035(a)(5))",
	window:      windowSpec{kind: windowRollingNow, years: 2},
	scope:       scopeByClass,
	ulCredit:    gplAnnexICredit,
	baseReqs: []reqSpec{
		{nameKey: ReqKeyTotalTime, metric: mTotalMinutes, threshold: 720, unit: "minutes"},
		{nameKey: ReqKeyLandings, metric: mLandings, threshold: 12, unit: "landings"},
		{nameKey: ReqKeyRefresherTraining, metric: mInstructorMinutes, threshold: 60, unit: "minutes"},
	},
	finalize: recencyFinalize(true),
}

// isGPL reports whether licenseType names an EASA gyroplane pilot licence.
func isGPL(licenseType string) bool {
	return strings.ToUpper(strings.TrimSpace(licenseType)) == "GPL"
}
