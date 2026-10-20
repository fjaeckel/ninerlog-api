package importtemplate

import "strings"

// launchMethodAliases maps a lower-cased launch-type cell to a flight launch
// method: NinerLog's own values, the Vereinsflieger "S.-Art" codes and the
// German words for each method.
var launchMethodAliases = map[string]string{
	"winch":       "winch",
	"aerotow":     "aerotow",
	"self-launch": "self-launch",
	"car":         "car",
	"bungee":      "bungee",

	"w": "winch",
	"f": "aerotow",
	"e": "self-launch",
	"a": "car",
	"g": "bungee",

	"winde":           "winch",
	"windenstart":     "winch",
	"f-schlepp":       "aerotow",
	"flugzeugschlepp": "aerotow",
	"eigenstart":      "self-launch",
	"autoschlepp":     "car",
	"gummiseil":       "bungee",
	"gummiseilstart":  "bungee",
}

// ParseLaunchMethod returns the flight launch method for a launch-type cell,
// case-insensitive and trimmed, or "" when the value is not recognised.
func ParseLaunchMethod(val string) string {
	return launchMethodAliases[strings.ToLower(strings.TrimSpace(val))]
}

// foreFlightClasses maps a ForeFlight Aircraft Table "Class" value to a
// NinerLog aircraft class.
var foreFlightClasses = map[string]string{
	"airplane_single_engine_land": "SEP_LAND",
	"airplane_single_engine_sea":  "SEP_SEA",
	"airplane_multi_engine_land":  "MEP_LAND",
	"airplane_multi_engine_sea":   "MEP_SEA",
	"glider":                      "GLIDER",
	"rotorcraft_gyroplane":        "GYROPLANE",
}

// ForeFlightAircraftClass returns the aircraft class for a ForeFlight "Class"
// value, case-insensitive and trimmed, or "" when it has no NinerLog class.
func ForeFlightAircraftClass(val string) string {
	return foreFlightClasses[strings.ToLower(strings.TrimSpace(val))]
}
