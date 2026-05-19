package cloudbackup

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

// DefaultJSONBuilder is the production implementation of JSONBuilder. It
// composes the existing per-resource services to produce a stable backup
// payload that matches the on-the-wire JSON export ExportDataJSON has emitted
// since v1.0.
//
// Stability guarantees:
//   - Top-level keys: exportedAt, version, format, flights, aircraft,
//     licenses, credentials.
//   - Flights are sorted chronologically (date, then off-block/departure).
//   - Aircraft are sorted by registration.
//   - Licenses are sorted by id; class ratings are sorted by id.
//   - Credentials are sorted by id.
//   - The exportedAt field is excluded from the SHA-256 fingerprint used for
//     "skip if unchanged" so a re-run on an otherwise-identical dataset is
//     correctly identified as a no-op.
type DefaultJSONBuilder struct {
	Flights     *service.FlightService
	Aircraft    *service.AircraftService
	Licenses    *service.LicenseService
	Credentials *service.CredentialService
	ClassRating *service.ClassRatingService
	// AttachCrew is called with the flight slice before serialisation so the
	// crew-table fallback that the export pathway relies on still fires.
	// Optional.
	AttachCrew func(ctx context.Context, flights []*models.Flight)
	// SortFlights is called with the flight slice before serialisation.
	// Optional; defaults to chronological order (date, off-block, id).
	SortFlights func(flights []*models.Flight)
	// Version is embedded in the payload. Defaults to "1.0".
	Version string
	// Format is embedded in the payload. Defaults to "NinerLog JSON Backup".
	Format string
	// Now returns the timestamp used for exportedAt and the filename.
	// Defaults to time.Now().UTC.
	Now func() time.Time
}

type licenseWithRatings struct {
	License      *models.License       `json:"license"`
	ClassRatings []*models.ClassRating `json:"classRatings"`
}

// payload is the wire layout of one backup. Field order matches the legacy
// ExportDataJSON output.
type payload struct {
	ExportedAt  string                `json:"exportedAt"`
	Version     string                `json:"version"`
	Format      string                `json:"format"`
	Flights     []*models.Flight      `json:"flights"`
	Aircraft    []*models.Aircraft    `json:"aircraft"`
	Licenses    []licenseWithRatings  `json:"licenses"`
	Credentials []*models.Credential  `json:"credentials"`
}

// BuildJSON gathers the user's data, serialises it to gzipped JSON, and
// returns a reader along with metadata for the BackupRun audit log.
func (b *DefaultJSONBuilder) BuildJSON(ctx context.Context, userID uuid.UUID) (io.ReadCloser, BuildMetadata, error) {
	now := b.now()

	flights, err := b.Flights.ListFlights(ctx, userID, nil)
	if err != nil {
		return nil, BuildMetadata{}, fmt.Errorf("list flights: %w", err)
	}
	if b.AttachCrew != nil {
		b.AttachCrew(ctx, flights)
	}
	if b.SortFlights != nil {
		b.SortFlights(flights)
	} else {
		sortFlightsChronological(flights)
	}

	aircraft, err := b.Aircraft.ListAircraft(ctx, userID)
	if err != nil {
		return nil, BuildMetadata{}, fmt.Errorf("list aircraft: %w", err)
	}
	sort.SliceStable(aircraft, func(i, j int) bool {
		return aircraft[i].Registration < aircraft[j].Registration
	})

	licenses, err := b.Licenses.ListLicenses(ctx, userID)
	if err != nil {
		return nil, BuildMetadata{}, fmt.Errorf("list licenses: %w", err)
	}
	sort.SliceStable(licenses, func(i, j int) bool {
		return licenses[i].ID.String() < licenses[j].ID.String()
	})

	licensesWithRatings := make([]licenseWithRatings, 0, len(licenses))
	for _, lic := range licenses {
		ratings, rerr := b.ClassRating.ListClassRatings(ctx, lic.ID, userID)
		if rerr != nil {
			return nil, BuildMetadata{}, fmt.Errorf("list class ratings: %w", rerr)
		}
		sort.SliceStable(ratings, func(i, j int) bool {
			return ratings[i].ID.String() < ratings[j].ID.String()
		})
		licensesWithRatings = append(licensesWithRatings, licenseWithRatings{
			License:      lic,
			ClassRatings: ratings,
		})
	}

	credentials, err := b.Credentials.ListCredentials(ctx, userID)
	if err != nil {
		return nil, BuildMetadata{}, fmt.Errorf("list credentials: %w", err)
	}
	sort.SliceStable(credentials, func(i, j int) bool {
		return credentials[i].ID.String() < credentials[j].ID.String()
	})

	return buildPayload(now, b.versionOrDefault(), b.formatOrDefault(), flights, aircraft, licensesWithRatings, credentials, len(licenses))
}

// buildPayload serialises the gathered data into the canonical gzipped JSON
// shape and returns both the reader and the audit metadata. It is split out
// from BuildJSON so unit tests can exercise the deterministic serialisation
// without spinning up a full database.
func buildPayload(
	now time.Time,
	version string,
	format string,
	flights []*models.Flight,
	aircraft []*models.Aircraft,
	licenses []licenseWithRatings,
	credentials []*models.Credential,
	licenseCount int,
) (io.ReadCloser, BuildMetadata, error) {
	p := payload{
		ExportedAt:  now.Format(time.RFC3339),
		Version:     version,
		Format:      format,
		Flights:     flights,
		Aircraft:    aircraft,
		Licenses:    licenses,
		Credentials: credentials,
	}

	// Compute a stable fingerprint that excludes exportedAt so identical data
	// at different times produces the same hash.
	fp := p
	fp.ExportedAt = ""
	fpBytes, err := json.Marshal(fp)
	if err != nil {
		return nil, BuildMetadata{}, fmt.Errorf("marshal fingerprint: %w", err)
	}
	sum := sha256.Sum256(fpBytes)
	hexSum := hex.EncodeToString(sum[:])

	// Marshal the real payload (pretty-printed, matching ExportDataJSON's
	// encoder.SetIndent("", "  ")) and gzip it.
	jsonBuf := &bytes.Buffer{}
	enc := json.NewEncoder(jsonBuf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(p); err != nil {
		return nil, BuildMetadata{}, fmt.Errorf("encode payload: %w", err)
	}

	gzBuf := &bytes.Buffer{}
	gz := gzip.NewWriter(gzBuf)
	if _, err := gz.Write(jsonBuf.Bytes()); err != nil {
		return nil, BuildMetadata{}, fmt.Errorf("gzip write: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, BuildMetadata{}, fmt.Errorf("gzip close: %w", err)
	}

	meta := BuildMetadata{
		SHA256:          hexSum,
		SizeBytes:       int64(gzBuf.Len()),
		FlightCount:     len(flights),
		AircraftCount:   len(aircraft),
		LicenseCount:    licenseCount,
		CredentialCount: len(credentials),
		ContentType:     "application/gzip",
		Filename:        fmt.Sprintf("ninerlog-backup-%s.json.gz", now.UTC().Format("2006-01-02T15-04-05Z")),
	}
	return io.NopCloser(bytes.NewReader(gzBuf.Bytes())), meta, nil
}

func (b *DefaultJSONBuilder) versionOrDefault() string {
	if b.Version == "" {
		return "1.0"
	}
	return b.Version
}

func (b *DefaultJSONBuilder) formatOrDefault() string {
	if b.Format == "" {
		return "NinerLog JSON Backup"
	}
	return b.Format
}

func (b *DefaultJSONBuilder) now() time.Time {
	if b.Now == nil {
		return time.Now().UTC()
	}
	return b.Now().UTC()
}

// sortFlightsChronological orders flights by date, then off-block/departure
// time, then ID — the same total ordering ExportDataJSON uses.
func sortFlightsChronological(flights []*models.Flight) {
	sort.SliceStable(flights, func(i, j int) bool {
		a, b := flights[i], flights[j]
		if !a.Date.Equal(b.Date) {
			return a.Date.Before(b.Date)
		}
		ta, tb := flightChronoTime(a), flightChronoTime(b)
		if ta != tb {
			return ta < tb
		}
		return a.ID.String() < b.ID.String()
	})
}

// flightChronoTime returns a comparable HH:MM:SS string for intra-day
// ordering. Prefers OffBlockTime and falls back to DepartureTime.
func flightChronoTime(f *models.Flight) string {
	if f == nil {
		return ""
	}
	if f.OffBlockTime != nil && *f.OffBlockTime != "" {
		return *f.OffBlockTime
	}
	if f.DepartureTime != nil && *f.DepartureTime != "" {
		return *f.DepartureTime
	}
	return ""
}
