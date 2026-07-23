package airports

import (
	"encoding/csv"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const ourAirportsURL = "https://davidmegginson.github.io/ourairports-data/airports.csv"

// airportsURL can be overridden in tests
var airportsURL = ourAirportsURL

// AirportInfo holds metadata about an airport
type AirportInfo struct {
	ICAO      string
	Name      string
	Latitude  float64
	Longitude float64
	Elevation int
	Country   string
}

var (
	db   map[string]AirportInfo
	once sync.Once
)

// Init downloads and caches OurAirports data. Called once on startup.
func Init() {
	once.Do(func() {
		start := time.Now()
		slog.Info("Loading airport database from OurAirports...")
		loaded, err := fetchAirports()
		if err != nil {
			slog.Warn("Failed to load OurAirports data; airport lookup will be unavailable", "error", err)
			db = make(map[string]AirportInfo)
			return
		}
		db = loaded
		slog.Info("Loaded airport database", "count", len(db), "duration", time.Since(start).Round(time.Millisecond).String())
	})
}

// Lookup returns airport info by ICAO code, or nil if not found
func Lookup(icao string) *AirportInfo {
	if db == nil {
		return nil
	}
	code := strings.ToUpper(icao)
	if a, ok := db[code]; ok {
		return &a
	}
	return nil
}

// Count returns the number of airports in the database
func Count() int {
	if db == nil {
		return 0
	}
	return len(db)
}

// Search returns airports matching a prefix (case-insensitive)
func Search(prefix string, limit int) []AirportInfo {
	if db == nil || prefix == "" {
		return nil
	}
	var results []AirportInfo
	upper := strings.ToUpper(prefix)
	for code, a := range db {
		if len(results) >= limit {
			break
		}
		if strings.HasPrefix(code, upper) {
			results = append(results, a)
		}
	}
	return results
}

// maxNearestDistanceNM bounds Nearest lookups: a fix further than this from
// every known airport (mid-ocean coordinates, bogus GPS data) returns nil
// rather than a misleading "nearest" hundreds of miles away.
const maxNearestDistanceNM = 30.0

// Nearest returns the airport closest to the given coordinates, or nil when
// the database is unavailable or no airport lies within 30 NM. Used to
// resolve a phone's GPS fix to a departure/arrival airport for tap-to-log.
func Nearest(lat, lon float64) *AirportInfo {
	if db == nil {
		return nil
	}
	var best *AirportInfo
	bestDist := maxNearestDistanceNM
	for code := range db {
		a := db[code]
		d := haversineNM(lat, lon, a.Latitude, a.Longitude)
		if d <= bestDist {
			bestDist = d
			best = &a
		}
	}
	return best
}

// haversineNM returns the great-circle distance between two points in
// nautical miles.
func haversineNM(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusNM = 3440.065
	dLat := degToRad(lat2 - lat1)
	dLon := degToRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(degToRad(lat1))*math.Cos(degToRad(lat2))*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return earthRadiusNM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func degToRad(d float64) float64 {
	return d * math.Pi / 180
}

// fetchAirports downloads the OurAirports CSV and parses it into a map.
// CSV columns: id, ident, type, name, latitude_deg, longitude_deg, elevation_ft,
// continent, iso_country, iso_region, municipality, scheduled_service,
// gps_code, iata_code, local_code, home_link, wikipedia_link, keywords
func fetchAirports() (map[string]AirportInfo, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(airportsURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	reader := csv.NewReader(resp.Body)
	reader.LazyQuotes = true

	// Read header to find column indices
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[col] = i
	}

	// Validate required columns exist
	required := []string{"ident", "type", "name", "latitude_deg", "longitude_deg", "iso_country"}
	for _, col := range required {
		if _, ok := colIdx[col]; !ok {
			return nil, fmt.Errorf("missing required column: %s", col)
		}
	}

	result := make(map[string]AirportInfo, 30000)

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // skip malformed rows
		}

		ident := record[colIdx["ident"]]
		apType := record[colIdx["type"]]
		name := record[colIdx["name"]]
		country := record[colIdx["iso_country"]]

		// Only include airports with 4-char ICAO codes and meaningful types
		if len(ident) != 4 {
			continue
		}
		// Skip heliports and closed airports for cleaner data
		if apType == "heliport" || apType == "closed" {
			continue
		}

		lat, err := strconv.ParseFloat(record[colIdx["latitude_deg"]], 64)
		if err != nil {
			continue
		}
		lng, err := strconv.ParseFloat(record[colIdx["longitude_deg"]], 64)
		if err != nil {
			continue
		}

		var elev int
		if idx, ok := colIdx["elevation_ft"]; ok && idx < len(record) && record[idx] != "" {
			if e, err := strconv.Atoi(record[idx]); err == nil {
				elev = e
			}
		}

		result[ident] = AirportInfo{
			ICAO:      ident,
			Name:      name,
			Latitude:  lat,
			Longitude: lng,
			Elevation: elev,
			Country:   country,
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("parsed 0 airports from CSV")
	}

	return result, nil
}
