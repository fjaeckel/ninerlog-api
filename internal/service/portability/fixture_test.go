package portability

import (
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// The fixture bundle every format test renders.
//
// It is deliberately awkward rather than tidy, because the point of these
// tests is to catch a format regression on the messy rows a real career
// produces, not on a clean happy path:
//
//   - a single-pilot training flight with an instructor and dual time;
//   - a multi-pilot IFR sector with an SIC and structured approaches;
//   - a simulator session, which is not flight time at all;
//   - a flight on an aircraft that was never added to the fleet;
//   - a remarks field that starts with "=", which a spreadsheet would
//     otherwise execute.

func mustUUID(s string) uuid.UUID { return uuid.MustParse(s) }

func ptr[T any](v T) *T { return &v }

func fixtureBundle() *Bundle {
	licenseID := mustUUID("aaaaaaaa-0000-4000-8000-000000000001")

	return &Bundle{
		ExportedAt: time.Date(2026, 3, 14, 9, 30, 0, 0, time.UTC),
		PilotName:  "Anna Beispiel",
		PilotEmail: "anna@example.test",

		Aircraft: []*models.Aircraft{
			{
				ID:            mustUUID("bbbbbbbb-0000-4000-8000-000000000001"),
				Registration:  "D-EABC",
				Type:          "C172",
				Make:          "Cessna",
				Model:         "172S Skyhawk",
				AircraftClass: ptr("SEP_LAND"),
				IsTailwheel:   false,
				IsActive:      true,
			},
			{
				ID:                mustUUID("bbbbbbbb-0000-4000-8000-000000000002"),
				Registration:      "D-IXYZ",
				Type:              "BE58",
				Make:              "Beechcraft",
				Model:             "Baron 58",
				AircraftClass:     ptr("MEP_LAND"),
				IsComplex:         true,
				IsHighPerformance: true,
				IsActive:          true,
			},
		},

		Flights: []*models.Flight{
			// 1. Dual training circuit detail, with an instructor on board.
			{
				ID:            mustUUID("cccccccc-0000-4000-8000-000000000001"),
				Date:          time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC),
				AircraftReg:   "D-EABC",
				AircraftType:  "C172",
				DepartureICAO: ptr("EDDF"),
				ArrivalICAO:   ptr("EDDF"),
				OffBlockTime:  ptr("08:15:00"),
				DepartureTime: ptr("08:25:00"),
				ArrivalTime:   ptr("09:40:00"),
				OnBlockTime:   ptr("09:50:00"),
				TotalTime:     95,
				DualTime:      95,
				IsDual:        true,
				TakeoffsDay:   6,
				LandingsDay:   6,
				AllLandings:   6,
				Remarks:       ptr("Circuits and bumps"),
				CrewMembers: []models.FlightCrewMember{
					{Name: "Karl Fluglehrer", Role: models.CrewRoleInstructor},
				},
				CreatedAt: time.Date(2026, 1, 12, 10, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 1, 12, 10, 0, 0, 0, time.UTC),
			},
			// 2. Multi-pilot IFR sector with structured approaches and an SIC.
			{
				ID:                      mustUUID("cccccccc-0000-4000-8000-000000000002"),
				Date:                    time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC),
				AircraftReg:             "D-IXYZ",
				AircraftType:            "BE58",
				DepartureICAO:           ptr("EDDF"),
				ArrivalICAO:             ptr("LSZH"),
				Route:                   ptr("EDDF DKB LSZH"),
				OffBlockTime:            ptr("14:00:00"),
				DepartureTime:           ptr("14:12:00"),
				ArrivalTime:             ptr("15:35:00"),
				OnBlockTime:             ptr("15:45:00"),
				TotalTime:               105,
				PICTime:                 105,
				IsPIC:                   true,
				MultiPilotTime:          105,
				NightTime:               25,
				IFRTime:                 90,
				ActualInstrumentTime:    40,
				SimulatedInstrumentTime: 10,
				CrossCountryTime:        105,
				Distance:                154.3,
				TakeoffsDay:             1,
				LandingsNight:           1,
				AllLandings:             1,
				Holds:                   1,
				ApproachesCount:         3,
				Approaches: []models.ApproachEntry{
					{Type: "ILS", Airport: ptr("LSZH"), Runway: ptr("14")},
					{Type: "RNAV/GPS", Airport: ptr("LSZH"), Runway: ptr("16")},
				},
				Endorsements: ptr("Line check completed"),
				// A remarks value that a spreadsheet would treat as a formula.
				Remarks: ptr("=SUM(A1:A9) noted in tech log"),
				CrewMembers: []models.FlightCrewMember{
					{Name: "Anna Beispiel", Role: models.CrewRolePIC},
					{Name: "Peter Kopilot", Role: models.CrewRoleSIC},
				},
				SignatureID: ptr(mustUUID("dddddddd-0000-4000-8000-000000000001")),
				CreatedAt:   time.Date(2026, 2, 3, 16, 0, 0, 0, time.UTC),
				UpdatedAt:   time.Date(2026, 2, 3, 16, 0, 0, 0, time.UTC),
			},
			// 3. FSTD session — not flight time, and every format has to say so.
			{
				ID:                      mustUUID("cccccccc-0000-4000-8000-000000000003"),
				Date:                    time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC),
				AircraftReg:             "SIM-FNPT2",
				AircraftType:            "FNPT II",
				FSTDType:                ptr("FNPT II MCC"),
				TotalTime:               120,
				SimulatedFlightTime:     120,
				SimulatedInstrumentTime: 120,
				DualTime:                120,
				IsDual:                  true,
				ApproachesCount:         4,
				Remarks:                 ptr("IR renewal training"),
				CreatedAt:               time.Date(2026, 2, 20, 18, 0, 0, 0, time.UTC),
				UpdatedAt:               time.Date(2026, 2, 20, 18, 0, 0, 0, time.UTC),
			},
			// 4. An aircraft the pilot never added to their fleet. Dropping this
			//    row is the exact failure this export exists to prevent.
			{
				ID:               mustUUID("cccccccc-0000-4000-8000-000000000004"),
				Date:             time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				AircraftReg:      "G-OLDY",
				AircraftType:     "PA18",
				DepartureICAO:    ptr("EGTB"),
				ArrivalICAO:      ptr("EGLM"),
				OffBlockTime:     ptr("11:00:00"),
				OnBlockTime:      ptr("11:55:00"),
				TotalTime:        55,
				PICTime:          55,
				SoloTime:         55,
				IsPIC:            true,
				CrossCountryTime: 55,
				Distance:         18.5,
				TakeoffsDay:      1,
				LandingsDay:      1,
				AllLandings:      1,
				LaunchMethod:     ptr("self-launch"),
				IsFlightReview:   true,
				CreatedAt:        time.Date(2026, 3, 1, 13, 0, 0, 0, time.UTC),
				UpdatedAt:        time.Date(2026, 3, 1, 13, 0, 0, 0, time.UTC),
			},
		},

		Licenses: []LicenseWithRatings{
			{
				License: &models.License{
					ID:                  licenseID,
					RegulatoryAuthority: "EASA",
					LicenseType:         "PPL(A)",
					LicenseNumber:       "DE.FCL.12345",
					IssueDate:           time.Date(2019, 6, 1, 0, 0, 0, 0, time.UTC),
					IssuingAuthority:    "LBA",
				},
				ClassRatings: []*models.ClassRating{
					{
						ID:         mustUUID("eeeeeeee-0000-4000-8000-000000000001"),
						LicenseID:  licenseID,
						ClassType:  models.ClassTypeSEPLand,
						IssueDate:  time.Date(2019, 6, 1, 0, 0, 0, 0, time.UTC),
						ExpiryDate: ptr(time.Date(2027, 6, 30, 0, 0, 0, 0, time.UTC)),
					},
				},
			},
		},

		Credentials: []*models.Credential{
			{
				ID:               mustUUID("ffffffff-0000-4000-8000-000000000001"),
				CredentialType:   models.CredentialTypeRadioBZF1,
				CredentialNumber: ptr("BZF-I-9987"),
				IssueDate:        time.Date(2019, 3, 12, 0, 0, 0, 0, time.UTC),
				IssuingAuthority: "Bundesnetzagentur",
			},
		},

		Contacts: []*models.Contact{
			{
				ID:    mustUUID("11111111-0000-4000-8000-000000000001"),
				Name:  "Karl Fluglehrer",
				Email: ptr("karl@example.test"),
			},
		},

		Baseline: &models.FlightBaseline{
			BaselineDate: time.Date(2018, 12, 31, 0, 0, 0, 0, time.UTC),
			TotalFlights: 210,
			TotalMinutes: 18600,
			PICMinutes:   9000,
			DualMinutes:  6000,
			NightMinutes: 1200,
			LandingsDay:  260,
			Notes:        ptr("Paper logbook volumes I and II"),
		},
	}
}
