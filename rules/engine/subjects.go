package engine

import (
	"slices"
	"strings"
)

// subjectRef is a selected subject plus the record data it refers to.
type subjectRef struct {
	Subject
	licence        *Licence
	licenceKind    string
	typeDesignator string
	variant        string
	issued         *Date
	validFrom      *Date
	expires        *Date
}

func containsFold(list []string, v string) bool {
	return slices.ContainsFunc(list, func(s string) bool { return normAuthority(s) == normAuthority(v) })
}

// licenceMatches applies the authority and licence-kind parts of applies_to.
func (p *prepared) licenceMatches(a *AppliesTo, l *Licence) bool {
	if l == nil {
		return len(a.Authorities) == 0 && len(a.LicenceKinds) == 0
	}
	if len(a.Authorities) > 0 && !containsFold(a.Authorities, l.Authority) {
		return false
	}
	if containsFold(a.ExcludeAuthorities, l.Authority) {
		return false
	}
	kind := p.licenceKind[l.ID]
	if len(a.LicenceKinds) > 0 && !slices.Contains(a.LicenceKinds, kind) {
		return false
	}
	return !slices.Contains(a.ExcludeLicenceKinds, kind)
}

func ratingMatches(a *AppliesTo, r *Rating) bool {
	if len(a.Classes) > 0 && !slices.Contains(a.Classes, r.Class) {
		return false
	}
	if slices.Contains(a.ExcludeClasses, r.Class) {
		return false
	}
	if len(a.ULKinds) > 0 {
		k := r.ULKind
		if k == "" {
			k = "none"
		}
		if !slices.Contains(a.ULKinds, k) {
			return false
		}
	}
	return true
}

func (p *prepared) ratingSubject(kind string, r *Rating) subjectRef {
	l := p.licenceOf(r.LicenceID)
	return subjectRef{
		Subject:        Subject{Kind: kind, ID: r.ID, Class: r.Class, ULKind: r.ULKind},
		licence:        l,
		licenceKind:    p.licenceKind[r.LicenceID],
		typeDesignator: r.TypeDesignator,
		issued:         r.Issued, validFrom: r.ValidFrom, expires: r.Expires,
	}
}

// matchingRatings returns the ratings whose licence and class match applies_to.
func (p *prepared) matchingRatings(a *AppliesTo) []*Rating {
	var out []*Rating
	for i := range p.ratings {
		r := &p.ratings[i]
		l := p.licenceOf(r.LicenceID)
		if l == nil || !p.licenceMatches(a, l) || !ratingMatches(a, r) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// subjects returns the subjects rule r applies to, in record order.
func (p *prepared) subjects(r *Rule, v *Vocabulary, asOf Date) []subjectRef {
	a := &r.AppliesTo
	var out []subjectRef
	switch a.Subject {
	case "rating":
		for _, rt := range p.matchingRatings(a) {
			out = append(out, p.ratingSubject("rating", rt))
		}
	case "passengers":
		seen := map[string]bool{}
		for _, rt := range p.matchingRatings(a) {
			key := normAuthority(p.licenceOf(rt.LicenceID).Authority) + "|" + rt.Class + "|" + rt.ULKind
			if seen[key] {
				continue
			}
			seen[key] = true
			s := p.ratingSubject("passengers", rt)
			s.ID = rt.LicenceID
			s.issued, s.validFrom, s.expires = nil, nil, nil
			out = append(out, s)
		}
	case "licence":
		for i := range p.rec.Licences {
			l := &p.rec.Licences[i]
			if p.licenceMatches(a, l) {
				out = append(out, subjectRef{
					Subject: Subject{Kind: "licence", ID: l.ID, Detail: p.licenceKind[l.ID]},
					licence: l, licenceKind: p.licenceKind[l.ID], issued: l.Issued, expires: l.Expires,
				})
			}
		}
	case "flight_review":
		for i := range p.rec.Licences {
			l := &p.rec.Licences[i]
			if p.licenceMatches(a, l) {
				out = append(out, subjectRef{Subject: Subject{Kind: "flight_review", ID: l.ID}, licence: l, licenceKind: p.licenceKind[l.ID]})
				break
			}
		}
	case "privilege":
		for i := range p.rec.Privileges {
			pv := &p.rec.Privileges[i]
			if len(a.PrivilegeKinds) > 0 && !slices.Contains(a.PrivilegeKinds, pv.Kind) {
				continue
			}
			l := p.licenceOf(pv.LicenceID)
			if !p.licenceMatches(a, l) {
				continue
			}
			out = append(out, subjectRef{
				Subject: Subject{Kind: "privilege", ID: pv.ID, Detail: pv.Detail},
				licence: l, licenceKind: p.licenceKind[pv.LicenceID],
				issued: pv.Issued, validFrom: pv.ValidFrom, expires: pv.Expires,
			})
		}
	case "credential":
		for i := range p.rec.Credentials {
			c := &p.rec.Credentials[i]
			if len(a.CredentialTypes) > 0 && !slices.Contains(a.CredentialTypes, c.Type) {
				continue
			}
			out = append(out, subjectRef{
				Subject: Subject{Kind: "credential", ID: c.ID, Detail: c.Type},
				issued:  c.Issued, validFrom: c.ValidFrom, expires: c.Expires,
			})
		}
	case "training":
		s := subjectRef{Subject: Subject{Kind: "training", Detail: a.Programme}}
		if len(a.Authorities) == 0 && len(a.LicenceKinds) == 0 {
			out = append(out, s)
			break
		}
		for i := range p.rec.Licences {
			l := &p.rec.Licences[i]
			if p.licenceMatches(a, l) {
				s.ID, s.licence, s.licenceKind = l.ID, l, p.licenceKind[l.ID]
				out = append(out, s)
				break
			}
		}
	case "type":
		for _, rt := range p.matchingRatings(a) {
			for i := range p.rec.Variants {
				vr := &p.rec.Variants[i]
				if vr.RatingID != rt.ID {
					continue
				}
				s := p.ratingSubject("type", rt)
				s.ID, s.Detail, s.variant = vr.ID, vr.Name, vr.Name
				s.issued, s.validFrom, s.expires = vr.Issued, nil, nil
				out = append(out, s)
			}
		}
	case "launch_method":
		for _, rt := range p.matchingRatings(a) {
			methods := map[string]bool{}
			for _, pv := range p.rec.Privileges {
				if pv.LicenceID == rt.LicenceID && pv.Kind == "LAUNCH_METHOD_TRAINED" {
					methods[strings.ToLower(pv.Detail)] = true
				}
			}
			for _, f := range p.flights {
				if f.Class == rt.Class && f.LaunchMethod != "" && !f.Date.After(asOf) {
					methods[f.LaunchMethod] = true
				}
			}
			for _, m := range v.LaunchMethods {
				if !methods[m] || (len(a.LaunchMethods) > 0 && !slices.Contains(a.LaunchMethods, m)) {
					continue
				}
				s := p.ratingSubject("launch_method", rt)
				s.Detail = m
				s.issued, s.validFrom, s.expires = nil, nil, nil
				out = append(out, s)
			}
		}
	}
	return out
}
