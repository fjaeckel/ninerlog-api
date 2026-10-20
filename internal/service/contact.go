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
	// user two contacts with the same name.
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

// WithRepository returns a service that reads and writes contacts through repo.
func (s *ContactService) WithRepository(repo repository.ContactRepository) *ContactService {
	return &ContactService{contactRepo: repo}
}

// CrewLinker resolves crew-member names to a single user's contacts across any
// number of flights, remembering the names it has already looked up. Every
// path that writes crew rows goes through it: flight create, flight update,
// spreadsheet import and backup restore.
//
// Not safe for concurrent use; take one per request.
type CrewLinker struct {
	svc    *ContactService
	userID uuid.UUID
	// cache is keyed by lower-cased name.
	cache   map[string]uuid.UUID
	created int

	pending        map[string]uuid.UUID
	pendingCreated int
}

// NewCrewLinker returns a linker for one user.
func (s *ContactService) NewCrewLinker(userID uuid.UUID) *CrewLinker {
	return &CrewLinker{svc: s, userID: userID, cache: make(map[string]uuid.UUID)}
}

// Created reports how many contacts this linker has had to create.
func (l *CrewLinker) Created() int { return l.created }

// Link resolves one flight's crew members to contacts, updating them in place.
//
// A caller-supplied ContactID is honoured only if the caller owns that
// contact; otherwise it is discarded and the name is resolved like any other.
// A member with a blank or over-length name is left unlinked; the crew row is
// still written with the name as logged. Only infrastructure failures abort.
func (l *CrewLinker) Link(ctx context.Context, members []models.FlightCrewMember) error {
	err := l.link(ctx, l.svc, members)
	l.Confirm()
	return err
}

// LinkIn is Link through repo, a repository bound to the transaction that
// writes the flight. The contacts it resolves or creates are remembered only
// after Confirm; Discard drops them.
func (l *CrewLinker) LinkIn(ctx context.Context, repo repository.ContactRepository, members []models.FlightCrewMember) error {
	return l.link(ctx, l.svc.WithRepository(repo), members)
}

// Confirm keeps what LinkIn resolved.
func (l *CrewLinker) Confirm() {
	for k, id := range l.pending {
		l.cache[k] = id
	}
	l.created += l.pendingCreated
	l.Discard()
}

// Discard drops what LinkIn resolved since the last Confirm.
func (l *CrewLinker) Discard() {
	l.pending = nil
	l.pendingCreated = 0
}

func (l *CrewLinker) link(ctx context.Context, svc *ContactService, members []models.FlightCrewMember) error {
	for i := range members {
		name := strings.TrimSpace(members[i].Name)
		members[i].Name = name
		if name == "" {
			members[i].ContactID = nil
			continue
		}

		if members[i].ContactID != nil {
			owned, err := svc.contactRepo.GetByID(ctx, *members[i].ContactID)
			if err == nil && owned.UserID == l.userID {
				continue
			}
			if err != nil && !errors.Is(err, repository.ErrNotFound) {
				return err
			}
			members[i].ContactID = nil
		}

		key := strings.ToLower(name)
		id, ok := l.pending[key]
		if !ok {
			id, ok = l.cache[key]
		}
		if !ok {
			contact, isNew, err := svc.FindOrCreateContact(ctx, l.userID, name)
			if errors.Is(err, models.ErrFieldTooLong) {
				members[i].ContactID = nil
				continue
			}
			if err != nil {
				return err
			}
			id = contact.ID
			if l.pending == nil {
				l.pending = make(map[string]uuid.UUID)
			}
			l.pending[key] = id
			if isNew {
				l.pendingCreated++
			}
		}
		linked := id
		members[i].ContactID = &linked
	}
	return nil
}

// LinkCrewMembers links a single flight's crew and reports how many contacts
// it created. Callers processing many flights should hold a CrewLinker
// instead.
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
// entries whose name was rewritten to match. Entries on flights carrying a
// completed instructor signature, and entries not linked to the contact, are
// excluded; the repository enforces that boundary in SQL.
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

	renamed, err := s.contactRepo.UpdateWithCrewRename(ctx, contact, crewRenameReason(existing.Name, contact.Name))
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

// DeleteContact removes a contact from the address book. It does not touch any
// flight: flight_crew_members.contact_id is ON DELETE SET NULL and the crew
// row keeps the name it was logged with. Deleting is always allowed, even for
// a contact referenced by signed flights.
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

// crewRenameReason is the reason recorded on flight revisions written by a
// contact rename.
func crewRenameReason(oldName, newName string) string {
	if oldName == newName {
		return "Crew names updated to contact: " + newName
	}
	return "Crew contact renamed: " + oldName + " \u2192 " + newName
}
