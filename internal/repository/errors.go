package repository

import "errors"

var (
	// ErrNotFound is returned when a resource is not found
	ErrNotFound = errors.New("not found")

	// ErrDuplicateEmail is returned when attempting to create a user with an email that already exists
	ErrDuplicateEmail = errors.New("email already exists")

	// ErrDuplicateRegistration is returned when a user already has an aircraft with the same registration
	ErrDuplicateRegistration = errors.New("aircraft registration already exists")
)
