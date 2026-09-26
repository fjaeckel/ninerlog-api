package models

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLicencePrivilegeValidate(t *testing.T) {
	str := func(s string) *string { return &s }
	day := func(s string) *time.Time {
		d, _ := time.Parse("2006-01-02", s)
		return &d
	}
	tests := []struct {
		name       string
		p          LicencePrivilege
		wantErr    error
		wantDetail *string
	}{
		{name: "P towing privilege without detail", p: LicencePrivilege{Kind: PrivilegeSailplaneTowing}},
		{name: "unknown kind", p: LicencePrivilege{Kind: "HOT_AIR"}, wantErr: ErrInvalidLicencePrivilege},
		{name: "launch method trained normalises case", p: LicencePrivilege{Kind: PrivilegeLaunchMethodTrained, Detail: str(" Aerotow ")}, wantDetail: str("aerotow")},
		{name: "launch method trained needs a detail", p: LicencePrivilege{Kind: PrivilegeLaunchMethodTrained}, wantErr: ErrInvalidLicencePrivilege},
		{name: "launch method trained rejects unknown method", p: LicencePrivilege{Kind: PrivilegeLaunchMethodTrained, Detail: str("catapult")}, wantErr: ErrInvalidLicencePrivilege},
		{name: "UL towing normalises kind", p: LicencePrivilege{Kind: PrivilegeULTowing, Detail: str("three_axis")}, wantDetail: str("THREE_AXIS")},
		{name: "UL towing needs a kind", p: LicencePrivilege{Kind: PrivilegeULTowing, Detail: str("  ")}, wantErr: ErrInvalidLicencePrivilege},
		{name: "UL towing rejects unknown kind", p: LicencePrivilege{Kind: PrivilegeULTowing, Detail: str("BALLOON")}, wantErr: ErrInvalidLicencePrivilege},
		{name: "Einweisung needs a type", p: LicencePrivilege{Kind: PrivilegeULTypeBriefing}, wantErr: ErrInvalidLicencePrivilege},
		{name: "Einweisung with type", p: LicencePrivilege{Kind: PrivilegeULTypeBriefing, Detail: str(" C42 ")}, wantDetail: str("C42")},
		{name: "Einweisung type too long", p: LicencePrivilege{Kind: PrivilegeULTypeBriefing, Detail: str(strings.Repeat("x", 101))}, wantErr: ErrFieldTooLong},
		{name: "blank detail becomes nil", p: LicencePrivilege{Kind: PrivilegeCloudFlying, Detail: str(" ")}},
		{name: "notes too long", p: LicencePrivilege{Kind: PrivilegeFIS, Notes: str(strings.Repeat("x", 1001))}, wantErr: ErrFieldTooLong},
		{name: "expiry before issue", p: LicencePrivilege{Kind: PrivilegeFIS, IssuedOn: day("2026-05-01"), ExpiresOn: day("2026-04-30")}, wantErr: ErrInvalidLicencePrivilege},
		{name: "expiry on issue day", p: LicencePrivilege{Kind: PrivilegeFIS, IssuedOn: day("2026-05-01"), ExpiresOn: day("2026-05-01")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.p.Validate()
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				return
			}
			switch {
			case tc.wantDetail == nil && tc.p.Detail != nil:
				t.Errorf("detail = %q, want nil", *tc.p.Detail)
			case tc.wantDetail != nil && (tc.p.Detail == nil || *tc.p.Detail != *tc.wantDetail):
				t.Errorf("detail = %v, want %q", tc.p.Detail, *tc.wantDetail)
			}
		})
	}
}

func TestLicencePrivilegeIsExpiredOn(t *testing.T) {
	exp := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	p := LicencePrivilege{Kind: PrivilegeFIS, ExpiresOn: &exp}
	if p.IsExpiredOn(exp.Add(15 * time.Hour)) {
		t.Error("expired on its expiry day")
	}
	if !p.IsExpiredOn(exp.AddDate(0, 0, 1)) {
		t.Error("not expired the day after")
	}
	if (&LicencePrivilege{Kind: PrivilegeFIS}).IsExpiredOn(exp) {
		t.Error("a privilege with no expiry date expired")
	}
}
