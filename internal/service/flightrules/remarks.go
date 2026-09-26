package flightrules

import (
	"regexp"
	"strings"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/pkg/duration"
)

// RemarkFlag is an optional inline marker appended to a flight's remarks
// when present (FAA logbook convention). Order matches FAA convention:
// [IPC], then [FR] (Flight Review), then [PC] (Proficiency Check).
type RemarkFlag int

const (
	FlagIPC RemarkFlag = iota
	FlagFlightReview
	FlagProficiencyCheck
)

// CombinedRemarks returns the user-facing combined Remarks + Endorsements
// string, suffixed with the declared-function-time annotations the paper
// layouts require ([PICUS h:mm] / [SPIC h:mm] / [Examiner h:mm] /
// [Relief h:mm] — the PIC and co-pilot columns fold these times in, per
// PICColumnTime and CoPilotColumnTime), then the launch method
// ([Launch: winch], see LaunchRemark), and optionally with FAA-style inline
// flags ([IPC] / [FR] / [PC]). Empty endorsements/remarks are skipped; the
// separator between a non-empty remark and a non-empty endorsement is " | ".
//
// Flag suffixes are only added when the corresponding boolean on `flight`
// is true AND the flag was requested by the caller. Most callers pass all
// three flags and let the model state decide.
func CombinedRemarks(f *models.Flight, flags ...RemarkFlag) string {
	if f == nil {
		return ""
	}
	out := ""
	if f.Remarks != nil {
		out = *f.Remarks
	}
	if f.Endorsements != nil && *f.Endorsements != "" {
		if out != "" {
			out += " | "
		}
		out += *f.Endorsements
	}
	addTime := func(minutes int, label string) {
		if minutes <= 0 {
			return
		}
		if out != "" {
			out += " "
		}
		out += "[" + label + " " + duration.FormatColonHM(minutes) + "]"
	}
	addTime(f.PICUSTime, "PICUS")
	addTime(f.SPICTime, "SPIC")
	addTime(f.ExaminerTime, "Examiner")
	addTime(f.ReliefTime, "Relief")
	if marker := LaunchRemark(f.LaunchMethod); marker != "" {
		if out != "" {
			out += " "
		}
		out += marker
	}
	addFlag := func(active bool, label string) {
		if !active {
			return
		}
		if out != "" {
			out += " "
		}
		out += label
	}
	for _, flag := range flags {
		switch flag {
		case FlagIPC:
			addFlag(f.IsIPC, "[IPC]")
		case FlagFlightReview:
			addFlag(f.IsFlightReview, "[FR]")
		case FlagProficiencyCheck:
			addFlag(f.IsProficiencyCheck, "[PC]")
		}
	}
	return strings.TrimSpace(out)
}

// launchRemarkPattern matches a LaunchRemark marker anywhere in a remarks string.
var launchRemarkPattern = regexp.MustCompile(`\s*\[Launch: ([a-z-]+)\]`)

// LaunchRemark returns the remarks marker for a launch method, "[Launch: winch]",
// or "" when method is nil or empty.
func LaunchRemark(method *string) string {
	if method == nil || strings.TrimSpace(*method) == "" {
		return ""
	}
	return "[Launch: " + strings.TrimSpace(*method) + "]"
}

// ExtractLaunchRemark removes the first LaunchRemark marker naming a valid
// launch method from remarks, returning the method and the remaining remarks.
// Without such a marker it returns "" and remarks unchanged.
func ExtractLaunchRemark(remarks string) (method, rest string) {
	for _, loc := range launchRemarkPattern.FindAllStringSubmatchIndex(remarks, -1) {
		m := remarks[loc[2]:loc[3]]
		if !models.IsValidLaunchMethod(m) {
			continue
		}
		return m, strings.TrimSpace(remarks[:loc[0]] + remarks[loc[1]:])
	}
	return "", remarks
}
