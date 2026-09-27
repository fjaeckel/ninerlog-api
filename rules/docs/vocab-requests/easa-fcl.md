# Vocabulary requests: easa-fcl (Part-FCL powered aircraft, Part-MED)

## easa.part-fcl.fcl-140-h.lapl-h: LAPL_H licence kind

- Article text (verbatim, EASA FCL.140.H(a)): "(a) Holders of an LAPL(H) shall exercise the privileges of their licence on a specific type only if in the last 12 months they have, in the relevant type, taken one of the following steps:"
- What the rule needs to say: select helicopter type ratings on a LAPL(H) only.
- Proposed vocabulary entry: `licence_kinds.LAPL_H: { aliases: [LAPL(H)] }`, removing LAPL(H) from the HELICOPTER aliases (and, if wanted, PPL_H / CPL_H / ATPL_H for MED.A.030 and FCL.065). Rules currently using HELICOPTER (easa.part-fcl.fcl-625-h.ir) would list the new kinds.
- Why existing entries do not work: HELICOPTER covers PPL(H), CPL(H), ATPL(H) and LAPL(H); a LAPL(H) recency rule would report every PPL(H) holder as lapsed. The rest of the article is expressible (typeDesignators [$subject], rolling_months 12, minutes.picOrDualOrSpic, takeoffs_and_landings, minutes.dual with simulator include, a proficiency_check event).
- Cases that would exercise it: a LAPL(H) R44 rating with 6 h and 6 take-offs and landings plus 1 h dual in 12 months is current; the same on a PPL(H) is not evaluated. The same split lets easa.part-med.med-a-030 require class 2 for PPL(H) and class 1 for CPL(H)/ATPL(H).

## easa.part-fcl.fcl-740-h.helicopter: single-engine helicopter types up to 3 175 kg

- Article text (verbatim, EASA FCL.740.H(a)(2)): "(2) for type ratings for single-engine helicopters up to a maximum take-off mass of 3 175 kg, they shall meet one of the following conditions:"
- What the rule needs to say: offer the (a)(2)(ii) alternative (6 h as PIC in the type within the validity period, 1 h refresher training in the 3 months before expiry, aircraft or FSTD) only for single-engine types up to 3 175 kg.
- Proposed vocabulary entry: optional rating fields `engines` (int) and `mtomKg` (int), and a node `when` condition (or applies_to selector) `{ subject: { maxEngines: 1, maxMtomKg: 3175 } }`, absent = unknown so the branch is not offered.
- Why existing entries do not work: nothing on a rating or subject says how many engines the type has or its mass; `ulCredit.minMtomKg` reads flight mass for ultralight credit only; offering the branch to every helicopter would report pilots of heavy or twin types as current.
- Cases that would exercise it: an R44 rating (1 engine, 1 134 kg) revalidated by 6 h PIC and a 1 h refresher without a check is current; an AW139 rating with the same flying is not.

## easa.part-fcl.fcl-815.mountain: landings on designated mountain surfaces

- Article text (verbatim, EASA FCL.815(d)(1)): "(1) completed at least six landings on a surface designated to require a mountain rating;"
- What the rule needs to say: count landings on designated mountain surfaces in the last 2 years (rolling_months 24), or a mountain proficiency check.
- Proposed vocabulary entry: optional flight field `mountainLandings` (int) and metric `mountain_landings` (sum, optional input, unit landings); or a flight flag `mountainSurface` with `landings.total`.
- Why existing entries do not work: no record field says where a landing was made; `not_recorded` would make the landing requirement untracked forever and its met/unmet coverage tags unreachable, and the proficiency check alone would report pilots who flew the six landings as lapsed.
- Cases that would exercise it: 6 mountain landings in 2 years keep the MOUNTAIN privilege current; 5 do not; a proficiency check with checkRating MOUNTAIN does.
