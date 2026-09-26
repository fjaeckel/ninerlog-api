package models

import "strings"

// LicenceKind is the classified kind of a free-text licence type.
type LicenceKind string

const (
	LicenceKindUnknown         LicenceKind = ""
	LicenceKindPPLA            LicenceKind = "PPL_A"
	LicenceKindLAPLA           LicenceKind = "LAPL_A"
	LicenceKindCPLA            LicenceKind = "CPL_A"
	LicenceKindATPLA           LicenceKind = "ATPL_A"
	LicenceKindMPL             LicenceKind = "MPL"
	LicenceKindSPL             LicenceKind = "SPL"
	LicenceKindLAPLS           LicenceKind = "LAPL_S"
	LicenceKindGPL             LicenceKind = "GPL"
	LicenceKindUL              LicenceKind = "UL"
	LicenceKindHelicopter      LicenceKind = "HELICOPTER"
	LicenceKindFAASport        LicenceKind = "FAA_SPORT"
	LicenceKindFAARecreational LicenceKind = "FAA_RECREATIONAL"
	LicenceKindFAAPrivate      LicenceKind = "FAA_PRIVATE"
	LicenceKindFAACommercial   LicenceKind = "FAA_COMMERCIAL"
	LicenceKindFAAATP          LicenceKind = "FAA_ATP"
	LicenceKindFAAGlider       LicenceKind = "FAA_GLIDER"
	LicenceKindIR              LicenceKind = "IR"
	LicenceKindInstructor      LicenceKind = "INSTRUCTOR"
)

// germanULLicenceAuthorities issue only ultralight licences.
var germanULLicenceAuthorities = map[string]bool{"DULV": true, "DAEC": true}

var exactLicenceKinds = map[string]LicenceKind{
	"PPL":          LicenceKindPPLA,
	"PPL(A)":       LicenceKindPPLA,
	"LAPL":         LicenceKindLAPLA,
	"LAPL(A)":      LicenceKindLAPLA,
	"CPL":          LicenceKindCPLA,
	"CPL(A)":       LicenceKindCPLA,
	"ATPL":         LicenceKindATPLA,
	"ATPL(A)":      LicenceKindATPLA,
	"MPL":          LicenceKindMPL,
	"MPL(A)":       LicenceKindMPL,
	"SPL":          LicenceKindSPL,
	"LAPL(S)":      LicenceKindLAPLS,
	"GPL":          LicenceKindGPL,
	"SPORT":        LicenceKindFAASport,
	"RECREATIONAL": LicenceKindFAARecreational,
	"PRIVATE":      LicenceKindFAAPrivate,
	"COMMERCIAL":   LicenceKindFAACommercial,
	"ATP":          LicenceKindFAAATP,
	"GLIDER":       LicenceKindFAAGlider,
	"IR":           LicenceKindIR,
	"IR(A)":        LicenceKindIR,
	"IR(H)":        LicenceKindIR,
	"EXAMINER":     LicenceKindInstructor,
	"CFI":          LicenceKindInstructor,
	"CFII":         LicenceKindInstructor,
	"MEI":          LicenceKindInstructor,
}

// instructorLicenceCodes are instructor and examiner certificate codes, bare or with a
// category suffix such as "FI(S)".
var instructorLicenceCodes = []string{"FI", "CRI", "IRI", "TRI", "SFI", "MCCI", "FE", "CRE", "IRE", "TRE"}

// ClassifyLicence classifies a free-text licence type, case- and whitespace-insensitively.
// DULV and DAeC licences are always ultralight; every other authority is classified by
// type alone. An unrecognised type returns LicenceKindUnknown.
func ClassifyLicence(licenceType, authority string) LicenceKind {
	lt := strings.ToUpper(strings.TrimSpace(licenceType))
	auth := strings.ToUpper(strings.TrimSpace(authority))

	if germanULLicenceAuthorities[auth] || isULLicenceText(lt) {
		return LicenceKindUL
	}
	if k, ok := exactLicenceKinds[lt]; ok {
		return k
	}
	for _, code := range instructorLicenceCodes {
		if lt == code || strings.HasPrefix(lt, code+"(") {
			return LicenceKindInstructor
		}
	}
	if strings.HasSuffix(lt, "(H)") {
		return LicenceKindHelicopter
	}
	return LicenceKindUnknown
}

// isULLicenceText reports whether an upper-cased licence type names an ultralight
// licence ("UL", "UL …", "UL-…", "Ultralight", "Ultraleicht").
func isULLicenceText(lt string) bool {
	return lt == "UL" || strings.HasPrefix(lt, "UL ") || strings.HasPrefix(lt, "UL-") ||
		strings.Contains(lt, "ULTRALIGHT") || strings.Contains(lt, "ULTRALEICHT")
}

// IsEASASailplane reports whether k is an SPL or LAPL(S).
func (k LicenceKind) IsEASASailplane() bool {
	return k == LicenceKindSPL || k == LicenceKindLAPLS
}

// IsAeroplane reports whether k grants aeroplane privileges.
func (k LicenceKind) IsAeroplane() bool {
	switch k {
	case LicenceKindPPLA, LicenceKindLAPLA, LicenceKindCPLA, LicenceKindATPLA, LicenceKindMPL,
		LicenceKindFAASport, LicenceKindFAARecreational, LicenceKindFAAPrivate,
		LicenceKindFAACommercial, LicenceKindFAAATP:
		return true
	}
	return false
}

// IsSailplane reports whether k grants sailplane privileges.
func (k LicenceKind) IsSailplane() bool {
	return k.IsEASASailplane() || k == LicenceKindFAAGlider
}

// IsMultiCrew reports whether k is a multi-crew aeroplane licence (ATPL, MPL).
func (k LicenceKind) IsMultiCrew() bool {
	return k == LicenceKindATPLA || k == LicenceKindMPL
}
