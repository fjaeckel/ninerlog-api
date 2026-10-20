package models

import "testing"

func TestSameCrew(t *testing.T) {
	m := func(name, role string) FlightCrewMember { return FlightCrewMember{Name: name, Role: CrewRole(role)} }
	tests := []struct {
		name string
		a, b []FlightCrewMember
		want bool
	}{
		{"both nil", nil, nil, true},
		{"nil and empty", nil, []FlightCrewMember{}, true},
		{"same order", []FlightCrewMember{m("Pat", "Instructor")}, []FlightCrewMember{m("Pat", "Instructor")}, true},
		{"different order", []FlightCrewMember{m("Pat", "Instructor"), m("Sam", "Student")}, []FlightCrewMember{m("Sam", "Student"), m("Pat", "Instructor")}, true},
		{"different name", []FlightCrewMember{m("Pat", "Instructor")}, []FlightCrewMember{m("Pam", "Instructor")}, false},
		{"different role", []FlightCrewMember{m("Pat", "Instructor")}, []FlightCrewMember{m("Pat", "Student")}, false},
		{"different length", []FlightCrewMember{m("Pat", "Instructor")}, nil, false},
		{"duplicates count", []FlightCrewMember{m("Pat", "Instructor"), m("Pat", "Instructor")}, []FlightCrewMember{m("Pat", "Instructor")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SameCrew(tt.a, tt.b); got != tt.want {
				t.Errorf("SameCrew() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCrewKeysSortByRoleThenName(t *testing.T) {
	keys := CrewKeys([]FlightCrewMember{
		{Name: "Zed", Role: "Instructor"}, {Name: "Amy", Role: "Student"}, {Name: "Bob", Role: "Instructor"},
	})
	want := []CrewKey{{"Bob", "Instructor"}, {"Zed", "Instructor"}, {"Amy", "Student"}}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys = %v, want %v", keys, want)
		}
	}
}
