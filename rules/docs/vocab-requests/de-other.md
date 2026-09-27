# Vocabulary requests: de-other (German ultralights, other.ninerlog)

None of these blocks a rule; every rule is encoded with the closest fit and says so in its
notes.

## other.ninerlog.expiry-only, other.ninerlog.expiry-only.faa-ultralight, other.ninerlog.privilege-expiry, other.ninerlog.privilege-expiry.launch-method: source kind for app policy

- Article text (verbatim, with cite): none. These are NinerLog's fallbacks (expiry date only)
  for authorities and privileges without a rule; no regulation states them.
- What the rule needs to say: "NinerLog policy, no regulation source".
- Proposed vocabulary entry: `source_kinds.app_policy` ("NinerLog behaviour with no regulation
  behind it"), and in `schema/rule.schema.json` make `source` optional when
  `source_kind: app_policy` (or allow `source.file: null` with no quote).
- Why existing entries do not work: `source` is required with an existing `sources/` file
  and `source_kind` must be regulation, national_law or association. The rules currently use
  `source_kind: national_law` with LuftPersV § 9(1) second sentence as the closest fit, and
  say so in `cite` and `notes`.
- Cases that would exercise it: the existing cases of the four rules.

## de.dulv.* and de.daec.*: association documents without stored text

- Article text: DULV and DAeC recency rules under LuftPersV § 45(4); private documents, not
  stored (copyright), titles not verified.
- What the rule needs to say: cite the association document by name as the primary source
  without a `sources/` file.
- Proposed vocabulary entry: allow `source.file` to be omitted (and `quote` empty) when
  `source_kind: association`, with a required `source.document` (title, edition, date).
- Why existing entries do not work: `source.file` must exist, so the association rules use
  the delegating statute (LuftPersV § 45(4)) as `source` and name the association rule in
  `citations` and `notes`.
- Cases that would exercise it: the existing cases of the five association rules.

## de.luftpersv.42.*: LuftPersV § 42 text missing from sources/

- Article text: not available; `sources/de/luftpersv-42.md` does not exist.
- What the rule needs to say: the training hours of § 42 (or of the association training
  rules § 42(2) delegates to).
- Proposed entry: add `sources/de/luftpersv-42.md` (same retrieval as the other LuftPersV
  files) so the training rules can quote it; then decide whether the hours are
  `national_law` or `association`.
- Why existing entries do not work: porting agents may not add sources; the rules quote
  § 45b instead and are `partial`.

## de.daec.ul-recency.weight-shift (and similar): coverage of `not_recorded` rows

- What the rule needs to say: a requirement that exists but NinerLog cannot count (the DAeC
  safety or performance training), shown as an untracked row.
- Proposed change: `RequiredTags` should not require `requirement:<id>:met` / `:unmet` for a
  leaf whose metric has `aggregate: none` (`not_recorded`), because such a leaf can never be
  met or unmet; require an `untracked` observation instead.
- Why existing entries do not work: with a `not_recorded` leaf the rule can never reach full
  coverage, so the row is left out and only described in `notes`.
