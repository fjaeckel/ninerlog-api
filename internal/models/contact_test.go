package models

import (
	"testing"
)

func TestValidCrewRoles(t *testing.T) {
	roles := ValidCrewRoles()
	if len(roles) != 7 {
		t.Errorf("ValidCrewRoles() count = %d, want 7", len(roles))
	}
}

func TestIsValidCrewRole_Valid(t *testing.T) {
	validRoles := []CrewRole{
		CrewRolePIC, CrewRoleSIC, CrewRoleInstructor, CrewRoleStudent,
		CrewRolePassenger, CrewRoleSafetyPilot, CrewRoleExaminer,
	}
	for _, role := range validRoles {
		if !IsValidCrewRole(role) {
			t.Errorf("IsValidCrewRole(%s) = false, want true", role)
		}
	}
}

func TestIsValidCrewRole_Invalid(t *testing.T) {
	if IsValidCrewRole("Captain") {
		t.Error("IsValidCrewRole(Captain) = true, want false")
	}
	if IsValidCrewRole("") {
		t.Error("IsValidCrewRole('') = true, want false")
	}
}

func TestCrewRoleConstants(t *testing.T) {
	tests := []struct {
		role CrewRole
		want string
	}{
		{CrewRolePIC, "PIC"},
		{CrewRoleSIC, "SIC"},
		{CrewRoleInstructor, "Instructor"},
		{CrewRoleStudent, "Student"},
		{CrewRolePassenger, "Passenger"},
		{CrewRoleSafetyPilot, "SafetyPilot"},
		{CrewRoleExaminer, "Examiner"},
	}
	for _, tt := range tests {
		if string(tt.role) != tt.want {
			t.Errorf("CrewRole = %s, want %s", tt.role, tt.want)
		}
	}
}
