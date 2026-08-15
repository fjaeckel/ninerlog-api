// Package portability turns a pilot's NinerLog account into files that other
// logbook products — and any future tool — can read.
//
// A pilot logbook is a legal record that spans a career. Somebody who logs
// twenty years of flying here must be able to walk away with all of it, in a
// shape the next product actually ingests, without hand-mapping columns in a
// spreadsheet. That is the whole purpose of this package: leaving is a
// first-class, supported operation, not an afterthought.
//
// Two kinds of output live here:
//
//   - Vendor targets (ForeFlight, LogTen Pro, MyFlightbook, CrewLounge
//     PILOTLOG) — a single CSV laid out the way that product's importer
//     expects, so the pilot uploads one file and is done. Each layout is one
//     table of columns in its own file, so a vendor changing their template is
//     a small, reviewable diff.
//
//   - The open archive (see archive.go) — a documented, versioned ZIP holding
//     every portable entity, including the ones no vendor models. It is the
//     guarantee that outlives any individual importer.
//
// Layering: this package holds the format logic and never imports Gin. It
// composes the per-resource services the same way cloudbackup.DefaultJSONBuilder
// does, so background jobs can reuse it.
package portability

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

// LicenseWithRatings pairs a license with the class ratings attached to it.
type LicenseWithRatings struct {
	License      *models.License
	ClassRatings []*models.ClassRating
}

// Bundle is everything portable about one pilot, gathered once and then
// rendered into whatever format the caller asked for.
//
// Every writer in this package takes a *Bundle, so adding a target never means
// touching the data-gathering path, and gathering never has to know which
// format it is feeding.
type Bundle struct {
	// ExportedAt stamps the output. Callers that need reproducible bytes
	// (tests, fingerprinting) set it explicitly.
	ExportedAt time.Time

	// PilotName is the account holder's name. Several formats need it to
	// resolve "self" — EASA logbooks name the PIC even when it is the pilot.
	PilotName  string
	PilotEmail string

	Flights     []*models.Flight
	Aircraft    []*models.Aircraft
	Licenses    []LicenseWithRatings
	Credentials []*models.Credential
	Contacts    []*models.Contact
	Signatures  []SignatureRecord

	// Baseline is the pre-NinerLog hours snapshot, or nil. It is not a flight,
	// so no vendor CSV can carry it; the open archive exports it explicitly so
	// the opening balance is not silently lost on the way out.
	Baseline *models.FlightBaseline
}

// SignatureRecord is one instructor sign-off, flattened for export. The
// signature image itself is exported as a separate file in the open archive
// and referenced here by name.
type SignatureRecord struct {
	Signature *models.FlightSignature
	// Flight is the signed flight, for the date/registration context that
	// makes the record readable on its own.
	Flight *models.Flight
	// ImageFilename is the archive-relative path of the signature image, or ""
	// when the signature carries no image.
	ImageFilename string
}

// FlightService is the slice of *service.FlightService this package needs.
// Declaring it as an interface keeps the writers unit-testable without a
// database.
type FlightService interface {
	ListFlights(ctx context.Context, userID uuid.UUID, opts *repository.FlightQueryOptions) ([]*models.Flight, error)
	GetBaseline(ctx context.Context, userID uuid.UUID) (*models.FlightBaseline, error)
}

// Gatherer assembles a Bundle from the per-resource services.
//
// Every field except Flights is optional: a deployment that has an entity
// disabled, or a test that only cares about one format, can leave the
// corresponding service nil and the bundle simply carries no rows for it.
type Gatherer struct {
	Flights     FlightService
	Aircraft    *service.AircraftService
	Licenses    *service.LicenseService
	ClassRating *service.ClassRatingService
	Credentials *service.CredentialService
	Contacts    *service.ContactService
	Signatures  *service.FlightSignatureService
	Users       *service.AuthService

	// AttachCrew populates Flight.CrewMembers from the crew table. The EASA
	// PIC-name rule resolves the instructor through it, so exports that name
	// the PIC are wrong without it. Optional.
	AttachCrew func(ctx context.Context, flights []*models.Flight)

	// Now supplies ExportedAt. Defaults to time.Now().UTC.
	Now func() time.Time
}

// Gather loads every portable entity for one user.
//
// Ownership: every underlying service scopes its query by userID, so a Bundle
// can only ever contain the caller's own data. Nothing here takes an ID from
// the request.
func (g *Gatherer) Gather(ctx context.Context, userID uuid.UUID) (*Bundle, error) {
	b := &Bundle{ExportedAt: g.now()}

	if g.Users != nil {
		if user, err := g.Users.GetUserByID(ctx, userID); err == nil && user != nil {
			b.PilotName = user.Name
			b.PilotEmail = user.Email
		}
	}

	if g.Flights != nil {
		flights, err := g.Flights.ListFlights(ctx, userID, nil)
		if err != nil {
			return nil, fmt.Errorf("list flights: %w", err)
		}
		if g.AttachCrew != nil {
			g.AttachCrew(ctx, flights)
		}
		SortFlightsChronological(flights)
		b.Flights = flights

		if baseline, err := g.Flights.GetBaseline(ctx, userID); err == nil {
			b.Baseline = baseline
		}
	}

	if g.Aircraft != nil {
		aircraft, err := g.Aircraft.ListAircraft(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("list aircraft: %w", err)
		}
		sort.SliceStable(aircraft, func(i, j int) bool {
			return aircraft[i].Registration < aircraft[j].Registration
		})
		b.Aircraft = aircraft
	}

	if g.Licenses != nil {
		licenses, err := g.Licenses.ListLicenses(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("list licenses: %w", err)
		}
		sort.SliceStable(licenses, func(i, j int) bool {
			return licenses[i].ID.String() < licenses[j].ID.String()
		})
		for _, lic := range licenses {
			entry := LicenseWithRatings{License: lic}
			if g.ClassRating != nil {
				ratings, rerr := g.ClassRating.ListClassRatings(ctx, lic.ID, userID)
				if rerr != nil {
					return nil, fmt.Errorf("list class ratings: %w", rerr)
				}
				sort.SliceStable(ratings, func(i, j int) bool {
					return ratings[i].ID.String() < ratings[j].ID.String()
				})
				entry.ClassRatings = ratings
			}
			b.Licenses = append(b.Licenses, entry)
		}
	}

	if g.Credentials != nil {
		credentials, err := g.Credentials.ListCredentials(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("list credentials: %w", err)
		}
		sort.SliceStable(credentials, func(i, j int) bool {
			return credentials[i].ID.String() < credentials[j].ID.String()
		})
		b.Credentials = credentials
	}

	if g.Contacts != nil {
		contacts, err := g.Contacts.ListContacts(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("list contacts: %w", err)
		}
		sort.SliceStable(contacts, func(i, j int) bool {
			if contacts[i].Name != contacts[j].Name {
				return contacts[i].Name < contacts[j].Name
			}
			return contacts[i].ID.String() < contacts[j].ID.String()
		})
		b.Contacts = contacts
	}

	if g.Signatures != nil {
		b.Signatures = g.gatherSignatures(ctx, userID, b.Flights)
	}

	return b, nil
}

// gatherSignatures walks the user's flights and collects their sign-off
// records. A signature belongs to a flight, so there is no account-wide list
// query; iterating flights is the only way to reach them.
//
// A failure on one flight is skipped rather than failing the export: a pilot
// leaving must get their flights out even if one signature row is unreadable.
func (g *Gatherer) gatherSignatures(ctx context.Context, userID uuid.UUID, flights []*models.Flight) []SignatureRecord {
	var out []SignatureRecord
	for _, f := range flights {
		sigs, err := g.Signatures.List(ctx, f.ID, userID)
		if err != nil {
			continue
		}
		for _, sig := range sigs {
			rec := SignatureRecord{Signature: sig, Flight: f}
			if len(sig.SignatureImage) > 0 {
				rec.ImageFilename = fmt.Sprintf("signatures/%s.png", sig.ID)
			}
			out = append(out, rec)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Signature.ID.String() < out[j].Signature.ID.String()
	})
	return out
}

func (g *Gatherer) now() time.Time {
	if g.Now == nil {
		return time.Now().UTC()
	}
	return g.Now().UTC()
}

// SortFlightsChronological orders flights by date, then off-block/departure
// time, then ID. Every export in this package uses this one total ordering, so
// two exports of the same data always produce the same row order.
func SortFlightsChronological(flights []*models.Flight) {
	sort.SliceStable(flights, func(i, j int) bool {
		a, b := flights[i], flights[j]
		if !a.Date.Equal(b.Date) {
			return a.Date.Before(b.Date)
		}
		ta, tb := chronoTime(a), chronoTime(b)
		if ta != tb {
			return ta < tb
		}
		return a.ID.String() < b.ID.String()
	})
}

func chronoTime(f *models.Flight) string {
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
