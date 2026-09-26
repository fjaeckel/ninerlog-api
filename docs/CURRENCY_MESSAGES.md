# Currency message keys

A binding cross-repo contract between `ninerlog-api`, `ninerlog-frontend` and
`ninerlog-ios`, in the same spirit as [SESSION_CONTRACT.md](./SESSION_CONTRACT.md).
Change it in the same PR as any behaviour it describes.

## Why keys and not prose

The currency engine used to return finished English sentences (and finished *German*
sentences from the ultralight evaluator, so a German user saw German UL cards next to
English EASA cards, and an English user saw German UL cards). No client could fix that,
because translating a rendered sentence is not possible.

The API therefore emits a **key** naming which statement is true, plus the **params**
that statement needs. The server keeps deciding *what* is true — that is the regulatory
logic, and it must not be duplicated in TypeScript and Swift where it would drift. The
client decides *how to say it*.

The `message` and `name` text fields are **gone**. Clients render keys; there is no
English fallback in the payload. The one exception is `CurrencyRequirement.name` on
custom currency rules, which is pilot-authored user data.

Server-sent notification emails render the same catalogue per locale in
`pkg/email/currency_messages.go`, chosen by the user's `PreferredLocale` — before this,
a German email template interpolated the API's English sentence.

## Deprecating a field

A deprecated field is made **optional in the spec at the moment it is deprecated**, while
the server keeps sending it. It is not left required until the PR that removes it.

This is not a formality. `swift-openapi-generator` maps a required field to a
non-optional `Swift.String`, and `Decodable` treats an absent required key as a fatal
error — not a nil. A client generated while the field is still `required` therefore stops
decoding the *entire* response the moment the server drops the field, even if that client
never reads it. The whole currency screen fails, not one label.

So a deprecation window that only migrates *rendering* protects nothing on iOS. The
client must also be generated against a schema that already tolerates the field's
absence. Marking it optional up front is what buys the window its value: clients
generated during the window keep working across the removal with no second regeneration,
which matters because App Store review latency means an older build is always still in
the field.

TypeScript hides this failure rather than avoiding it — `openapi-typescript` emits a
required property, but nothing validates at runtime, so the web client keeps working with
a type that has quietly become a lie.

## Removed fields

Clients still rendering these must drop the code; the API no longer sends them.

| Field | Replaced by |
| --- | --- |
| `message` on rating / passenger / flight-review / requirement / launch-method results | `messageKey` + `messageParams` |
| `name` on regulatory requirements | `nameKey` (custom-rule `name` stays — it is user data) |
| `passengerPrivilege` on `PassengerCurrency` | nothing — no code path ever populated it, so no response content changes |

`passengerPrivilege` is called out because the web client renders it today. The branch
was unreachable, so deleting that markup changes nothing a user sees, but it should go
with the rest — along with any test fixture that supplies the field, since such a
fixture keeps passing while covering a surface the API cannot produce.

## Rules

1. **Keys are plain strings, not an enum.** Adding a key is not a breaking change.
   A client that meets an unrecognised key should render its own generic wording.
2. **Params never repeat fields the object already carries.** `classType`,
   `regulatoryAuthority`, `licenseType`, `expiryDate`, `current`/`required`/`unit`,
   `launches`/`method`, `dayExpiresOn`/`nightExpiresOn` are all fields; the client
   composes from them. `messageParams` carries only `days`, `needed` and `date`. The
   exception is `remedyParams` (`missing`, `unit`, `method`): a remedy is also the
   `reasonKey` of a readiness item, which does not carry the row's fields, so its params
   are complete on their own.
3. **A key means one statement, not one authority.** LAPL, SPL, SPL-TMG and German UL
   all report `rating.recency_current`; which regulation to cite comes from
   `ruleDescriptionKey`.
4. **User data is never keyed.** Custom currency rule requirement names are written by
   the pilot and are returned in `name` with no `nameKey`. Render them as-is. The same
   split applies outside currency: the hints from `GET /announcements` carry stable string
   `id`s that double as localisation keys, while operator-authored announcements carry
   author-written text in `message` and are never translated.
5. `unknown` status is not self-explanatory — the key disambiguates
   `rating.no_expiry_date` (the user must enter data), `rating.ul_kind_required` (the user
   must set the ultralight kind), `rating.evaluation_failed` (backend problem, retry) and
   `rating.ir_not_applicable` (structurally N/A).

## `ClassRatingCurrency.status`

| Status | Meaning | Typical keys |
| --- | --- | --- |
| `current` | May exercise the privileges | `rating.recency_current`, `rating.revalidation_current`, `rating.valid_until` |
| `expiring` | Expiry date approaching, or expiry-anchored revalidation experience (FCL.740.A, FCL.625.A) not yet met | `rating.expiring`, `rating.revalidation_not_met`, `rating.revalidation_expiring_met` |
| `expired` | Past the expiry date; FAA §61.57 currency not met | `rating.expired`, `rating.pax_not_current`, `rating.glider_not_current` |
| `lapsed` | A rolling recency rule (LAPL, SPL, SPL TMG, GPL, German UL) is not met: the licence is valid, the privileges may not be exercised until recency is restored. There is no expiry date to count down to. | `rating.recency_not_met` |
| `unknown` | Not determinable; see rule 5 | `rating.no_expiry_date`, `rating.ul_kind_required`, … |

`lapsed` is new; a client that does not know it should render it as not current, never as
current. `expired` stays reserved for a date expiry.

`ClassRatingCurrency.unclassifiedFlights`, on a German ultralight rating, counts flights in the
window on `ULTRALIGHT` aircraft with no kind that the rule did not count (the pilot holds UL
ratings of more than one kind, or one with no kind). It is absent when zero. Render it as a
prompt to set the aircraft's kind; it never changes the status.

## `ClassRatingCurrency.messageKey`

| Key | Params | Meaning |
| --- | --- | --- |
| `rating.no_expiry_date` | — | No expiry date recorded; the pilot must enter one |
| `rating.no_expiry_date_manual` | — | Same, for authorities with no automated rules — tracked manually |
| `rating.evaluation_failed` | — | Flight data could not be read; transient, retry |
| `rating.expired` | — | Past `expiryDate` |
| `rating.expiring` | `days` | Expiry approaching, no experience requirements attached |
| `rating.valid_until` | — | Valid, no experience requirements attached |
| `rating.window_not_open` | `date` | Recently revalidated; the experience window opens on `date` |
| `rating.revalidation_not_met` | — | Revalidation experience incomplete |
| `rating.revalidation_not_met_prof_check` | — | Same, and a proficiency check may substitute (FCL.740.A(b)(2)) |
| `rating.revalidation_expiring_met` | `days` | Requirements met, expiry approaching |
| `rating.revalidation_current` | — | Requirements met, not near expiry |
| `rating.recency_not_met` | — | Rolling recency incomplete (LAPL, SPL, GPL, UL); status `lapsed` |
| `rating.recency_current` | — | Rolling recency satisfied (LAPL, SPL, GPL, UL) |
| `rating.sfcl_tmg_exempt` | — | SPL TMG privileges need no SFCL.160(b) experience: the pilot holds Part-FCL TMG privileges (SFCL.160(c)) |
| `rating.ul_kind_required` | — | German ultralight rating with no ultralight kind: recency (LuftPersV §45) depends on the kind, so nothing is evaluated and no passenger currency is reported. Status `unknown`; the pilot must set the rating's kind |
| `rating.ir_hours_and_check_not_met` | — | EASA IR: neither IFR hours nor proficiency check |
| `rating.ir_hours_not_met` | — | EASA IR: IFR hours short |
| `rating.ir_check_not_met` | — | EASA IR: annual proficiency check missing |
| `rating.ir_current` | — | FAA IR: §61.57(c) satisfied |
| `rating.ir_lapsed_safety_pilot` | — | FAA IR: 6–12 months, recoverable with a safety pilot |
| `rating.ir_expired_ipc` | — | FAA IR: >12 months, IPC required |
| `rating.ir_not_applicable` | — | No instrument privileges on this certificate (§61.315) |
| `rating.pax_not_current` | `needed` | FAA Tier-1: day landings short |
| `rating.pax_day_current_night_not` | `needed` | FAA Tier-1: day met, night short |
| `rating.pax_current_day_night` | — | FAA Tier-1: day and night met |
| `rating.flight_review_glider_alternative` | — | FAA glider: no current flight review, but three instructional glider flights in the flight review period stand in for it (§61.56(b)) |
| `flight_review.*` | as in [`FlightReviewStatus.messageKey`](#flightreviewstatusmessagekey) | FAA glider: the rating takes the flight review's statement (`none_on_record`, `expired`, `expiring`, `current`) |

`rating.glider_not_current` and `rating.glider_current` are no longer sent: an FAA glider
rating follows the §61.56 flight review, and §61.57(a) is reported as passenger currency
under `ruleDescriptionKey` `faa_glider` ([SAILPLANES.md](./SAILPLANES.md#faa-gliders-14-cfr-part-61)).
Clients may drop their strings.

## `PassengerCurrency.messageKey`

| Key | Params | Meaning |
| --- | --- | --- |
| `pax.evaluation_failed` | — | Flight data could not be read |
| `pax.not_current` | `needed` | Day requirement short by `needed` landings (German UL §45a: take-offs and landings, `dayLandings` is the smaller count) |
| `pax.current_day_no_night_privilege` | — | Day met; this licence has no night privilege (German UL: and the §84a passenger authorisation is recorded) |
| `pax.current_day_night_ir_waived` | — | Day met; night waived for IR holders (FCL.060(b)(2)(ii)) |
| `pax.current_day_night` | — | Day and night both met |
| `pax.day_current_night_not` | `needed` | Day met, night short by `needed` |
| `pax.current_day_privilege_separate` | — | UL: experience met, but the passenger endorsement is proved separately. Sent only when the licence privileges could not be read |
| `pax.ul_authorisation_missing` | — | German UL: §45a experience met, but no current `UL_PASSENGER_AUTH` privilege (LuftPersV §84a) is recorded on the licence. `dayStatus` `unknown`; `requirements` shows the progress toward the authorisation |
| `pax.gpl_experience_not_met` | `needed` | GPL: `needed` more minutes as PIC on gyroplanes since licence issue before passengers may be carried (FCL.205.G(a)(2)) |

`PassengerCurrency.requirements` holds informational rows that never change `dayStatus`: the
SFCL.115(a)(2) prerequisites on `easa_spl_pax` and `easa_spl_tmg_pax`, and progress toward
the §84a authorisation on a German UL entry without one (see the name keys below).

Expiry dates are **not** in the message — read `dayExpiresOn` / `nightExpiresOn` and
render them yourself. See [DOMAIN.md](./DOMAIN.md#passenger-currency-expiry-dayexpireson--nightexpireson).

## `PrivilegeCurrency.messageKey`

`CurrencyStatusResponse.privileges[]` reports each licence privilege
([SAILPLANES.md](./SAILPLANES.md#privileges)). `status` is `current`, `lapsed`, `expired` or
`unknown`. `messageParams.date` is the privilege's expiry date, absent when it has none; it
is sent with every key but `privilege.evaluation_failed`.

| Key | Status | Meaning |
| --- | --- | --- |
| `privilege.valid` | `current` | Valid; the kind has no recency rule |
| `privilege.recency_current` | `current` | Valid and its recency rule is met |
| `privilege.recency_not_met` | `lapsed` | Its recency rule is not met; the unmet rows carry the remedy |
| `privilege.expired` | `expired` | Past its expiry date |
| `privilege.evaluation_failed` | `unknown` | Flight data could not be read |

| `ruleDescriptionKey` | Rule |
| --- | --- |
| `privilege_expiry` | Expiry only (aerobatics, TMG night, BI(S), FE(S), UL passenger authorisation, UL type briefing) |
| `sfcl_155_launch_method` | SFCL.155(a) trained launch method; no recency of its own |
| `sfcl_205_towing`, `sfcl_205_banner_towing` | SFCL.205(c): 5 tows in 24 months |
| `faa_61_69_towing` | 14 CFR 61.69(a)(5): 3 tows or 3 aerotows as glider PIC in 24 calendar months |
| `sfcl_215_cloud_flying` | SFCL.215: 1 h or 5 flights in 24 months |
| `sfcl_360_fi_s` | SFCL.360: 30 h or 60 launches of instruction in 3 years |
| `dulv_ul_towing` | DULV towing authorisation: 10 tows in 24 months per kind |

## `FlightReviewStatus.messageKey`

| Key | Params | Meaning |
| --- | --- | --- |
| `flight_review.evaluation_failed` | — | Could not be determined |
| `flight_review.none_on_record` | — | No flight review logged |
| `flight_review.expired` | `date` | Expired; `date` is when the last one was completed |
| `flight_review.expiring` | `days`, `date` | Expiring in `days`; `date` is the last completion |
| `flight_review.current` | `date` | Current; `date` is the last completion, `expiresOn` the validity end |

## `CurrencyRequirement`

`nameKey` is one of: `requirement.total_time`, `.pic_time`, `.ifr_time`, `.landings`,
`.day_landings`, `.night_landings`, `.refresher_training`, `.training_flight`,
`.proficiency_check`, `.approaches`, `.holds`, `.route_sectors`, `.launches`,
`.sep_land_time`, `.sep_land_landings`, `.sep_sea_time`,
`.sep_sea_landings`, `.flight_time`, `.training_flights`, `.tmg_time`, `.tmg_landings`,
`.tmg_training_flight`, `.flight_review`. Absent on custom rules — see rule 4.
`requirement.launches_and_landings` is no longer sent (it was the FAA glider §61.57(a) row).

`.training_flight` (LAPL(A) FCL.140.A(a)(1), German UL §45(2), (2a) and gyroplane) is one
flight of at least 1 h total time with dual time: `current` is the longest such flight in
minutes and `required` is 60. `.refresher_training` (FCL.740.A(b)(1)(ii), FCL.740.A(b)(2),
FCL.240.G) is dual minutes summed over the window.

The four `sep_*` keys are the per-class minimums of EASA FCL.140.A(b), present only on a
LAPL license holding both SEP(land) and SEP(sea) ratings. LAPL results also carry a
`.proficiency_check` requirement: the FCL.140.A(a)(2) alternative to the experience rows.

The sailplane rules (see [SAILPLANES.md](./SAILPLANES.md)) use these keys:

| `nameKey` | Rule | `unit` | Meaning |
| --- | --- | --- | --- |
| `requirement.flight_time` | SFCL.160(a)(1), (b)(1) | `minutes` | PIC, dual or supervised solo time on sailplanes including TMGs |
| `requirement.launches` | SFCL.160(a)(1)(i) | `launches` | Launches on sailplanes excluding TMGs |
| `requirement.training_flights` | SFCL.160(a)(1)(ii) | `flights` | Training flights with an instructor on sailplanes excluding TMGs |
| `requirement.tmg_time` | SFCL.160(b)(1)(i) | `minutes` | PIC, dual or supervised solo time on TMGs |
| `requirement.tmg_landings` | SFCL.160(b)(1)(ii) | `landings` | Take-offs and landings on TMGs |
| `requirement.tmg_training_flight` | SFCL.160(b)(1)(iii) | `minutes` | Longest training flight with an instructor on a TMG (needs 60) |
| `requirement.proficiency_check` | SFCL.160(a)(2), (b)(2) | `check` | The alternative to the experience rows |
| `requirement.flight_review` | FAA §61.56(a) | `review` | The latest flight review, met while it is current or expiring |
| `requirement.training_flights` | FAA §61.56(b) | `flights` | Instructional glider flights in the flight review period (needs 3); the alternative to the flight review row |

Privilege and passenger-prerequisite rows use these keys:

| `nameKey` | Rule | `unit` | Meaning |
| --- | --- | --- | --- |
| `requirement.tows` | SFCL.205(c), 61.69, DULV | `tows` | Tows flown as tug pilot (needs 5, 3 or 10) |
| `requirement.towed_glider_flights` | 61.69(a)(5)(ii) | `flights` | Aerotows as PIC of a glider; the alternative to `requirement.tows` |
| `requirement.cloud_flying_time` | SFCL.215 | `minutes` | IFR time as PIC on sailplanes (needs 60) |
| `requirement.cloud_flying_flights` | SFCL.215 | `flights` | Sailplane flights as PIC with IFR time (needs 5); the alternative to the time row |
| `requirement.instruction_time` | SFCL.360 | `minutes` | Instruction given on sailplanes including TMGs (needs 1800) |
| `requirement.instruction_launches` | SFCL.360 | `launches` | Launches of flights with instruction given (needs 60); the alternative to the time row |
| `requirement.fi_refresher` | SFCL.360 | `training` | FI(S) refresher training; always `requirement.untracked` |
| `requirement.pax_prerequisite_time` | SFCL.115(a)(2) | `minutes` | PIC time on sailplanes including TMGs since licence issue (needs 600) |
| `requirement.pax_prerequisite_launches` | SFCL.115(a)(2) | `launches` | Launches as PIC since licence issue (needs 30); the alternative to the time row |
| `requirement.pax_competence_flight` | SFCL.115(a)(2) | `flight` | Passenger-competence training flight; always `requirement.untracked` |
| `requirement.ul_xc_flights` | LuftPersV §84a | `flights` | Cross-country flights with an instructor on the kind (needs 5) |
| `requirement.ul_xc_landing_flights` | LuftPersV §84a | `flights` | Of those, flights with an intermediate landing (needs 2) |
| `requirement.ul_xc_distance` | LuftPersV §84a | `km` | Their total distance (needs 200) |

Where a rule accepts either of two rows, the privilege is met when one is; the other stays
unmet with its remedy.

| `messageKey` | Params | Meaning |
| --- | --- | --- |
| `requirement.progress` | — | The common case: render `current` / `required` `unit` |
| `requirement.prof_check_completed` | `date` | Proficiency check, or FAA flight review, completed on `date` |
| `requirement.prof_check_missing` | — | Not completed in the validity period |
| `requirement.untracked` | — | Informational: NinerLog cannot count this requirement. `met` is false, `current` 0, `required` 1; it never changes a status and carries no remedy |

`LaunchMethodCurrency.messageKey` is always `launch_method.progress`; render
`launches` / `required` for `method` (`winch`, `car`, `aerotow`, `self-launch`, `bungee`).
`trained` is true when the pilot recorded a `LAUNCH_METHOD_TRAINED` privilege for the
method (SFCL.155(a)); such a method is listed even at `0` launches.

## `validUntil`

`CurrencyRequirement.validUntil`, `LaunchMethodCurrency.validUntil` and
`ClassRatingCurrency.validUntil` are dates, not keys: the last day a met row, a met launch
method or a `current` rating stays met if the pilot does not fly again. They are present
only on rolling-window rules (LAPL, SPL, SPL TMG, GPL, German UL, FAA §61.57) and only when
met; expiry-anchored rules (FCL.740.A, FCL.625.A) rely on `expiryDate`. A rating's date is
the latest on which any accepted alternative still holds, so with a recent proficiency
check it can be later than every experience row. Render it as "until 31 May 2027"; it is
never a reason to show a warning by itself. How it is computed:
[DOMAIN.md](./DOMAIN.md#recency-projection-validuntil-and-remedies).

## Remedies

An unmet row carries `remedyKey` and `remedyParams`: what the pilot does to restore it.
They are sent on every unmet row, also on the alternative the pilot is not using (the
proficiency check of a rating current by experience); show them where the rating or method
is not current.

| `remedyKey` | On | `remedyParams` | Meaning |
| --- | --- | --- | --- |
| `remedy.fly_more` | count and sum rows | `missing`, `unit` | Fly `missing` more `unit` (`minutes`, `landings`, `launches`, `flights`, `approaches`, `holds`). The row's `nameKey` says what counts — `requirement.training_flights` means flights with an instructor, `requirement.pic_time` minutes as PIC |
| `remedy.training_flight` | `requirement.training_flight`, `requirement.tmg_training_flight` | — | Fly one flight of at least 1 h with an instructor |
| `remedy.proficiency_check` | `requirement.proficiency_check` | — | Take a proficiency check with an examiner. The row only exists where the rule accepts one (the FCL.625.A IR check is mandatory rather than an alternative) |
| `remedy.launch_method_dual` | `LaunchMethodCurrency` | `method`, `missing` | SFCL.155(d): fly `missing` launches with `method` dual or as supervised solo |
| `remedy.privilege_with_instructor` | SFCL.205 and SFCL.215 privilege rows | `missing`, `unit` | Fly `missing` more `unit` (`tows`, `minutes`, `flights`) dual or under the supervision of an instructor |

`remedy.fly_more` also appears on privilege and passenger-prerequisite rows, with the units
`tows` and `km` besides those above.

The FAA §61.56 flight review row has no remedy. Custom rules carry none.

## Readiness (`GET /currency/readiness`)

Each `ReadinessItem` has `ready`, a `status` and a `reasonKey` with `params`. Keys are
reused from the tables above wherever one says the right thing:

| `kind` | `ready` | `reasonKey` | `params` |
| --- | --- | --- | --- |
| `rating` | `current` or `expiring` | the rating's `messageKey` (for example `rating.recency_current`, `rating.revalidation_not_met`, `rating.expired`) | the rating's `messageParams` |
| `rating`, status `lapsed` | no | the remedy of the first unmet experience row, else `remedy.proficiency_check` | its `remedyParams` |
| `launch_method` | met | `readiness.launch_method_current` | `date`: the method's `validUntil` |
| `launch_method` | not met, status `lapsed` | `remedy.launch_method_dual` | `method`, `missing` |
| `passengers` | day status `current` | the passenger `messageKey` (`pax.not_current` with `needed` when short) | the passenger `messageParams` |
| `credential` | status `valid` | `readiness.credential_valid` | `date`: the expiry date, absent when the certificate has none |
| `credential` | status `expired` | `readiness.credential_expired` | `date`: the expiry date |

The credential type is not repeated: look it up by `credentialId`. Lena's Saturday
("solo ✓ winch ✓ aerotow ✗ (2 launches) passengers ✓") is one `rating`, two
`launch_method` and one `passengers` item; the aerotow item is
`remedy.launch_method_dual` with `{method: "aerotow", missing: 2}`.

| Key | Params | Meaning |
| --- | --- | --- |
| `readiness.launch_method_current` | `date` | The launch method is current on the date, until `date` |
| `readiness.credential_valid` | `date`? | The medical certificate is valid on the date |
| `readiness.credential_expired` | `date` | The medical certificate expired on `date`, on or before the date asked |

## Adding a key

1. Add the constant in `internal/service/currency/messages.go` (remedy and readiness keys
   live there too).
2. Add it to `knownMessageKeys` in `messages_test.go` — the sweep in
   `TestEveryRatingResultCarriesAKey` fails on any emitted key not in the catalogue.
3. Add a row here.
4. Add the string to `ninerlog-frontend/src/i18n/locales/{en,de}/currency.json` under
   `messages.`, to the iOS catalogue, and — if the key can reach a notification email —
   to both maps in `pkg/email/currency_messages.go`, which
   `TestCurrencyMessageCataloguesMatch` keeps in step.
