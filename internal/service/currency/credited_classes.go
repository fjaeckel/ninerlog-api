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
