package cloudbackup

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/internal/service/currency"
	"github.com/google/uuid"
)

// DefaultJSONBuilder is the production implementation of JSONBuilder. It
// composes the per-resource services to produce a stable backup payload
// matching the ExportDataJSON wire format.
//
// Stability guarantees:
//   - Top-level keys: exportedAt, version, format, flights, aircraft,
//     licenses, credentials.
//   - Flights are sorted chronologically (date, then off-block/departure).
//   - Aircraft are sorted by registration.
//   - Licenses are sorted by id; class ratings are sorted by id.
//   - Credentials are sorted by id.
//   - The exportedAt field is excluded from the SHA-256 fingerprint used for
//     "skip if unchanged".
type DefaultJSONBuilder struct {
	Flights     *service.FlightService
	Aircraft    *service.AircraftService
	Licenses    *service.LicenseService
	Credentials *service.CredentialService
	ClassRating *service.ClassRatingService
	// Contacts, CustomCurrency and Notifications back the payload sections of
	// the same name. A nil service omits its section rather than failing the
	// backup.
	Contacts       *service.ContactService
	CustomCurrency *currency.CustomService
	Notifications  *service.NotificationService
	// AttachCrew is called with the flight slice before serialisation.
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

// BuildJSON gathers the user's data, serialises it to gzipped JSON, and
// returns a reader along with metadata for the BackupRun audit log.
func (b *DefaultJSONBuilder) BuildJSON(ctx context.Context, userID uuid.UUID) (io.ReadCloser, BuildMetadata, error) {
	p, err := b.Gather(ctx, userID)
	if err != nil {
		return nil, BuildMetadata{}, err
	}
	return serialisePayload(p)
}

// Gather collects every section of a user's backup in canonical order. It is
// the single definition of what a backup contains: GET /exports/json writes
// the result directly, a cloud backup run gzips it.
func (b *DefaultJSONBuilder) Gather(ctx context.Context, userID uuid.UUID) (Payload, error) {
	now := b.now()

	flights, err := b.Flights.ListFlights(ctx, userID, nil)
	if err != nil {
		return Payload{}, fmt.Errorf("list flights: %w", err)
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
		return Payload{}, fmt.Errorf("list aircraft: %w", err)
	}
	sort.SliceStable(aircraft, func(i, j int) bool {
		return aircraft[i].Registration < aircraft[j].Registration
	})

	licenses, err := b.Licenses.ListLicenses(ctx, userID)
	if err != nil {
		return Payload{}, fmt.Errorf("list licenses: %w", err)
	}
	sort.SliceStable(licenses, func(i, j int) bool {
		return licenses[i].ID.String() < licenses[j].ID.String()
	})

	licensesWithRatings := make([]LicenseWithRatings, 0, len(licenses))
	for _, lic := range licenses {
		ratings, rerr := b.ClassRating.ListClassRatings(ctx, lic.ID, userID)
		if rerr != nil {
			return Payload{}, fmt.Errorf("list class ratings: %w", rerr)
		}
		sort.SliceStable(ratings, func(i, j int) bool {
			return ratings[i].ID.String() < ratings[j].ID.String()
		})
		licensesWithRatings = append(licensesWithRatings, LicenseWithRatings{
			License:      lic,
			ClassRatings: ratings,
		})
	}

	credentials, err := b.Credentials.ListCredentials(ctx, userID)
	if err != nil {
		return Payload{}, fmt.Errorf("list credentials: %w", err)
	}
	sort.SliceStable(credentials, func(i, j int) bool {
		return credentials[i].ID.String() < credentials[j].ID.String()
	})

	contacts, err := b.gatherContacts(ctx, userID)
	if err != nil {
		return Payload{}, err
	}

	rules, err := b.gatherCustomCurrencyRules(ctx, userID)
	if err != nil {
		return Payload{}, err
	}

	prefs, err := b.gatherNotificationPreferences(ctx, userID)
	if err != nil {
		return Payload{}, err
	}

	return Payload{
		ExportedAt:              now.Format(time.RFC3339),
		Version:                 b.versionOrDefault(),
		Format:                  b.formatOrDefault(),
		Flights:                 flights,
		Aircraft:                aircraft,
		Licenses:                licensesWithRatings,
		Credentials:             credentials,
		Contacts:                contacts,
		CustomCurrencyRules:     rules,
		NotificationPreferences: prefs,
		FlightBaseline:          NewFlightBaseline(b.gatherBaseline(ctx, userID)),
	}, nil
}

// gatherContacts returns the user's address book, sorted by id.
func (b *DefaultJSONBuilder) gatherContacts(ctx context.Context, userID uuid.UUID) ([]*models.Contact, error) {
	if b.Contacts == nil {
		return nil, nil
	}
	contacts, err := b.Contacts.ListContacts(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list contacts: %w", err)
	}
	sort.SliceStable(contacts, func(i, j int) bool {
		return contacts[i].ID.String() < contacts[j].ID.String()
	})
	return contacts, nil
}

// gatherCustomCurrencyRules returns the portable half of each rule, sorted by
// name so the fingerprint is stable.
func (b *DefaultJSONBuilder) gatherCustomCurrencyRules(ctx context.Context, userID uuid.UUID) ([]CustomCurrencyRule, error) {
	if b.CustomCurrency == nil {
		return nil, nil
	}
	stored, err := b.CustomCurrency.List(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list custom currency rules: %w", err)
	}
	rules := make([]CustomCurrencyRule, 0, len(stored))
	for _, s := range stored {
		rules = append(rules, NewCustomCurrencyRule(s.Rule))
	}
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Name < rules[j].Name })
	return rules, nil
}

// gatherNotificationPreferences returns the user's notification settings.
func (b *DefaultJSONBuilder) gatherNotificationPreferences(ctx context.Context, userID uuid.UUID) (*NotificationPreferences, error) {
	if b.Notifications == nil {
		return nil, nil
	}
	prefs, err := b.Notifications.GetPreferences(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get notification preferences: %w", err)
	}
	return NewNotificationPreferences(prefs), nil
}

// gatherBaseline returns the user's carried-forward hours snapshot, or nil if
// they have none. A missing baseline is the normal case, not an error.
func (b *DefaultJSONBuilder) gatherBaseline(ctx context.Context, userID uuid.UUID) *models.FlightBaseline {
	baseline, err := b.Flights.GetBaseline(ctx, userID)
	if err != nil {
		return nil
	}
	return baseline
}

// serialisePayload writes the gathered data as gzipped JSON and returns both
// the reader and the audit metadata.
func serialisePayload(p Payload) (io.ReadCloser, BuildMetadata, error) {
	// Fingerprint excludes exportedAt.
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

	now, err := time.Parse(time.RFC3339, p.ExportedAt)
	if err != nil {
		return nil, BuildMetadata{}, fmt.Errorf("parse exportedAt: %w", err)
	}
	meta := BuildMetadata{
		SHA256:          hexSum,
		SizeBytes:       int64(gzBuf.Len()),
		FlightCount:     len(p.Flights),
		AircraftCount:   len(p.Aircraft),
		LicenseCount:    len(p.Licenses),
		CredentialCount: len(p.Credentials),
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
