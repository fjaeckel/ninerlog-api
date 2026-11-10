package handlers

import "testing"

func TestForeFlightAircraftClass(t *testing.T) {
	cases := []struct {
		name, class, engine string
		want                string
	}{
		{"C172 piston", "airplane_single_engine_land", "Piston", "SEP_LAND"},
		{"floatplane piston", "airplane_single_engine_sea", "Piston", "SEP_SEA"},
		{"twin piston", "airplane_multi_engine_land", "Piston", "MEP_LAND"},
		{"twin piston sea", "airplane_multi_engine_sea", "Piston", "MEP_SEA"},
		{"no engine type", "airplane_multi_engine_land", "", "MEP_LAND"},
		{"diesel", "airplane_single_engine_land", "Diesel", "SEP_LAND"},
		{"electric", "airplane_single_engine_land", "Electric", "SEP_LAND"},
		{"PC-12 turboprop", "airplane_single_engine_land", "Turboprop", "SET_LAND"},
		{"Caravan amphib", "airplane_single_engine_sea", "Turboprop", "SET_SEA"},
		{"King Air", "airplane_multi_engine_land", "Turboprop", "MET_LAND"},
		{"Twin Otter floats", "airplane_multi_engine_sea", "Turboprop", "MET_SEA"},
		{"turbine spelling", "airplane_multi_engine_land", "Turbine", "MET_LAND"},
		{"SF50 jet", "airplane_single_engine_land", "Turbofan", "SEJ_LAND"},
		{"A320 turbofan", "airplane_multi_engine_land", "Turbofan", "MEJ_LAND"},
		{"turbojet", "airplane_multi_engine_land", "Turbojet", "MEJ_LAND"},
		{"jet", "airplane_multi_engine_land", "Jet", "MEJ_LAND"},
		{"jet on water", "airplane_multi_engine_sea", "Turbofan", "MET_SEA"},
		{"case and space", " Airplane_Multi_Engine_Land ", " TURBOPROP ", "MET_LAND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := foreFlightAircraftClass(tc.class, tc.engine)
			if got == nil || *got != tc.want {
				t.Errorf("foreFlightAircraftClass(%q, %q) = %v, want %s", tc.class, tc.engine, got, tc.want)
			}
		})
	}

	for _, class := range []string{"", "rotorcraft_helicopter", "glider", "airplane"} {
		if got := foreFlightAircraftClass(class, "Piston"); got != nil {
			t.Errorf("foreFlightAircraftClass(%q) = %q, want nil", class, *got)
		}
	}
}
