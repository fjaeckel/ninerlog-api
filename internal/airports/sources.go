package airports

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Upstream datasets. Both are fetched on every reload and merged; see
// mergeSources for how a record is picked when both describe the same ICAO.
const (
	// sourceOurAirports is the OurAirports CSV export.
	sourceOurAirports = "ourairports"
	// sourceMWGG is the mwgg/Airports JSON dataset.
	sourceMWGG = "mwgg"
	// sourceMerged marks a record assembled from both datasets.
	sourceMerged = "merged"

	ourAirportsDefaultURL = "https://davidmegginson.github.io/ourairports-data/airports.csv"
	mwggDefaultURL        = "https://raw.githubusercontent.com/mwgg/Airports/master/airports.json"
)

// Source URLs, overridden in tests.
var (
	ourAirportsURL = ourAirportsDefaultURL
	mwggURL        = mwggDefaultURL
)

// httpClient is shared by both sources; the timeout covers the whole body.
var httpClient = &http.Client{Timeout: 90 * time.Second}

// maxSourceBytes caps how much is read from one upstream.
const maxSourceBytes = 128 << 20 // 128 MB

// fetchError carries the failure reason used as a metric label.
type fetchError struct {
	reason string
	err    error
}

func (e *fetchError) Error() string { return fmt.Sprintf("%s: %v", e.reason, e.err) }
func (e *fetchError) Unwrap() error { return e.err }

func failure(reason string, format string, args ...any) *fetchError {
	return &fetchError{reason: reason, err: fmt.Errorf(format, args...)}
}

// reasonOf returns the metric label for a fetch failure.
func reasonOf(err error) string {
	var fe *fetchError
	if errors.As(err, &fe) {
		return fe.reason
	}
	return "error"
}

// countingReader counts bytes read from an upstream.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// get issues the request and returns a byte-counting, size-capped body.
func get(ctx context.Context, url string) (*http.Response, *countingReader, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, failure("request", "building request: %w", err)
	}
	req.Header.Set("User-Agent", "ninerlog-api/airports")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, failure("request", "HTTP GET failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, nil, failure("status", "unexpected status %d", resp.StatusCode)
	}
	return resp, &countingReader{r: io.LimitReader(resp.Body, maxSourceBytes)}, nil
}

// fetchOurAirports downloads the OurAirports CSV and parses it into a map
// keyed by ICAO code.
//
// CSV columns: id, ident, type, name, latitude_deg, longitude_deg,
// elevation_ft, continent, iso_country, iso_region, municipality,
// scheduled_service, gps_code, iata_code, local_code, home_link,
// wikipedia_link, keywords
func fetchOurAirports(ctx context.Context) (map[string]AirportInfo, int64, error) {
	resp, body, err := get(ctx, ourAirportsURL)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	reader := csv.NewReader(body)
	reader.LazyQuotes = true
	reader.ReuseRecord = true

	header, err := reader.Read()
	if err != nil {
		return nil, body.n, failure("decode", "failed to read CSV header: %w", err)
	}

	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[col] = i
	}

	required := []string{"ident", "type", "name", "latitude_deg", "longitude_deg", "iso_country"}
	for _, col := range required {
		if _, ok := colIdx[col]; !ok {
			return nil, body.n, failure("decode", "missing required column: %s", col)
		}
	}

	field := func(record []string, col string) string {
		idx, ok := colIdx[col]
		if !ok || idx >= len(record) {
			return ""
		}
		return record[idx]
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

		ident := field(record, "ident")
		apType := field(record, "type")

		// Keep only 4-char ICAO identifiers; drop heliports and closed fields.
		if len(ident) != 4 {
			continue
		}
		if apType == "heliport" || apType == "closed" {
			continue
		}

		lat, err := strconv.ParseFloat(field(record, "latitude_deg"), 64)
		if err != nil {
			continue
		}
		lng, err := strconv.ParseFloat(field(record, "longitude_deg"), 64)
		if err != nil {
			continue
		}

		var elev int
		if v := field(record, "elevation_ft"); v != "" {
			if e, err := strconv.Atoi(v); err == nil {
				elev = e
			}
		}

		result[ident] = AirportInfo{
			ICAO:      ident,
			Name:      field(record, "name"),
			Latitude:  lat,
			Longitude: lng,
			Elevation: elev,
			Country:   field(record, "iso_country"),
			IATA:      field(record, "iata_code"),
			City:      field(record, "municipality"),
			Source:    sourceOurAirports,
		}
	}

	if len(result) == 0 {
		return nil, body.n, failure("empty", "parsed 0 airports from CSV")
	}
	return result, body.n, nil
}

// mwggAirport is one entry of the mwgg/Airports JSON object, which is keyed
// by ICAO code.
type mwggAirport struct {
	ICAO      string  `json:"icao"`
	IATA      string  `json:"iata"`
	Name      string  `json:"name"`
	City      string  `json:"city"`
	State     string  `json:"state"`
	Country   string  `json:"country"`
	Elevation int     `json:"elevation"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	TZ        string  `json:"tz"`
}

// fetchMWGG downloads the mwgg/Airports JSON dataset and parses it into a map
// keyed by ICAO code.
func fetchMWGG(ctx context.Context) (map[string]AirportInfo, int64, error) {
	resp, body, err := get(ctx, mwggURL)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	var raw map[string]mwggAirport
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, body.n, failure("decode", "failed to decode JSON: %w", err)
	}

	result := make(map[string]AirportInfo, len(raw))
	for key, a := range raw {
		icao := a.ICAO
		if icao == "" {
			icao = key
		}
		if len(icao) != 4 {
			continue
		}
		result[icao] = AirportInfo{
			ICAO:      icao,
			Name:      a.Name,
			Latitude:  a.Lat,
			Longitude: a.Lon,
			Elevation: a.Elevation,
			Country:   a.Country,
			IATA:      a.IATA,
			City:      a.City,
			Timezone:  a.TZ,
			Source:    sourceMWGG,
		}
	}

	if len(result) == 0 {
		return nil, body.n, failure("empty", "parsed 0 airports from JSON")
	}
	return result, body.n, nil
}
