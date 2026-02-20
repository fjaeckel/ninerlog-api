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
	result := ClassRatingCurrency{
		ClassRatingID:       rating.ID,
		ClassType:           rating.ClassType,
		LicenseID:           rating.LicenseID,
		RegulatoryAuthority: license.RegulatoryAuthority,
		LicenseType:         license.LicenseType,
		RuleDescription:     "No auto-calculated rules — currency tracked by class rating expiry date only",
		Progress:            nil,
	}

	if rating.ExpiryDate != nil {
		expStr := rating.ExpiryDate.Format("2006-01-02")
		result.ExpiryDate = &expStr

		if rating.IsExpired() {
			result.Status = StatusExpired
			result.Message = fmt.Sprintf("%s rating expired on %s", rating.ClassType, expStr)
		} else if rating.IsExpiringSoon(90) {
			daysLeft := int(time.Until(*rating.ExpiryDate).Hours() / 24)
			result.Status = StatusExpiring
			result.Message = fmt.Sprintf("%s rating expires in %d days (%s)", rating.ClassType, daysLeft, expStr)
		} else {
			result.Status = StatusCurrent
			result.Message = fmt.Sprintf("%s rating valid until %s", rating.ClassType, expStr)
		}
	} else {
		result.Status = StatusUnknown
		result.Message = fmt.Sprintf("%s rating — no expiry date set (manual tracking)", rating.ClassType)
	}

	return result
}
