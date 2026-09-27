# Requests from the easa-sfcl porting family

## easa.part-sfcl.sfcl-115-a-2.passengers: a coverage exemption for not_recorded leaves

- Article text (verbatim, with cite): EASA SFCL.115(a)(2)(ii)(A) "have completed, after the
  issue of the SPL, at least 10 hours of flight time or 30 launches or take-offs and
  landings as PIC on sailplanes and, additionally, one training flight during which holders
  shall demonstrate to an FI(S) the competence required for the carriage of passengers"
- What the rule needs to say: the passenger-competence training flight is required but not
  recorded in NinerLog; show it as an untracked row (the API's requirement.pax_competence_flight).
- Proposed entry: none in the vocabulary. `RequiredTags` should not require
  `requirement:<id>:met` / `:unmet` for leaves whose metric has `aggregate: none`
  (`not_recorded`), which can never be tracked; today such a leaf makes the rule's coverage
  unreachable, so the porting guide's "something NinerLog cannot count" pattern is unusable.
- Why existing entries do not work: no event kind names a passenger-competence flight; a
  flight flag would be indistinguishable from any training flight.
- Cases that would exercise it: the existing sfcl-115-a-2 cases with an informational
  `pax_competence_flight` row asserted `{ tracked: false }`.
- Until then the rule is partial and takes the training flight as met.

## easa.part-sfcl.sfcl-130.spl-training: verbatim source for SFCL.130

- sources/easa/sfcl-130.md does not exist; the rule quotes "TODO: verify" and points at that
  file, which `rulescheck` reports as an error. SFCL.150(b)(1) also refers to
  SFCL.130(a)(2)(v) for the TMG extension figures. Please add SFCL.130 (same EUR-Lex
  consolidation as the other Part-SFCL sources); the rule's figures then need checking.
