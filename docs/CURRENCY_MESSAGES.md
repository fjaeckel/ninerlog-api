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

## Rules

1. **Keys are plain strings, not an enum.** Adding a key is not a breaking change.
   A client that meets an unrecognised key should render its own generic wording.
2. **Params never repeat fields the object already carries.** `classType`,
   `regulatoryAuthority`, `licenseType`, `expiryDate`, `current`/`required`/`unit`,
   `launches`/`method`, `dayExpiresOn`/`nightExpiresOn` are all fields; the client
   composes from them. `messageParams` carries only `days`, `needed` and `date`.
3. **A key means one statement, not one authority.** LAPL, SPL, SPL-TMG and German UL
   all report `rating.recency_current`; which regulation to cite comes from
   `ruleDescriptionKey`.
4. **User data is never keyed.** Custom currency rule requirement names are written by
   the pilot and are returned in `name` with no `nameKey`. Render them as-is. The same
   split applies outside currency: the hints from `GET /announcements` carry stable string
   `id`s that double as localisation keys, while operator-authored announcements carry
   author-written text in `message` and are never translated.
5. `unknown` status is not self-explanatory — the key disambiguates
   `rating.no_expiry_date` (the user must enter data), `rating.evaluation_failed`
   (backend problem, retry) and `rating.ir_not_applicable` (structurally N/A).

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
| `rating.recency_not_met` | — | Rolling recency incomplete (LAPL, SPL, UL) |
| `rating.recency_current` | — | Rolling recency satisfied (LAPL, SPL, UL) |
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
| `rating.glider_not_current` | `needed` | FAA glider: launches short |
| `rating.glider_current` | — | FAA glider: launches met |

## `PassengerCurrency.messageKey`

| Key | Params | Meaning |
| --- | --- | --- |
| `pax.evaluation_failed` | — | Flight data could not be read |
| `pax.not_current` | `needed` | Day requirement short by `needed` landings |
| `pax.current_day_no_night_privilege` | — | Day met; this licence has no night privilege |
| `pax.current_day_night_ir_waived` | — | Day met; night waived for IR holders (FCL.060(b)(2)(ii)) |
| `pax.current_day_night` | — | Day and night both met |
| `pax.day_current_night_not` | `needed` | Day met, night short by `needed` |
| `pax.current_day_privilege_separate` | — | UL: experience met, but the passenger endorsement is proved separately |

Expiry dates are **not** in the message — read `dayExpiresOn` / `nightExpiresOn` and
render them yourself. See [DOMAIN.md](./DOMAIN.md#passenger-currency-expiry-dayexpireson--nightexpireson).

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
`.launches_and_landings`. Absent on custom rules — see rule 4.

| `messageKey` | Params | Meaning |
| --- | --- | --- |
| `requirement.progress` | — | The common case: render `current` / `required` `unit` |
| `requirement.prof_check_completed` | `date` | Proficiency check completed on `date` |
| `requirement.prof_check_missing` | — | Not completed in the validity period |

`LaunchMethodCurrency.messageKey` is always `launch_method.progress`; render
`launches` / `required` for `method`.

## Adding a key

1. Add the constant in `internal/service/currency/messages.go`.
2. Add it to `knownMessageKeys` in `messages_test.go` — the sweep in
   `TestEveryRatingResultCarriesAKey` fails on any emitted key not in the catalogue.
3. Add a row here.
4. Add the string to `ninerlog-frontend/src/i18n/locales/{en,de}/currency.json` under
   `messages.`, to the iOS catalogue, and — if the key can reach a notification email —
   to both maps in `pkg/email/currency_messages.go`, which
   `TestCurrencyMessageCataloguesMatch` keeps in step.
