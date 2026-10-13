package currency

import (
	"context"
	"log/slog"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/google/uuid"
)

// SetPrivilegeSource wires the licence privileges. With none wired, results
// carry no privileges and no privilege-dependent statements.
func (s *Service) SetPrivilegeSource(l PrivilegeLister) {
	s.privileges = l
}

// privilegeData returns the privilege flight read, nil when the provider has none.
func (s *Service) privilegeData() PrivilegeDataProvider {
	pdp, _ := s.flightData.(PrivilegeDataProvider)
	return pdp
}

// loadPrivileges returns the user's privileges by licence; ok is false when
// no source is wired or the read failed.
func (s *Service) loadPrivileges(ctx context.Context, userID uuid.UUID) (map[uuid.UUID][]*models.LicencePrivilege, bool) {
	if s.privileges == nil {
		return nil, false
	}
	list, err := s.privileges.ListByUser(ctx, userID)
	if err != nil {
		slog.Error("currency: list licence privileges failed", "user_id", userID, "error", err)
		return nil, false
	}
	out := make(map[uuid.UUID][]*models.LicencePrivilege, len(list))
	for _, p := range list {
		out[p.LicenseID] = append(out[p.LicenseID], p)
	}
	return out, true
}

// activePrivilege returns the first privilege of kind (and detail, when not
// empty) that has not expired at the context's instant.
func activePrivilege(ctx context.Context, privs []*models.LicencePrivilege, kind models.LicencePrivilegeKind, detail string) *models.LicencePrivilege {
	now := nowFrom(ctx)
	for _, p := range privs {
		if p.Kind != kind || p.IsExpiredOn(now) {
			continue
		}
		if detail != "" && (p.Detail == nil || *p.Detail != detail) {
			continue
		}
		return p
	}
	return nil
}

// evaluatePrivileges evaluates every privilege, in licence order.
func (s *Service) evaluatePrivileges(ctx context.Context, licenses []*models.License, privs map[uuid.UUID][]*models.LicencePrivilege) []PrivilegeCurrency {
	pdp := s.privilegeData()
	var out []PrivilegeCurrency
	for _, lic := range licenses {
		for _, p := range privs[lic.ID] {
			out = append(out, EvaluatePrivilege(ctx, p, lic, pdp))
		}
	}
	return out
}

// applyTrainedLaunchMethods marks the SFCL.155(c) launch methods the pilot is
// trained for (SFCL.155(a)) and adds trained methods never logged, in
// launchMethods order. A self-launch row counts the TMG take-offs in the window.
func (s *Service) applyTrainedLaunchMethods(ctx context.Context, result *ClassRatingCurrency, license *models.License, privs []*models.LicencePrivilege, dp FlightDataProvider) {
	trained := map[string]bool{}
	for _, p := range privs {
		if p.Kind == models.PrivilegeLaunchMethodTrained && p.Detail != nil {
			trained[*p.Detail] = true
		}
	}
	byMethod := map[string]LaunchMethodCurrency{}
	for _, lm := range result.LaunchMethodCurrency {
		byMethod[lm.Method] = lm
	}
	var out []LaunchMethodCurrency
	for _, method := range launchMethods {
		lm, logged := byMethod[method]
		if !logged {
			if !trained[method] {
				continue
			}
			lm = LaunchMethodCurrency{Method: method, Required: launchMethodRequired(method), MessageKey: MsgLaunchMethodProgress}
			if method == models.LaunchMethodSelfLaunch {
				since := easaSPLRule.window.rollingSince(nowFrom(ctx))
				if p, err := dp.GetProgressByAircraftClass(ctx, license.UserID, []models.ClassType{models.ClassTypeTMG}, false, since); err == nil {
					lm.Launches = p.Launches
				}
			}
			lm.Met = lm.Launches >= lm.Required
			if !lm.Met {
				missing := lm.Required - lm.Launches
				m := method
				lm.RemedyKey = RemedyLaunchMethodDual
				lm.RemedyParams = &MessageParams{Method: &m, Missing: &missing}
			}
		}
		lm.Trained = trained[method]
		out = append(out, lm)
	}
	result.LaunchMethodCurrency = out
}

// applyPassengerPrivileges adds what the licence's privileges change about a
// passenger currency: SFCL.115(a)(2) prerequisites and SFCL.160(e)(2) night
// passengers with a TMG night privilege on an SPL, and the LuftPersV §84a
// passenger authorisation on a German ultralight entry.
func (s *Service) applyPassengerPrivileges(ctx context.Context, pax *PassengerCurrency, license *models.License, privs []*models.LicencePrivilege, dp FlightDataProvider) {
	pdp := s.privilegeData()
	switch {
	case pax.RuleDescriptionKey == "easa_spl_pax" || pax.RuleDescriptionKey == "easa_spl_tmg_pax":
		if pdp != nil {
			if rows, err := spl115PassengerPrerequisites(ctx, license, pdp); err == nil {
				pax.Requirements = rows
			}
		}
		if pax.RuleDescriptionKey == "easa_spl_tmg_pax" && activePrivilege(ctx, privs, models.PrivilegeTMGNight, "") != nil {
			applyTMGNightPassengers(ctx, pax, license, dp)
		}
	case pax.RuleDescriptionKey == "ul_pax" && pax.ULKind != nil:
		if activePrivilege(ctx, privs, models.PrivilegeULPassengerAuth, "") != nil {
			if pax.DayStatus == StatusCurrent {
				pax.setMsg(MsgPaxCurrentDayNoNight, nil)
			}
			return
		}
		if pdp != nil {
			if rows, err := ulPassengerAuthorisationProgress(ctx, license.UserID, *pax.ULKind, pdp); err == nil {
				pax.Requirements = rows
			}
		}
		if pax.DayStatus == StatusCurrent {
			pax.DayStatus = StatusUnknown
			pax.setMsg(MsgPaxULAuthorisationMissing, nil)
		}
	}
}

// applyTMGNightPassengers evaluates SFCL.160(e)(2) night passenger carriage in
// a TMG: one of the take-offs and landings as PIC in the preceding 90 days at night.
func applyTMGNightPassengers(ctx context.Context, pax *PassengerCurrency, license *models.License, dp FlightDataProvider) {
	pax.NightPrivilege = true
	pax.NightRequired = 1
	pax.NightExpiresOn = nil
	if pax.NightLandings >= 1 {
		pax.NightStatus = StatusCurrent
		if days, err := dp.GetLandingDaysByAircraftClass(ctx, license.UserID, models.ClassTypeTMG, true, true, paxWindowStart(nowFrom(ctx))); err == nil {
			pax.NightExpiresOn = paxExpiryString(paxExpiryDate(days, 1, nightLandings))
		}
	} else {
		pax.NightStatus = StatusExpired
	}
	if pax.DayStatus != StatusCurrent {
		return
	}
	if pax.NightStatus == StatusCurrent {
		pax.setMsg(MsgPaxCurrentDayNight, nil)
	} else {
		pax.setMsg(MsgPaxDayCurrentNightNot, msgNeeded(1))
	}
}
