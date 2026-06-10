package handlers

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// resolveLogbookAllowedClasses determines which aircraft classes belong to a
// specific logbook license.
//
// Priority:
//  1. Explicit class ratings on the license.
//  2. For glider licenses without ratings, default to OTHER (sailplane class).
//  3. For non-glider licenses without ratings, include all known aircraft
//     classes except classes claimed by other separate-logbook licenses
//     (currently glider OTHER).
func (h *APIHandler) resolveLogbookAllowedClasses(ctx context.Context, userID, licenseID uuid.UUID) (map[string]bool, error) {
	lic, err := h.licenseService.GetLicense(ctx, licenseID, userID)
	if err != nil {
		return nil, err
	}

	classRatings, err := h.classRatingService.ListClassRatings(ctx, licenseID, userID)
	if err == nil && len(classRatings) > 0 {
		allowed := make(map[string]bool, len(classRatings))
		for _, cr := range classRatings {
			allowed[string(cr.ClassType)] = true
		}
		return allowed, nil
	}

	if isGliderLogbookLicenseType(lic.LicenseType) {
		return map[string]bool{"OTHER": true}, nil
	}

	// License without explicit ratings: include all known classes except those
	// reserved by other separate-logbook licenses.
	claimedByOtherLogbooks := make(map[string]bool)
	licenses, lerr := h.licenseService.ListLicenses(ctx, userID)
	if lerr == nil {
		for _, other := range licenses {
			if other.ID == licenseID || !other.RequiresSeparateLogbook {
				continue
			}
			if isGliderLogbookLicenseType(other.LicenseType) {
				claimedByOtherLogbooks["OTHER"] = true
			}
		}
	}

	aircraft, aerr := h.aircraftService.ListAircraft(ctx, userID)
	if aerr != nil {
		return nil, aerr
	}

	allowed := map[string]bool{}
	for _, ac := range aircraft {
		if ac.AircraftClass == nil || *ac.AircraftClass == "" {
			continue
		}
		if claimedByOtherLogbooks[*ac.AircraftClass] {
			continue
		}
		allowed[*ac.AircraftClass] = true
	}

	return allowed, nil
}

func isGliderLogbookLicenseType(licenseType string) bool {
	lt := strings.ToUpper(strings.TrimSpace(licenseType))
	return lt == "SPL" || lt == "LAPL(S)" || lt == "FAA_GLIDER" || lt == "GLIDER"
}
