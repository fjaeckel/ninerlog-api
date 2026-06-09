package currency

import (
	"context"
	"fmt"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

// OtherEvaluator handles authorities without specific currency rules.
// It only tracks expiry — no auto-calculated currency.
type OtherEvaluator struct{}

func NewOtherEvaluator() *OtherEvaluator {
	return &OtherEvaluator{}
}

func (e *OtherEvaluator) Authority() string {
	return "" // fallback for any unrecognized authority
}

func (e *OtherEvaluator) Evaluate(ctx context.Context, rating *models.ClassRating, license *models.License, dataProvider FlightDataProvider) ClassRatingCurrency {
	return evalRatingRule(ctx, &otherExpiryRule, rating, license, dataProvider)
}

// otherExpiryRule — expiry-only tracking for unrecognized authorities.
var otherExpiryRule = ratingRule{
	displayKey:  "",
	description: "No auto-calculated rules — currency tracked by class rating expiry date only",
	scope:       scopeByClass,
	finalize: func(_ context.Context, rt *ratingRuntime) {
		rating := rt.rating
		if rating.ExpiryDate != nil {
			expStr := *rt.result.ExpiryDate
			if rating.IsExpired() {
				rt.result.Status = StatusExpired
				rt.result.Message = fmt.Sprintf("%s rating expired on %s", rating.ClassType, expStr)
			} else if rating.IsExpiringSoon(90) {
				daysLeft := int(time.Until(*rating.ExpiryDate).Hours() / 24)
				rt.result.Status = StatusExpiring
				rt.result.Message = fmt.Sprintf("%s rating expires in %d days (%s)", rating.ClassType, daysLeft, expStr)
			} else {
				rt.result.Status = StatusCurrent
				rt.result.Message = fmt.Sprintf("%s rating valid until %s", rating.ClassType, expStr)
			}
		} else {
			rt.result.Status = StatusUnknown
			rt.result.Message = fmt.Sprintf("%s rating — no expiry date set (manual tracking)", rating.ClassType)
		}
	},
}
