package models

import (
	"errors"
	"fmt"
)

// ErrFieldTooLong is returned when a text field exceeds its maximum allowed length.
var ErrFieldTooLong = errors.New("field exceeds maximum length")

// ValidateStringLength checks that a string field does not exceed the given max length.
func ValidateStringLength(fieldName, value string, maxLen int) error {
	if len(value) > maxLen {
		return fmt.Errorf("%w: %s (max %d characters)", ErrFieldTooLong, fieldName, maxLen)
	}
	return nil
}

// ValidateOptionalStringLength checks an optional string field.
func ValidateOptionalStringLength(fieldName string, value *string, maxLen int) error {
	if value != nil {
		return ValidateStringLength(fieldName, *value, maxLen)
	}
	return nil
}

// ValidateFlightTextFields validates all text field lengths on a Flight.
func ValidateFlightTextFields(f *Flight) error {
	checks := []error{
		ValidateStringLength("aircraftReg", f.AircraftReg, 20),
		ValidateStringLength("aircraftType", f.AircraftType, 20),
		ValidateOptionalStringLength("departureICAO", f.DepartureICAO, 4),
		ValidateOptionalStringLength("arrivalICAO", f.ArrivalICAO, 4),
		ValidateOptionalStringLength("offBlockTime", f.OffBlockTime, 8),
		ValidateOptionalStringLength("onBlockTime", f.OnBlockTime, 8),
		ValidateOptionalStringLength("departureTime", f.DepartureTime, 8),
		ValidateOptionalStringLength("arrivalTime", f.ArrivalTime, 8),
		ValidateOptionalStringLength("route", f.Route, 500),
		ValidateOptionalStringLength("instructorName", f.InstructorName, 100),
		ValidateOptionalStringLength("instructorComments", f.InstructorComments, 1000),
		ValidateOptionalStringLength("launchMethod", f.LaunchMethod, 20),
		ValidateOptionalStringLength("remarks", f.Remarks, 2000),
		ValidateOptionalStringLength("picName", f.PICName, 255),
		ValidateOptionalStringLength("fstdType", f.FSTDType, 50),
		ValidateOptionalStringLength("endorsements", f.Endorsements, 2000),
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	// Validate approach entries
	for _, a := range f.Approaches {
		if !ValidApproachTypes[a.Type] {
			return fmt.Errorf("invalid approach type: %s", a.Type)
		}
	}
	return nil
}

// ValidateAircraftTextFields validates all text field lengths on an Aircraft.
func ValidateAircraftTextFields(a *Aircraft) error {
	checks := []error{
		ValidateStringLength("registration", a.Registration, 20),
		ValidateStringLength("type", a.Type, 20),
		ValidateStringLength("make", a.Make, 50),
		ValidateStringLength("model", a.Model, 50),
		ValidateOptionalStringLength("notes", a.Notes, 1000),
		ValidateOptionalStringLength("aircraftClass", a.AircraftClass, 30),
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	return nil
}

// ValidateCredentialTextFields validates all text field lengths on a Credential.
func ValidateCredentialTextFields(cr *Credential) error {
	checks := []error{
		ValidateStringLength("credentialType", string(cr.CredentialType), 50),
		ValidateOptionalStringLength("credentialNumber", cr.CredentialNumber, 50),
		ValidateStringLength("issuingAuthority", cr.IssuingAuthority, 100),
		ValidateOptionalStringLength("notes", cr.Notes, 1000),
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	return nil
}

// ValidateLicenseTextFields validates all text field lengths on a License.
func ValidateLicenseTextFields(l *License) error {
	checks := []error{
		ValidateStringLength("regulatoryAuthority", l.RegulatoryAuthority, 30),
		ValidateStringLength("licenseType", l.LicenseType, 30),
		ValidateStringLength("licenseNumber", l.LicenseNumber, 50),
		ValidateStringLength("issuingAuthority", l.IssuingAuthority, 100),
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	return nil
}

// ValidateContactTextFields validates all text field lengths on a Contact.
func ValidateContactTextFields(ct *Contact) error {
	checks := []error{
		ValidateStringLength("name", ct.Name, 100),
		ValidateOptionalStringLength("email", ct.Email, 254),
		ValidateOptionalStringLength("phone", ct.Phone, 20),
		ValidateOptionalStringLength("notes", ct.Notes, 1000),
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	return nil
}
