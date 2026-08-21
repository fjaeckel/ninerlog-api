package flightrules

import (
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/models"
)

const (
	pilot   = "Amelia Earhart"
	captain = "Captain Smith"
)

var (
	multiPilot  = &AircraftFacts{Registration: "D-AIBC", IsMultiPilot: true}
	singlePilot = &AircraftFacts{Registration: "D-EFGH"}
)

func crewed(members ...models.FlightCrewMember) *models.Flight {
	return &models.Flight{TotalTime: 90, CrewMembers: members}
}

func member(name string, role models.CrewRole) models.FlightCrewMember {
	return models.FlightCrewMember{Name: name, Role: role}
}

// Co-pilot time requires a co-pilot seat the operation actually calls for.
// The same crew list resolves differently depending on the aircraft, which is
// the whole point: a GA pilot sitting beside a friend in a C172 is not a
// crew member and has no co-pilot time to log (EASA FCL.010; 14 CFR
// §61.51(f)).
func TestDetermineRole_CoPilotSeatDependsOnAircraft(t *testing.T) {
	cases := []struct {
		name     string
		flight   *models.Flight
		userName string
		aircraft *AircraftFacts
		want     Role
	}{
		{
			name:     "third-party PIC on a multi-pilot aircraft",
			flight:   crewed(member(captain, models.CrewRolePIC)),
			userName: pilot,
			aircraft: multiPilot,
			want:     RoleSIC,
		},
		{
			name:     "third-party PIC on a single-pilot aircraft",
			flight:   crewed(member(captain, models.CrewRolePIC)),
			userName: pilot,
			aircraft: singlePilot,
			want:     RolePassenger,
		},
		{
			name:     "third-party PIC on an aircraft absent from the fleet",
			flight:   crewed(member(captain, models.CrewRolePIC)),
			userName: pilot,
			aircraft: nil,
			want:     RolePassenger,
		},
		{
			name: "self-declared co-pilot seat on a single-pilot aircraft",
			flight: crewed(
				member(captain, models.CrewRolePIC),
				member(pilot, models.CrewRoleSIC),
			),
			userName: pilot,
			aircraft: singlePilot,
			want:     RoleSIC,
		},
		{
			name: "safety pilot in simulated instrument flight",
			flight: crewed(
				member(captain, models.CrewRolePIC),
				member(pilot, models.CrewRoleSafetyPilot),
			),
			userName: pilot,
			aircraft: singlePilot,
			want:     RoleSIC,
		},
		{
			name:     "safety pilot with no PIC crew entry",
			flight:   crewed(member(pilot, models.CrewRoleSafetyPilot)),
			userName: pilot,
			aircraft: singlePilot,
			want:     RoleSIC,
		},
		{
			name: "instructor on board outranks the co-pilot question",
			flight: crewed(
				member(captain, models.CrewRolePIC),
				member("CFI Mueller", models.CrewRoleInstructor),
			),
			userName: pilot,
			aircraft: singlePilot,
			want:     RoleDualReceiving,
		},
		{
			name: "user is the designated PIC",
			flight: crewed(
				member(pilot, models.CrewRolePIC),
				member(captain, models.CrewRoleSIC),
			),
			userName: pilot,
			aircraft: multiPilot,
			want:     RolePIC,
		},
		{
			name:     "passengers on board leave the user as PIC",
			flight:   crewed(member("Pax A", models.CrewRolePassenger)),
			userName: pilot,
			aircraft: singlePilot,
			want:     RolePIC,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetermineRole(tc.flight, tc.userName, tc.aircraft); got != tc.want {
				t.Errorf("DetermineRole = %v, want %v", got, tc.want)
			}
		})
	}
}

// A co-pilot time the pilot entered is trusted on any aircraft; one a
// previous derivation wrote is not, so re-deriving can correct it.
func TestDetermineRole_DeclaredCoPilotTime(t *testing.T) {
	declared := crewed(member(captain, models.CrewRolePIC))
	declared.SICTime = 90
	declared.SICTimeOverride = true
	if got := DetermineRole(declared, pilot, singlePilot); got != RoleSIC {
		t.Errorf("declared co-pilot time = %v, want RoleSIC", got)
	}

	derived := crewed(member(captain, models.CrewRolePIC))
	derived.SICTime = 90
	if got := DetermineRole(derived, pilot, singlePilot); got != RolePassenger {
		t.Errorf("derived co-pilot time = %v, want RolePassenger", got)
	}

	// No crew list at all: an import that declares co-pilot time.
	noCrew := &models.Flight{TotalTime: 90, SICTime: 90, SICTimeOverride: true}
	if got := DetermineRole(noCrew, pilot, nil); got != RoleSIC {
		t.Errorf("declared co-pilot time without crew = %v, want RoleSIC", got)
	}
}

// The multi-pilot column records time in aeroplanes certificated for a
// minimum crew of two (FCL.010). A safety pilot makes the user a required
// crew member but does not make a C172 a multi-pilot aeroplane.
func TestIsMultiPilotOperation_RequiresMultiPilotAircraft(t *testing.T) {
	twoCrew := crewed(
		member(captain, models.CrewRolePIC),
		member(pilot, models.CrewRoleSIC),
	)
	if !IsMultiPilotOperation(twoCrew, RoleSIC, multiPilot) {
		t.Error("two-pilot crew on a multi-pilot aircraft must be a multi-pilot operation")
	}
	if IsMultiPilotOperation(twoCrew, RoleSIC, singlePilot) {
		t.Error("a single-pilot aircraft is never a multi-pilot operation")
	}
	if IsMultiPilotOperation(twoCrew, RoleSIC, nil) {
		t.Error("an unknown aircraft is not a multi-pilot operation")
	}

	safety := crewed(
		member(captain, models.CrewRolePIC),
		member(pilot, models.CrewRoleSafetyPilot),
	)
	if IsMultiPilotOperation(safety, RoleSIC, singlePilot) {
		t.Error("a safety pilot logs co-pilot time but not multi-pilot time")
	}
}

// A passenger flight is not flight time and never reaches an aggregate.
func TestCountsAsFlightTime_Passenger(t *testing.T) {
	if CountsAsFlightTime(&models.Flight{IsPassenger: true}) {
		t.Error("a passenger flight must not count as flight time")
	}
	if !CountsAsFlightTime(&models.Flight{}) {
		t.Error("an ordinary flight must count as flight time")
	}
}
