package models

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// LicencePrivilegeKind is the kind of a privilege recorded on a licence.
type LicencePrivilegeKind string

const (
	PrivilegeSailplaneTowing     LicencePrivilegeKind = "SAILPLANE_TOWING"
	PrivilegeBannerTowing        LicencePrivilegeKind = "BANNER_TOWING"
	PrivilegeCloudFlying         LicencePrivilegeKind = "CLOUD_FLYING"
	PrivilegeAerobaticBasic      LicencePrivilegeKind = "AEROBATIC_BASIC"
	PrivilegeAerobaticAdvanced   LicencePrivilegeKind = "AEROBATIC_ADVANCED"
	PrivilegeTMGNight            LicencePrivilegeKind = "TMG_NIGHT"
	PrivilegeFIS                 LicencePrivilegeKind = "FI_S"
	PrivilegeBIS                 LicencePrivilegeKind = "BI_S"
	PrivilegeFES                 LicencePrivilegeKind = "FE_S"
	PrivilegeULPassengerAuth     LicencePrivilegeKind = "UL_PASSENGER_AUTH"
	PrivilegeULTowing            LicencePrivilegeKind = "UL_TOWING"
	PrivilegeULTypeBriefing      LicencePrivilegeKind = "UL_TYPE_BRIEFING"
	PrivilegeLaunchMethodTrained LicencePrivilegeKind = "LAUNCH_METHOD_TRAINED"
)

// Bounds on licence privilege fields.
const (
	LicencePrivilegeDetailMaxLen = 100
	LicencePrivilegeNotesMaxLen  = 1000
)

// ErrInvalidLicencePrivilege is returned when a privilege fails validation.
var ErrInvalidLicencePrivilege = errors.New("invalid licence privilege")

// ValidLicencePrivilegeKinds returns every privilege kind.
func ValidLicencePrivilegeKinds() []LicencePrivilegeKind {
	return []LicencePrivilegeKind{
		PrivilegeSailplaneTowing, PrivilegeBannerTowing, PrivilegeCloudFlying,
		PrivilegeAerobaticBasic, PrivilegeAerobaticAdvanced, PrivilegeTMGNight,
		PrivilegeFIS, PrivilegeBIS, PrivilegeFES,
		PrivilegeULPassengerAuth, PrivilegeULTowing, PrivilegeULTypeBriefing,
		PrivilegeLaunchMethodTrained,
	}
}

// IsValid reports whether k is a known privilege kind.
func (k LicencePrivilegeKind) IsValid() bool {
	for _, v := range ValidLicencePrivilegeKinds() {
		if v == k {
			return true
		}
	}
	return false
}

// LicencePrivilege is a rating, endorsement or authorisation recorded on a
// licence. Detail is the launch method (LAUNCH_METHOD_TRAINED), the ultralight
// kind (UL_TOWING) or the aircraft type (UL_TYPE_BRIEFING).
type LicencePrivilege struct {
	ID        uuid.UUID            `json:"id"`
	UserID    uuid.UUID            `json:"userId"`
	LicenseID uuid.UUID            `json:"licenseId"`
	Kind      LicencePrivilegeKind `json:"kind"`
	Detail    *string              `json:"detail,omitempty"`
	IssuedOn  *time.Time           `json:"issuedOn,omitempty"`
	ExpiresOn *time.Time           `json:"expiresOn,omitempty"`
	Notes     *string              `json:"notes,omitempty"`
	CreatedAt time.Time            `json:"createdAt"`
	UpdatedAt time.Time            `json:"updatedAt"`
}

// Validate checks the kind, the kind's detail rule, the dates and text
// lengths. It trims detail (blank becomes nil), lower-cases a launch method
// and upper-cases an ultralight kind, and truncates dates to the day.
func (p *LicencePrivilege) Validate() error {
	if !p.Kind.IsValid() {
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidLicencePrivilege, p.Kind)
	}
	if p.Detail != nil {
		d := strings.TrimSpace(*p.Detail)
		switch {
		case d == "":
			p.Detail = nil
		case p.Kind == PrivilegeLaunchMethodTrained:
			d = strings.ToLower(d)
			p.Detail = &d
		case p.Kind == PrivilegeULTowing:
			d = strings.ToUpper(d)
			p.Detail = &d
		default:
			p.Detail = &d
		}
	}
	switch p.Kind {
	case PrivilegeLaunchMethodTrained:
		if p.Detail == nil || !IsValidLaunchMethod(*p.Detail) {
			return fmt.Errorf("%w: detail must be a launch method (%s) for kind %s",
				ErrInvalidLicencePrivilege, strings.Join(ValidLaunchMethods(), ", "), p.Kind)
		}
	case PrivilegeULTowing:
		if p.Detail == nil || !IsValidRatingULKind(ULKind(*p.Detail)) {
			return fmt.Errorf("%w: detail must be an ultralight kind for kind %s", ErrInvalidLicencePrivilege, p.Kind)
		}
	case PrivilegeULTypeBriefing:
		if p.Detail == nil {
			return fmt.Errorf("%w: detail (the aircraft type) is required for kind %s", ErrInvalidLicencePrivilege, p.Kind)
		}
	}
	if err := ValidateOptionalStringLength("detail", p.Detail, LicencePrivilegeDetailMaxLen); err != nil {
		return err
	}
	if err := ValidateOptionalStringLength("notes", p.Notes, LicencePrivilegeNotesMaxLen); err != nil {
		return err
	}
	p.IssuedOn = dateOnlyPtr(p.IssuedOn)
	p.ExpiresOn = dateOnlyPtr(p.ExpiresOn)
	if p.IssuedOn != nil && p.ExpiresOn != nil && p.ExpiresOn.Before(*p.IssuedOn) {
		return fmt.Errorf("%w: expiresOn must not be before issuedOn", ErrInvalidLicencePrivilege)
	}
	return nil
}

// IsExpiredOn reports whether the privilege has an expiry date before day.
func (p *LicencePrivilege) IsExpiredOn(day time.Time) bool {
	return p.ExpiresOn != nil && DateOnly(*p.ExpiresOn).Before(DateOnly(day))
}

func dateOnlyPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	d := DateOnly(*t)
	return &d
}
