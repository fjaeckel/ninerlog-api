package models

import (
	"regexp"
	"strings"

	"github.com/fjaeckel/ninerlog-api/pkg/registration"
)

var germanGliderMark = regexp.MustCompile(`^[0-9]{4}$`)

// ClassFromRegistration returns the aircraft class a German registration
// implies: D-[0-9]{4} is GLIDER, D-M… is ULTRALIGHT. Every other registration,
// including D-K…, returns ok false. See docs/AIRCRAFT_REGISTRATIONS.md.
func ClassFromRegistration(reg string) (ClassType, bool) {
	res := registration.Normalize(reg)
	if !res.Matched || res.Entry.Prefix != "D" {
		return "", false
	}
	mark := strings.TrimPrefix(res.Value, "D-")
	switch {
	case germanGliderMark.MatchString(mark):
		return ClassTypeGlider, true
	case strings.HasPrefix(mark, "M"):
		return ClassTypeUL, true
	}
	return "", false
}

// InferImportedAircraftClass returns the class for an aircraft an import
// creates: sourceClass when set, else ClassFromRegistration, else GLIDER when
// towedLaunch, else nil.
func InferImportedAircraftClass(sourceClass ClassType, reg string, towedLaunch bool) *string {
	class := sourceClass
	if class == "" {
		class, _ = ClassFromRegistration(reg)
	}
	if class == "" && towedLaunch {
		class = ClassTypeGlider
	}
	if class == "" {
		return nil
	}
	s := string(class)
	return &s
}
