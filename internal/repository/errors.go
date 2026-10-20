package repository

import (
	"errors"
)

var (
	// ErrNotFound is returned when a resource is not found
	ErrNotFound = errors.New("not found")

	// ErrDuplicateEmail is returned when attempting to create a user with an email that already exists
	ErrDuplicateEmail = errors.New("email already exists")

	// ErrDuplicateRegistration is returned when a user already has an aircraft with the same registration
	ErrDuplicateRegistration = errors.New("aircraft registration already exists")

	// ErrDuplicate is returned when a uniqueness constraint is violated
	ErrDuplicate = errors.New("duplicate resource")

	// ErrCrewWrite wraps the error of a failed crew insert or replace that
	// happened inside a flight write; the flight write was rolled back.
	ErrCrewWrite = errors.New("crew write failed")

	// ErrDocumentFileLimit is returned when an image would push a licence or
	// credential past its per-document image cap.
	ErrDocumentFileLimit = errors.New("document image limit reached")
)

// CrewWriteError carries the cause of a failed crew write inside a flight
// write. It matches ErrCrewWrite and its cause.
type CrewWriteError struct{ Err error }

func (e *CrewWriteError) Error() string { return ErrCrewWrite.Error() + ": " + e.Err.Error() }

// Unwrap returns ErrCrewWrite and the cause.
func (e *CrewWriteError) Unwrap() []error { return []error{ErrCrewWrite, e.Err} }
