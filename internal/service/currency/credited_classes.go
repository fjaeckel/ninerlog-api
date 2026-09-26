package currency

import "github.com/fjaeckel/ninerlog-api/internal/models"

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
