package models

import "strings"

// ULKind is the kind of an ultralight (German "Luftsportgeräteart").
type ULKind string

const (
	ULKindThreeAxis            ULKind = "THREE_AXIS"
	ULKindThreeAxisMotorglider ULKind = "THREE_AXIS_MOTORGLIDER"
	ULKindWeightShift          ULKind = "WEIGHT_SHIFT"
	ULKindGyroplane            ULKind = "GYROPLANE"
	ULKindHelicopter           ULKind = "HELICOPTER"
	ULKindPoweredParaglider    ULKind = "POWERED_PARAGLIDER"
	ULKindSailplane            ULKind = "SAILPLANE"
)

// ValidAircraftULKinds returns the kinds an ultralight aircraft may have.
func ValidAircraftULKinds() []ULKind {
	return []ULKind{
		ULKindThreeAxis, ULKindThreeAxisMotorglider, ULKindWeightShift,
		ULKindGyroplane, ULKindHelicopter, ULKindPoweredParaglider, ULKindSailplane,
	}
}

// ValidRatingULKinds returns the kinds an ULTRALIGHT class rating may cover.
func ValidRatingULKinds() []ULKind {
	return []ULKind{
		ULKindThreeAxis, ULKindWeightShift, ULKindGyroplane,
		ULKindHelicopter, ULKindPoweredParaglider, ULKindSailplane,
	}
}

// IsValidAircraftULKind reports whether k is a valid aircraft ultralight kind.
func IsValidAircraftULKind(k ULKind) bool {
	return containsULKind(ValidAircraftULKinds(), k)
}

// IsValidRatingULKind reports whether k is a valid class rating ultralight kind.
func IsValidRatingULKind(k ULKind) bool {
	return containsULKind(ValidRatingULKinds(), k)
}

// PartFCLClass returns the Part-FCL class whose FCL.035(a)(4) requirements
// flights in an ultralight of kind k are credited toward.
func (k ULKind) PartFCLClass() (ClassType, bool) {
	switch k {
	case ULKindThreeAxis:
		return ClassTypeSEPLand, true
	case ULKindThreeAxisMotorglider:
		return ClassTypeTMG, true
	default:
		return "", false
	}
}

// AircraftKindsForRating returns the aircraft kinds a rating of kind k covers.
func AircraftKindsForRating(k ULKind) []ULKind {
	if k == ULKindThreeAxis {
		return []ULKind{ULKindThreeAxis, ULKindThreeAxisMotorglider}
	}
	return []ULKind{k}
}

// IsULClass reports whether a free-text aircraft class names ULTRALIGHT.
func IsULClass(aircraftClass *string) bool {
	return aircraftClass != nil && strings.ToUpper(strings.TrimSpace(*aircraftClass)) == string(ClassTypeUL)
}

func containsULKind(kinds []ULKind, k ULKind) bool {
	for _, v := range kinds {
		if v == k {
			return true
		}
	}
	return false
}
