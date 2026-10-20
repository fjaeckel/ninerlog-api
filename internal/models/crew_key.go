package models

import (
	"reflect"
	"sort"
)

// CrewKey is a crew member reduced to what two crew lists are compared on.
type CrewKey struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

// SortCrewKeys sorts keys by role, then name.
func SortCrewKeys(keys []CrewKey) {
	sort.SliceStable(keys, func(i, j int) bool {
		if keys[i].Role != keys[j].Role {
			return keys[i].Role < keys[j].Role
		}
		return keys[i].Name < keys[j].Name
	})
}

// CrewKeys returns members as name and role pairs sorted by role, then name.
func CrewKeys(members []FlightCrewMember) []CrewKey {
	keys := make([]CrewKey, 0, len(members))
	for _, m := range members {
		keys = append(keys, CrewKey{Name: m.Name, Role: string(m.Role)})
	}
	SortCrewKeys(keys)
	return keys
}

// SameCrew reports whether a and b hold the same members by name and role.
func SameCrew(a, b []FlightCrewMember) bool {
	return SameCrewKeys(CrewKeys(a), CrewKeys(b))
}

// SameCrewKeys reports whether two sorted key lists are equal.
func SameCrewKeys(a, b []CrewKey) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}
