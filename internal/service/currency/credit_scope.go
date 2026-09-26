package currency

import "github.com/fjaeckel/ninerlog-api/internal/models"

// CreditScope lists the aircraft whose flights a rating's recency rule counts
// beside the rating's own ultralights: the rule's aircraft classes (the
// rating's own class included) and the ultralights credited from other
// disciplines. A cross-class rule (IR) lists no classes.
type CreditScope struct {
	Classes []models.ClassType
	ULs     []ULSelector
	// CountsTowed reports whether towed launches count toward the rating.
	CountsTowed bool
}

// CreditScoper is implemented by evaluators that declare a rating's CreditScope.
type CreditScoper interface {
	CreditScope(rating *models.ClassRating, license *models.License, peers []*models.ClassRating) CreditScope
}

// ruleCreditScope returns the CreditScope of rule for rating.
func ruleCreditScope(rule *ratingRule, rating *models.ClassRating, peers []*models.ClassRating) CreditScope {
	s := CreditScope{CountsTowed: includeTowedFlights(rating.ClassType, rule.countsTowed)}
	classes := resolveClasses(rule, rating, peers)
	s.Classes = append(s.Classes, classes...)
	if rule.ulCredit != nil {
		if ul := rule.ulCredit(rating, classes); ul != nil {
			s.CountsTowed = s.CountsTowed || ul.countsTowed
			if !ul.native {
				s.ULs = append(s.ULs, ul.sel)
			}
		}
	}
	if rule.extraCredit != nil {
		if extra := rule.extraCredit(rating); extra != nil {
			s.Classes = append(s.Classes, extra.classes...)
			if len(extra.ulKinds) > 0 {
				s.ULs = append(s.ULs, ULSelector{Kinds: extra.ulKinds})
			}
		}
	}
	return s
}

// Covers reports whether a flight on ac counts under the scope, launch method
// aside. Ultralights are covered only through ULs.
func (s CreditScope) Covers(ac *models.Aircraft) bool {
	if ac == nil {
		return false
	}
	class := models.NormalizeAircraftClass(ac.AircraftClass)
	if class == models.ClassTypeUL {
		for _, sel := range s.ULs {
			if sel.Matches(ac) {
				return true
			}
		}
		return false
	}
	return class != "" && containsClass(s.Classes, class)
}

// Matches reports whether ac is an ultralight the selector selects.
func (sel ULSelector) Matches(ac *models.Aircraft) bool {
	if ac == nil || !models.IsULClass(ac.AircraftClass) {
		return false
	}
	if ac.ULKind == nil {
		if !sel.IncludeUnspecified {
			return false
		}
	} else if !containsULKind(sel.Kinds, *ac.ULKind) {
		return false
	}
	return sel.MinMTOMKg == 0 || (ac.MTOMKg != nil && *ac.MTOMKg >= sel.MinMTOMKg)
}

// containsULKind reports whether kinds contains k.
func containsULKind(kinds []models.ULKind, k models.ULKind) bool {
	for _, v := range kinds {
		if v == k {
			return true
		}
	}
	return false
}

// CreditScope returns the scope of the EASA rule for rating.
func (e *EASAEvaluator) CreditScope(rating *models.ClassRating, license *models.License, peers []*models.ClassRating) CreditScope {
	return ruleCreditScope(easaSelectRule(rating, license), rating, peers)
}

// CreditScope returns the scope of the FAA rule for rating.
func (e *FAAEvaluator) CreditScope(rating *models.ClassRating, license *models.License, peers []*models.ClassRating) CreditScope {
	return ruleCreditScope(faaSelectRule(rating, license), rating, peers)
}

// CreditScope returns the scope of the German UL rule for an ULTRALIGHT
// rating, and of the EASA rule otherwise.
func (e *GermanULEvaluator) CreditScope(rating *models.ClassRating, license *models.License, peers []*models.ClassRating) CreditScope {
	if rating.ClassType != models.ClassTypeUL {
		return e.easa.CreditScope(rating, license, peers)
	}
	return ruleCreditScope(germanULSelectRule(ratingULKind(rating), license), rating, peers)
}

// CreditScope returns the scope of the fallback rule for rating.
func (e *OtherEvaluator) CreditScope(rating *models.ClassRating, _ *models.License, peers []*models.ClassRating) CreditScope {
	return ruleCreditScope(otherSelectRule(rating), rating, peers)
}

// CreditScope returns the CreditScope of rating on license, evaluated with
// the licence's other class ratings as peers.
func (s *Service) CreditScope(license *models.License, rating *models.ClassRating, peers []*models.ClassRating) CreditScope {
	if cs, ok := s.evaluatorFor(license).(CreditScoper); ok {
		return cs.CreditScope(rating, license, peers)
	}
	return CreditScope{Classes: []models.ClassType{rating.ClassType}}
}
