package airports

import (
	"strings"
	"time"
)

// SetTestDB replaces the airport database in tests. Passing nil clears it.
func SetTestDB(data map[string]AirportInfo) {
	if data == nil {
		current.Store(nil)
		return
	}
	normalized := make(map[string]AirportInfo, len(data))
	for code, a := range data {
		if a.ICAO == "" {
			a.ICAO = code
		}
		a.ICAO = strings.ToUpper(a.ICAO)
		normalized[a.ICAO] = a
	}
	current.Store(newSnapshot(normalized, time.Now()))
}
