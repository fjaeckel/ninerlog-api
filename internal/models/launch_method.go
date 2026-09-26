package models

import (
	"errors"
	"strings"
)

// Sailplane launch methods (SFCL.155).
const (
	LaunchMethodWinch      = "winch"
	LaunchMethodAerotow    = "aerotow"
	LaunchMethodSelfLaunch = "self-launch"
	LaunchMethodCar        = "car"
	LaunchMethodBungee     = "bungee"
)

// ErrInvalidLaunchMethod is returned for a launch method outside ValidLaunchMethods.
var ErrInvalidLaunchMethod = errors.New("invalid launch method")

// ValidLaunchMethods returns every accepted launch method.
func ValidLaunchMethods() []string {
	return []string{
		LaunchMethodWinch, LaunchMethodAerotow, LaunchMethodSelfLaunch,
		LaunchMethodCar, LaunchMethodBungee,
	}
}

// IsValidLaunchMethod reports whether m is one of ValidLaunchMethods.
func IsValidLaunchMethod(m string) bool {
	for _, v := range ValidLaunchMethods() {
		if v == m {
			return true
		}
	}
	return false
}

// TowedLaunchMethods returns the launch methods that do not count toward
// powered classes.
func TowedLaunchMethods() []string {
	return []string{LaunchMethodWinch, LaunchMethodAerotow, LaunchMethodCar, LaunchMethodBungee}
}

// IsTowedLaunch reports whether m is a launch by external means (winch,
// aerotow, car or bungee).
func IsTowedLaunch(m string) bool {
	switch m {
	case LaunchMethodWinch, LaunchMethodAerotow, LaunchMethodCar, LaunchMethodBungee:
		return true
	}
	return false
}

// NormalizeLaunchMethod trims and lower-cases f.LaunchMethod and clears it
// when blank.
func NormalizeLaunchMethod(f *Flight) {
	if f.LaunchMethod == nil {
		return
	}
	m := strings.ToLower(strings.TrimSpace(*f.LaunchMethod))
	if m == "" {
		f.LaunchMethod = nil
		return
	}
	f.LaunchMethod = &m
}

// LaunchMethodApplies reports whether a flight on an aircraft of this class
// and ultralight kind carries a launch method: no class, GLIDER, TMG, or
// ULTRALIGHT with no kind or kind SAILPLANE or THREE_AXIS_MOTORGLIDER.
func LaunchMethodApplies(aircraftClass *string, ulKind *ULKind) bool {
	if aircraftClass == nil {
		return true
	}
	switch ClassType(strings.ToUpper(strings.TrimSpace(*aircraftClass))) {
	case "", ClassTypeGlider, ClassTypeTMG:
		return true
	case ClassTypeUL:
		return ulKind == nil || *ulKind == "" || *ulKind == ULKindSailplane || *ulKind == ULKindThreeAxisMotorglider
	}
	return false
}
