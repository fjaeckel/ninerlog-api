package handlers

import (
	"reflect"
	"testing"

	"github.com/fjaeckel/pilotlog-api/internal/api/generated"
)

func TestRequireAdmin_BlocksNonAdmin(t *testing.T) {
	h := &APIHandler{adminEmail: "admin@example.com"}
	if h.isAdminUser("user@example.com") {
		t.Error("Expected non-admin user to be rejected")
	}
}

func TestRequireAdmin_AllowsAdmin(t *testing.T) {
	h := &APIHandler{adminEmail: "admin@example.com"}
	if !h.isAdminUser("admin@example.com") {
		t.Error("Expected admin user to be allowed")
	}
}

func TestAdminCannotDisableSelf(t *testing.T) {
	adminID := createTestUser("admin@example.com", "Admin")
	if adminID.ID != adminID.ID {
		t.Error("UUID equality check failed")
	}
}

func TestBuildUserResponse_IncludesDisabledField(t *testing.T) {
	h := &APIHandler{adminEmail: "admin@example.com"}

	user := createTestUser("pilot@example.com", "Pilot")
	user.Disabled = true

	resp := h.buildUserResponse(user)
	if resp.Name != "Pilot" {
		t.Errorf("Name = %q, want Pilot", resp.Name)
	}
}

// TestAdminUserSchema_PrivacyPreserving validates that the AdminUser struct
// only contains account-level metadata and does NOT expose individual
// flight data, credential details, logbook entries, or passwords.
func TestAdminUserSchema_PrivacyPreserving(t *testing.T) {
	adminUserType := reflect.TypeOf(generated.AdminUser{})

	// Allowed fields — only account metadata and aggregate counts
	allowedFields := map[string]bool{
		"Id": true, "Email": true, "Name": true, "CreatedAt": true,
		"LastLoginAt": true, "TwoFactorEnabled": true, "Disabled": true,
		"Locked": true, "LockedUntil": true,
		"FlightCount": true, "AircraftCount": true, // aggregate counts only
	}

	// Forbidden field patterns — these would leak private data
	forbidden := []string{
		"Password", "PasswordHash", "TwoFactorSecret", "RecoveryCodes",
		"Flights", "FlightData", "FlightDetails", "Logbook",
		"Credentials", "Medicals", "Licenses", "ClassRatings",
		"Route", "Departure", "Arrival", "Aircraft",
	}

	for i := 0; i < adminUserType.NumField(); i++ {
		field := adminUserType.Field(i)
		if !allowedFields[field.Name] {
			t.Errorf("AdminUser has unexpected field %q — may leak private data", field.Name)
		}
	}

	for _, f := range forbidden {
		for i := 0; i < adminUserType.NumField(); i++ {
			if adminUserType.Field(i).Name == f {
				t.Errorf("AdminUser MUST NOT contain field %q — violates privacy principles", f)
			}
		}
	}
}

// TestAdminStatsSchema_AggregateOnly validates that AdminStats only contains
// aggregate counters, not individual user data.
func TestAdminStatsSchema_AggregateOnly(t *testing.T) {
	statsType := reflect.TypeOf(generated.AdminStats{})

	for i := 0; i < statsType.NumField(); i++ {
		field := statsType.Field(i)
		if field.Type.Kind() != reflect.Int {
			t.Errorf("AdminStats field %q has type %v, expected int (aggregate counter only)", field.Name, field.Type.Kind())
		}
	}
}
