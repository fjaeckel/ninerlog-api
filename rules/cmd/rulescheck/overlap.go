package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/fjaeckel/ninerlog-rules/engine"
)

// otherValue stands for record values outside a finite vocabulary domain (e.g. an authority
// the vocabulary does not list, or a licence type no alias classifies).
const otherValue = "<other>"

// selection is one applies_to dimension: the values a rule can select, within a universe.
type selection map[string]bool

// selectValues returns the universe values that include/exclude lists admit; an empty include
// list admits the whole universe.
func selectValues(universe, include, exclude []string, norm func(string) string) selection {
	if norm == nil {
		norm = func(s string) string { return s }
	}
	out := selection{}
	inc := map[string]bool{}
	for _, x := range include {
		inc[norm(x)] = true
	}
	exc := map[string]bool{}
	for _, x := range exclude {
		exc[norm(x)] = true
	}
	for _, u := range universe {
		u = norm(u)
		if (len(include) == 0 || inc[u]) && !exc[u] {
			out[u] = true
		}
	}
	return out
}

func (s selection) intersects(o selection) bool {
	for k := range s {
		if o[k] {
			return true
		}
	}
	return false
}

// licenceSelection returns the (authority, licence kind) pairs a rule can select.
func licenceSelection(v *engine.Vocabulary, a *engine.AppliesTo) selection {
	up := func(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }
	auths := selectValues(append(slices.Clone(v.RecordAuthorities), otherValue), a.Authorities, a.ExcludeAuthorities, up)
	allKinds := append(keys(v.LicenceKinds), otherValue)
	out := selection{}
	for auth := range auths {
		kinds := allKinds
		if auth == "DULV" || auth == "DAEC" {
			kinds = []string{"UL"}
		}
		for k := range selectValues(kinds, a.LicenceKinds, a.ExcludeLicenceKinds, nil) {
			out[auth+"|"+k] = true
		}
	}
	return out
}

// usesLicence reports whether a subject kind is selected through a licence.
func usesLicence(a *engine.AppliesTo) bool {
	switch a.Subject {
	case "credential":
		return false
	case "training":
		return len(a.Authorities)+len(a.ExcludeAuthorities)+len(a.LicenceKinds)+len(a.ExcludeLicenceKinds) > 0
	}
	return true
}

// ratingBased reports whether a subject kind is selected through a rating.
func ratingBased(subject string) bool {
	switch subject {
	case "rating", "passengers", "type", "launch_method":
		return true
	}
	return false
}

// periodsOverlap reports whether two effective periods share a day.
func periodsOverlap(a, b *engine.Rule) bool {
	if a.EffectiveTo != nil && b.EffectiveFrom != nil && a.EffectiveTo.Before(*b.EffectiveFrom) {
		return false
	}
	if b.EffectiveTo != nil && a.EffectiveFrom != nil && b.EffectiveTo.Before(*a.EffectiveFrom) {
		return false
	}
	return true
}

// canShareSubject reports whether rules a and b can select the same subject, and on which
// dimensions they meet (for the report).
func canShareSubject(v *engine.Vocabulary, a, b *engine.Rule) (bool, string) {
	x, y := &a.AppliesTo, &b.AppliesTo
	if x.Subject != y.Subject || !periodsOverlap(a, b) {
		return false, ""
	}
	var on []string
	if x.Subject == "training" && x.Programme != y.Programme {
		return false, ""
	}
	if usesLicence(x) && usesLicence(y) {
		lx, ly := licenceSelection(v, x), licenceSelection(v, y)
		if !lx.intersects(ly) {
			return false, ""
		}
	}
	if ratingBased(x.Subject) {
		classes := keys(v.Classes)
		cx := selectValues(classes, x.Classes, x.ExcludeClasses, nil)
		cy := selectValues(classes, y.Classes, y.ExcludeClasses, nil)
		if !cx.intersects(cy) {
			return false, ""
		}
		ul := append(slices.Clone(v.ULKinds), "none")
		if !selectValues(ul, x.ULKinds, nil, nil).intersects(selectValues(ul, y.ULKinds, nil, nil)) {
			return false, ""
		}
		if x.TypeRated != nil && y.TypeRated != nil && *x.TypeRated != *y.TypeRated {
			return false, ""
		}
		on = append(on, "classes "+strings.Join(sortedSel(intersect(cx, cy)), ","))
	}
	switch x.Subject {
	case "privilege":
		px := selectValues(v.PrivilegeKinds, x.PrivilegeKinds, nil, nil)
		py := selectValues(v.PrivilegeKinds, y.PrivilegeKinds, nil, nil)
		if !px.intersects(py) {
			return false, ""
		}
		on = append(on, "privileges "+strings.Join(sortedSel(intersect(px, py)), ","))
	case "credential":
		cx := selectValues(v.CredentialTypes, x.CredentialTypes, nil, nil)
		cy := selectValues(v.CredentialTypes, y.CredentialTypes, nil, nil)
		if !cx.intersects(cy) {
			return false, ""
		}
		on = append(on, "credentials "+strings.Join(sortedSel(intersect(cx, cy)), ","))
	case "launch_method":
		if !selectValues(v.LaunchMethods, x.LaunchMethods, nil, nil).intersects(selectValues(v.LaunchMethods, y.LaunchMethods, nil, nil)) {
			return false, ""
		}
	}
	return true, strings.Join(on, "; ")
}

func intersect(a, b selection) selection {
	out := selection{}
	for k := range a {
		if b[k] {
			out[k] = true
		}
	}
	return out
}

func sortedSel(s selection) []string {
	out := make([]string, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	sort.Strings(out)
	if len(out) > 6 {
		out = append(out[:6], fmt.Sprintf("... (%d)", len(out)))
	}
	return out
}

// resolvedOverlap reports whether a declared supersedes or a shared group resolves the pair.
func resolvedOverlap(a, b *engine.Rule) bool {
	if slices.Contains(a.Supersedes, b.ID) || slices.Contains(b.Supersedes, a.ID) {
		return true
	}
	return a.Group != "" && a.Group == b.Group
}

// checkOverlaps lists pairs of supported or partial rules that can select the same subject
// without a declared supersedes or shared group (DESIGN.md section 13).
func checkOverlaps(cat *engine.Catalogue) []string {
	var live []*engine.Rule
	for _, r := range cat.Rules {
		if r.Support != "not_supported" {
			live = append(live, r)
		}
	}
	var out []string
	for i, a := range live {
		for _, b := range live[i+1:] {
			ok, on := canShareSubject(cat.Vocabulary, a, b)
			if !ok || resolvedOverlap(a, b) {
				continue
			}
			msg := fmt.Sprintf("%s and %s can both evaluate the same %s subject", a.ID, b.ID, a.AppliesTo.Subject)
			if on != "" {
				msg += " (" + on + ")"
			}
			out = append(out, msg)
		}
	}
	sort.Strings(out)
	return out
}
