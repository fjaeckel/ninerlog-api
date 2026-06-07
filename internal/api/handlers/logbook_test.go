package handlers

import (
	"strings"
	"testing"
)

// collectAllowedRegistrations mirrors the logic in ListFlights: given the set of
// class types a license is rated for and the user's aircraft (reg → class), it
// returns the upper-cased registrations whose class qualifies for the logbook.
// These registrations are handed to the SQL layer, which filters and paginates
// together so the reported total stays correct (the previous in-memory filter
// ran AFTER pagination and undercounted multi-page logbooks).
func collectAllowedRegistrations(allowedClasses map[string]bool, regToClass map[string]string) []string {
	regs := make([]string, 0, len(regToClass))
	for reg, class := range regToClass {
		if class != "" && allowedClasses[class] {
			regs = append(regs, strings.ToUpper(reg))
		}
	}
	return regs
}

func containsReg(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func TestCollectAllowedRegistrations(t *testing.T) {
	// A license rated for SEP_LAND and TMG should only collect registrations of
	// aircraft whose class matches; aircraft with no class or a different class
	// must be excluded.
	allowedClasses := map[string]bool{
		"SEP_LAND": true,
		"TMG":      true,
	}
	regToClass := map[string]string{
		"D-EABC": "SEP_LAND",
		"D-KXYZ": "TMG",
		"D-EFGH": "MEP_LAND", // not in allowed classes
		"D-NOPE": "",         // no class assigned
	}

	regs := collectAllowedRegistrations(allowedClasses, regToClass)

	if len(regs) != 2 {
		t.Fatalf("Expected 2 allowed registrations, got %d (%v)", len(regs), regs)
	}
	if !containsReg(regs, "D-EABC") {
		t.Errorf("Expected D-EABC to be allowed, got %v", regs)
	}
	if !containsReg(regs, "D-KXYZ") {
		t.Errorf("Expected D-KXYZ to be allowed, got %v", regs)
	}
	if containsReg(regs, "D-EFGH") {
		t.Errorf("MEP_LAND aircraft should not be allowed, got %v", regs)
	}
	if containsReg(regs, "D-NOPE") {
		t.Errorf("Unclassified aircraft should not be allowed, got %v", regs)
	}
}

func TestCollectAllowedRegistrations_AllMatch(t *testing.T) {
	allowedClasses := map[string]bool{"SEP_LAND": true}
	regToClass := map[string]string{
		"D-EABC": "SEP_LAND",
		"D-EFGH": "SEP_LAND",
	}

	regs := collectAllowedRegistrations(allowedClasses, regToClass)

	if len(regs) != 2 {
		t.Fatalf("Expected 2 allowed registrations, got %d (%v)", len(regs), regs)
	}
}

func TestCollectAllowedRegistrations_NoneMatch(t *testing.T) {
	// When no aircraft match the license's ratings, the allowed registration set
	// is empty. The SQL layer treats an empty set (with filtering enabled) as
	// "no qualifying flights".
	allowedClasses := map[string]bool{"IR": true}
	regToClass := map[string]string{
		"D-EABC": "SEP_LAND",
	}

	regs := collectAllowedRegistrations(allowedClasses, regToClass)

	if len(regs) != 0 {
		t.Fatalf("Expected 0 allowed registrations, got %d (%v)", len(regs), regs)
	}
}

func TestCollectAllowedRegistrations_UpperCases(t *testing.T) {
	// Registrations are upper-cased so the SQL UPPER(aircraft_reg) IN (...)
	// comparison matches regardless of how the aircraft was entered.
	allowedClasses := map[string]bool{"SEP_LAND": true}
	regToClass := map[string]string{
		"d-eabc": "SEP_LAND",
	}

	regs := collectAllowedRegistrations(allowedClasses, regToClass)

	if len(regs) != 1 || regs[0] != "D-EABC" {
		t.Fatalf("Expected [D-EABC], got %v", regs)
	}
}
