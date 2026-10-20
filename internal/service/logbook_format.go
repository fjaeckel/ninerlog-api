package service

import "github.com/fjaeckel/ninerlog-api/internal/models"

// Printed logbook formats of GET /exports/pdf.
const (
	LogbookFormatEASA       = "easa"
	LogbookFormatSailplane  = "sailplane"
	LogbookFormatUltralight = "ultralight"
)

// LogbookFormatForLicence returns the printed logbook format of a licence's
// logbook: LogbookFormatUltralight for an ultralight licence (by type, or by
// a DULV/DAeC regulatory or issuing authority), LogbookFormatSailplane for an
// SPL, LAPL(S) or FAA glider licence, LogbookFormatEASA otherwise.
func LogbookFormatForLicence(l *models.License) string {
	if l == nil {
		return LogbookFormatEASA
	}
	kind := models.ClassifyLicence(l.LicenseType, l.RegulatoryAuthority)
	if kind == models.LicenceKindUL ||
		models.ClassifyLicence(l.LicenseType, l.IssuingAuthority) == models.LicenceKindUL {
		return LogbookFormatUltralight
	}
	if kind.IsSailplane() {
		return LogbookFormatSailplane
	}
	return LogbookFormatEASA
}
