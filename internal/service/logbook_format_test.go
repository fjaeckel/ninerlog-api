package service

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

func TestLogbookFormatForLicence(t *testing.T) {
	tests := []struct {
		name string
		lic  *models.License
		want string
	}{
		{"L5 Lena SPL", &models.License{RegulatoryAuthority: "LBA", LicenseType: "SPL"}, LogbookFormatSailplane},
		{"LAPL(S)", &models.License{RegulatoryAuthority: "EASA", LicenseType: "LAPL(S)"}, LogbookFormatSailplane},
		{"FAA glider", &models.License{RegulatoryAuthority: "FAA", LicenseType: "Glider"}, LogbookFormatSailplane},
		{"M Mehmet DULV UL", &models.License{RegulatoryAuthority: "DULV", LicenseType: "UL"}, LogbookFormatUltralight},
		{"UL by issuing authority", &models.License{RegulatoryAuthority: "LBA", IssuingAuthority: "DAeC", LicenseType: "Sportpilotenlizenz"}, LogbookFormatUltralight},
		{"UL by type", &models.License{RegulatoryAuthority: "LBA", LicenseType: "Ultraleicht"}, LogbookFormatUltralight},
		{"A2 Mark ATPL(A)", &models.License{RegulatoryAuthority: "EASA", LicenseType: "ATPL(A)"}, LogbookFormatEASA},
		{"N Anna PPL(A)", &models.License{RegulatoryAuthority: "EASA", LicenseType: "PPL(A)"}, LogbookFormatEASA},
		{"unknown type", &models.License{RegulatoryAuthority: "EASA", LicenseType: "Balloon"}, LogbookFormatEASA},
		{"nil licence", nil, LogbookFormatEASA},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LogbookFormatForLicence(tt.lic); got != tt.want {
				t.Errorf("LogbookFormatForLicence = %q, want %q", got, tt.want)
			}
		})
	}
}
