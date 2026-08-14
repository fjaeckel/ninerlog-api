package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

var (
	ErrContactNotFound     = errors.New("contact not found")
	ErrUnauthorizedContact = errors.New("unauthorized access to contact")

	// ErrContactNameExists is returned when a create or rename would give the
	// user two contacts with the same name. A contact is identified by its
	// name, so returning the pre-existing row instead would hand the caller a
	// different resource than the one it asked to create; the caller is told
	// about the collision and can look the existing one up.
	ErrContactNameExists = errors.New("a contact with this name already exists")

	// ErrContactNameRequired is returned for an empty or whitespace-only name.
	ErrContactNameRequired = errors.New("contact name is required")
)

type ContactService struct {
	contactRepo repository.ContactRepository
}

func NewContactService(contactRepo repository.ContactRepository) *ContactService {
	return &ContactService{contactRepo: contactRepo}
}

func (s *ContactService) CreateContact(ctx context.Context, contact *models.Contact) error {
	contact.Name = strings.TrimSpace(contact.Name)
	if contact.Name == "" {
		return ErrContactNameRequired
	}
	if err := models.ValidateContactTextFields(contact); err != nil {
		return err
	}
	if err := s.contactRepo.Create(ctx, contact); err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			return ErrContactNameExists
		}
		return err
	}
	return nil
}

// FindOrCreateContact finds an existing contact by exact name (case-insensitive)
// or creates a new one. Returns the contact and whether it was newly created.
func (s *ContactService) FindOrCreateContact(ctx context.Context, userID uuid.UUID, name string) (*models.Contact, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false, ErrContactNameRequired
	}
	if err := models.ValidateContactTextFields(&models.Contact{Name: name}); err != nil {
		return nil, false, err
	}
	return s.contactRepo.FindOrCreateByName(ctx, userID, name)
}

// CrewLinker resolves crew-member names to a single user's contacts across any
// number of flights, remembering the names it has already looked up.
//
// Every path that writes crew rows goes through it — flight create, flight
// update, spreadsheet import and backup restore — so the address book is a
// function of the logbook rather than of which entry point the pilot happened
// to use. Before this existed, only import created contacts, and a pilot who
// logged flights in the UI ended up with an empty contact list.
//
// Not safe for concurrent use; take one per request.
type CrewLinker struct {
	svc    *ContactService
	userID uuid.UUID
	// cache is keyed by lower-cased name. An import walks hundreds of flights
	// flown with the same handful of people; without it, every crew row is a
	// database round-trip.
	cache   map[string]*models.Contact
	created int
}

// NewCrewLinker returns a linker for one user.
func (s *ContactService) NewCrewLinker(userID uuid.UUID) *CrewLinker {
	return &CrewLinker{svc: s, userID: userID, cache: make(map[string]*models.Contact)}
}

// Created reports how many contacts this linker has had to create.
func (l *CrewLinker) Created() int { return l.created }

// Link resolves one flight's crew members to contacts, updating them in place.
//
// A caller-supplied ContactID is honoured only if the caller owns that contact;
// otherwise it is discarded and the name is resolved like any other. Contacts
// are per-user, and a crew row is the only place a client can propose a contact
// id, so an unverified one would let a flight reference another user's address
// book.
//
// A member with a blank name is left unlinked: there is nobody to link. So is
// one whose name is too long to be a contact — the crew row is still written
// with the name as logged, and one unusable name does not cost the rest of the
// crew their links. Only infrastructure failures abort.
func (l *CrewLinker) Link(ctx context.Context, members []models.FlightCrewMember) error {
	for i := range members {
		name := strings.TrimSpace(members[i].Name)
		members[i].Name = name
		if name == "" {
			members[i].ContactID = nil
			continue
		}

		if members[i].ContactID != nil {
			owned, err := l.svc.contactRepo.GetByID(ctx, *members[i].ContactID)
			if err == nil && owned.UserID == l.userID {
				continue
			}
			if err != nil && !errors.Is(err, repository.ErrNotFound) {
				return err
			}
			members[i].ContactID = nil
		}

		key := strings.ToLower(name)
		contact, ok := l.cache[key]
		if !ok {
			var err error
			var isNew bool
			contact, isNew, err = l.svc.FindOrCreateContact(ctx, l.userID, name)
			if errors.Is(err, models.ErrFieldTooLong) {
				members[i].ContactID = nil
				continue
			}
			if err != nil {
				return err
			}
			l.cache[key] = contact
			if isNew {
				l.created++
			}
		}
		id := contact.ID
		members[i].ContactID = &id
	}
	return nil
}

// LinkCrewMembers links a single flight's crew and reports how many contacts it
// created. Callers processing many flights should hold a CrewLinker instead so
// the name cache survives between them.
func (s *ContactService) LinkCrewMembers(ctx context.Context, userID uuid.UUID, members []models.FlightCrewMember) (int, error) {
	l := s.NewCrewLinker(userID)
	err := l.Link(ctx, members)
	return l.Created(), err
}

func (s *ContactService) GetContact(ctx context.Context, id, userID uuid.UUID) (*models.Contact, error) {
	contact, err := s.contactRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrContactNotFound
		}
		return nil, err
	}
	if contact.UserID != userID {
		return nil, ErrUnauthorizedContact
	}
	return contact, nil
}

func (s *ContactService) ListContacts(ctx context.Context, userID uuid.UUID) ([]*models.Contact, error) {
	return s.contactRepo.GetByUserID(ctx, userID, nil)
}

// ListContactsUpdatedSince returns the user's contacts that changed strictly
// after the given instant, for delta-syncing clients.
func (s *ContactService) ListContactsUpdatedSince(ctx context.Context, userID uuid.UUID, updatedSince time.Time) ([]*models.Contact, error) {
	return s.contactRepo.GetByUserID(ctx, userID, &updatedSince)
}

func (s *ContactService) SearchContacts(ctx context.Context, userID uuid.UUID, query string, limit int) ([]*models.Contact, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	return s.contactRepo.Search(ctx, userID, query, limit)
}

// UpdateContact updates a contact and returns the number of logbook crew
// entries whose name was rewritten to match.
//
// Renaming a contact is a correction to the address book, and the crew entries
// that reference it carry a copy of the name, so the rename has to reach them
// or fixing a typo would leave the logbook still showing it. Entries on flights
// carrying a completed instructor signature are excluded: those are attested
// records, and the repository enforces that boundary in SQL.
//
// Contacts a crew entry is not linked to are unaffected, which is why the
// pre-000060 rows that were written with a NULL contact_id keep their text
// until something links them.
func (s *ContactService) UpdateContact(ctx context.Context, contact *models.Contact, userID uuid.UUID) (int, error) {
	existing, err := s.contactRepo.GetByID(ctx, contact.ID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return 0, ErrContactNotFound
		}
		return 0, err
	}
	if existing.UserID != userID {
		return 0, ErrUnauthorizedContact
	}
	contact.Name = strings.TrimSpace(contact.Name)
	if contact.Name == "" {
		return 0, ErrContactNameRequired
	}
	if err := models.ValidateContactTextFields(contact); err != nil {
		return 0, err
	}
	contact.UserID = existing.UserID

	renamed, err := s.contactRepo.UpdateWithCrewRename(ctx, contact)
	if err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			return 0, ErrContactNameExists
		}
		if errors.Is(err, repository.ErrNotFound) {
			return 0, ErrContactNotFound
		}
		return 0, err
	}
	return renamed, nil
}

// DeleteContact removes a contact from the address book. It does not remove
// the person from any flight: flight_crew_members.contact_id is ON DELETE SET
// NULL and the crew row keeps the name it was logged with, so the logbook — the
// legal record — reads exactly the same afterwards. Only the link, the email
// and the phone number are lost. Deleting is therefore always allowed, even for
// a contact referenced by signed flights.
//
// Re-logging the same person afterwards creates a fresh contact and links the
// new crew row to it; the older crew rows stay unlinked.
func (s *ContactService) DeleteContact(ctx context.Context, id, userID uuid.UUID) error {
	contact, err := s.contactRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrContactNotFound
		}
		return err
	}
	if contact.UserID != userID {
		return ErrUnauthorizedContact
	}
	return s.contactRepo.Delete(ctx, id)
}
