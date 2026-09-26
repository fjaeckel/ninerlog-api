# Sailplanes and TMGs

What NinerLog needs to know about gliders, self-launching sailplanes and touring motor
gliders (TMGs): how to classify the aircraft, which EASA rules apply to their flights, and
how the currency engine implements each point. The general engine design is in
[DOMAIN.md](./DOMAIN.md#currency-engine).

Source: Commission Implementing Regulation (EU) 2018/1976, Annex III (Part-SFCL), added by
Regulation (EU) 2020/357. Part-SFCL replaced Part-FCL Subpart S (`FCL.140.S`, `FCL.110.S` …)
for sailplane pilots; anything citing `FCL.140.S` is out of date.

## Aircraft and classes

Definitions (Regulation 2018/1976 Art. 2 and Regulation 1178/2011 Art. 2):

| Term | Meaning |
| --- | --- |
| Sailplane | Heavier-than-air aircraft supported by the dynamic reaction of the air against its fixed lifting surfaces, whose free flight does not depend on an engine |
| Powered sailplane | A sailplane with one or more engines that has, with engine(s) inoperative, the characteristics of a sailplane |
| Self-launching sailplane (SLG) | A powered sailplane that can take off under its own power, usually with a retractable engine or propeller |
| TMG (touring motor glider) | A specific class of powered sailplane with an integrally mounted, non-retractable engine and non-retractable propeller, capable of taking off and climbing under its own power |

Every TMG is a sailplane in Part-SFCL, so "on sailplanes" includes TMGs and the rules
say "excluding TMGs" where they mean otherwise. A self-launching sailplane is a sailplane
and not a TMG, even though it has an engine.

German registrations don't tell them apart: `D-K…` marks every *Motorsegler* (SLG and
TMG), `D-0…`…`D-9…` pure sailplanes, `D-M…` ultralights including UL motorgliders.

How to set the aircraft's `aircraft_class` in NinerLog:

| Aircraft | `aircraft_class` | Counts toward |
| --- | --- | --- |
| Sailplane, including self-launching (e.g. ASK 21, DG-808, Arcus M) | `GLIDER` | `GLIDER` rating (SFCL.160(a)), self-launch launch method when `launchMethod` is `self-launch` |
| TMG (e.g. SF 25, Dimona, Super Dimona) | `TMG` | SPL TMG rating (SFCL.160(b)), the 5 h of SFCL.160(a), self-launch launch method, PPL/LAPL TMG and SEP pools (FCL.740.A, FCL.140.A) |
| UL sailplane or UL motorglider | `ULTRALIGHT` | German UL rules (LuftPersV §45) |

The class decides. A self-launching sailplane classed `TMG` would feed the TMG and
aeroplane rules instead of the sailplane rules; the engine cannot detect that mistake.

## Launch methods (SFCL.155)

`Flight.launchMethod` is one of `winch`, `car`, `aerotow`, `self-launch`, `bungee`.

SFCL.155(a): a pilot may use only launch methods they trained for. Initial training:

| Method | Training |
| --- | --- |
| Winch, car | 10 dual launches + 5 supervised solo launches |
| Aerotow, self-launch | 5 dual launches + 5 supervised solo launches; self-launch dual may be in a TMG |
| Bungee | 3 launches dual or supervised solo |

SFCL.155(c), verbatim: "In order to maintain the privileges for each launching method …
SPL holders shall complete a minimum of five launches during the last two years, except
for bungee launch, in which case they shall complete only two launches. In the case of
self-launch, launches may be done in self-launch or through take-offs in TMGs or a
combination of these."

SFCL.155(d): a pilot who lapses flies the missing launches dual or supervised solo.

Winch, car, aerotow and bungee are **towed** launches: they count only toward a `GLIDER`
rating or a sailplane-licence rule, never toward a powered class, its proficiency check or
its passenger currency. Self-launches are not towed.

### Launch method in and out

Every write path accepts only those five values (`models.ValidateFlightTextFields`,
`ErrInvalidLaunchMethod`); the service trims and lower-cases the value first and treats a
blank one as none. A flight create or update with any other value is a 400, and so is a
`POST /imports/json` restore: the restore stops at that flight with
`Failed to import flight on <date> (<reg>): invalid launch method: <value>`, like any other
invalid flight in a backup.

CSV import maps a launch column to the `launchMethod` import field
(`importtemplate.ParseLaunchMethod`, case-insensitive, trimmed):

| Cell | Launch method |
| --- | --- |
| `W`, `Winde`, `Windenstart`, `winch` | `winch` |
| `F`, `F-Schlepp`, `Flugzeugschlepp`, `aerotow` | `aerotow` |
| `E`, `Eigenstart`, `self-launch` | `self-launch` |
| `A`, `Autoschlepp`, `car` | `car` |
| `G`, `Gummiseil`, `Gummiseilstart`, `bungee` | `bungee` |

The single letters are Vereinsflieger's `S.-Art` codes; the generic German `Startart`
header and English `Launch Method`/`Launch Type` headers are recognised too. Any other
value imports the flight with no launch method; the row does not fail.

An imported launch method (from a launch column or a `[Launch: …]` remarks marker) is kept
only when `models.LaunchMethodApplies` to the flight's aircraft: the class already in the
fleet, or for an aircraft the import creates, the class it infers. It applies to no class
at all, `GLIDER`, `TMG`, and `ULTRALIGHT` with no kind or kind `SAILPLANE` or
`THREE_AXIS_MOTORGLIDER`; for every other class (`SEP*`, `MEP*`, `SET*`, `GYROPLANE`, other
ultralight kinds, `OTHER`) it is dropped. Vereinsflieger writes `E` for powered aircraft
too, so a club Cessna classed `SEP_LAND` imports with no launch method and no launch marker
in its remarks. The preview applies the same rule. `POST`/`PUT /flights` do not: a launch
method entered by hand is stored whatever the aircraft.

Exports: the standard CSV layout ends with a `LaunchMethod` column. The EASA, FAA and
vsimakhin/web-logbook CSV layouts and the EASA and FAA PDFs have no launch column, so
`flightrules.CombinedRemarks` appends `[Launch: <method>]` to the remarks cell after the
function-time annotations. On import a `[Launch: <method>]` marker in a remarks column is
read back as the launch method (a launch column wins) and removed from the remarks, so all
four CSV layouts round-trip it. The JSON backup carries the field as is. The AMC1 SFCL.050
sailplane PDF layout with its own launch columns is WP-22.

Aircraft an import creates get a class so the currency engine counts their flights; see
[Aircraft class on import](./AIRCRAFT_REGISTRATIONS.md#aircraft-class-on-import). A towed
launch on an aircraft with no other class evidence makes it `GLIDER`.

## Recency (SFCL.160)

### (a) Sailplanes, excluding TMGs

> SPL holders shall exercise SPL privileges, excluding TMGs, only if in the last 24 months
> before the planned flight they:
> (1) completed, on sailplanes, at least five hours of flight time as PIC or flying dual or
> solo under the supervision of an FI(S), including, on sailplanes, excluding TMGs, at
> least: (i) 15 launches; and (ii) two training flights with an FI(S); or
> (2) passed a proficiency check with an FE(S) on a sailplane, excluding TMGs; the
> proficiency check shall be based on the skill test for SPL.

Reading the text:

- The 5 h may be flown on any sailplane, TMGs included ("on sailplanes"); the 15 launches
  and the two training flights must be on sailplanes excluding TMGs.
- Dual and supervised solo time count toward the 5 h, not PIC time alone.
- "Two training flights" is a number of flights, with no minimum duration. Two short
  winch circuits with an instructor count; one long dual flight does not.

### (b) TMGs

> SPL holders shall exercise their TMG privileges only if in the last 24 months before the
> planned flight they:
> (1) completed at least 12 hours of flight time as PIC or flying dual or solo under the
> supervision of an FI(S), including, on TMGs, at least: (i) six hours flight time;
> (ii) 12 take-offs and landings; and (iii) a training flight of at least one hour total
> flight time with an instructor; or
> (2) passed a proficiency check with an examiner … based on the skill test as specified
> in point SFCL.150(b)(2).

- The 12 h may include sailplane hours; 6 h, 12 take-offs and landings and the training
  flight must be on TMGs.
- The training flight is one flight of at least 1 h total time, not 1 h of dual time
  added up over several flights.

### (c) Part-FCL TMG holders

SPL holders who also hold a Part-FCL licence with TMG privileges (e.g. PPL(A) or LAPL(A)
with a TMG class rating) are exempt from (b); the Part-FCL rules for that licence apply
instead (FCL.740.A or FCL.140.A).

### (d) Logbook

Dual flights, supervised flights, training flights and proficiency checks under (a) and (b)
must be entered in the logbook and signed by the FI(S) or FE(S).

### (e) Passengers

> SPL holders shall carry passengers only if in the preceding 90 days they have carried out
> as PIC, at least: (1) three launches in sailplanes, excluding TMGs, if passengers are to
> be carried in sailplanes, excluding TMGs; or (2) three take-offs and landings in TMGs, if
> passengers are to be carried in a TMG. For carrying passengers at night in a TMG, at
> least one of those take-offs and landings shall be carried out at night.

SFCL.115(a)(2) additionally requires, once after licence issue, 10 h or 30 launches as PIC
plus a passenger-competence training flight (or an FI(S)/BI(S) certificate).

## How NinerLog implements it

Code: `internal/service/currency/easa.go` (`easaSPLRule`, `easaSPLTMGRule`,
`easaLaunchMethodCurrency`, `EvaluatePassengerCurrency`); data:
`internal/repository/postgres/currency_flight_data.go`.

| Rule | Requirement (`nameKey`) | Measured as | Classes |
| --- | --- | --- | --- |
| SFCL.160(a)(1) | `requirement.flight_time` ≥ 300 min | PIC + dual minutes | `GLIDER` + `TMG` |
| SFCL.160(a)(1)(i) | `requirement.launches` ≥ 15 | take-offs, at least one per flight | `GLIDER` |
| SFCL.160(a)(1)(ii) | `requirement.training_flights` ≥ 2 | flights with dual time | `GLIDER` |
| SFCL.160(a)(2) | `requirement.proficiency_check` | a proficiency check flight in 24 months | `GLIDER` |
| SFCL.160(b)(1) | `requirement.flight_time` ≥ 720 min | PIC + dual minutes | `GLIDER` + `TMG` |
| SFCL.160(b)(1)(i) | `requirement.tmg_time` ≥ 360 min | PIC + dual minutes | `TMG` |
| SFCL.160(b)(1)(ii) | `requirement.tmg_landings` ≥ 12 | landings | `TMG` |
| SFCL.160(b)(1)(iii) | `requirement.tmg_training_flight` ≥ 60 min | longest total time of a flight with dual time | `TMG` |
| SFCL.160(b)(2) | `requirement.proficiency_check` | a proficiency check flight in 24 months | `TMG` |
| SFCL.160(c) | `rating.sfcl_tmg_exempt`, no requirements | the pilot holds a `TMG` rating on a Part-FCL licence | — |
| SFCL.155(c) | `launchMethodCurrency[]` | take-offs per `launchMethod`; `self-launch` adds `TMG` take-offs | `GLIDER` (+ `TMG`) |
| SFCL.160(e)(1) | passenger currency, `easa_spl_pax` | landing days of flights with PIC time | `GLIDER` |
| SFCL.160(e)(2) | passenger currency, `easa_spl_tmg_pax` | landing days of flights with PIC time | `TMG`, SPL licence only |

- Recency is met by either all experience rows or the proficiency check.
- A supervised solo flight has no instructor on board, so NinerLog logs it as PIC time.
- The glider rule applies to a `GLIDER` rating on any licence and to every non-TMG rating
  on an `SPL` or `LAPL(S)` licence; only a `GLIDER` rating pools TMG hours.
- `launchMethodCurrency` lists every method the pilot has ever logged on the rating's
  class, so a lapsed method shows as `0 / 5`. It is informational: the rating status does
  not depend on it, because NinerLog doesn't know which methods the pilot is trained for.
- Passenger currency for `GLIDER` never reports night privilege.
- SFCL.160(c) is applied by `Service.EvaluateAll`: an SPL TMG result is reported current
  with `rating.sfcl_tmg_exempt` when any EASA-evaluated licence that is neither a sailplane
  nor an ultralight licence (PPL, LAPL, CPL, ATPL) carries a `TMG` rating.
- AMC1 SFCL.160 credits Annex I sailplanes toward the hourly requirements only. PIC time on
  `ULTRALIGHT` aircraft of kind `SAILPLANE` or `THREE_AXIS_MOTORGLIDER` counts toward the 5 h
  of (a) and the 12 h of (b), and `THREE_AXIS_MOTORGLIDER` PIC time toward the 6 h on TMGs;
  their dual time, launches, landings and training flights never count, because a training
  flight needs an ORA.ATO.135-authorised aircraft. The result lists the kinds in
  `creditedUltralightKinds`.

### Known gaps

- SFCL.160(e)(2) night passenger carriage in a TMG is not evaluated for SPL holders.
- SFCL.155(a) initial launch-method training and SFCL.115(a)(2) passenger prerequisites are
  not tracked.
- "Solo under the supervision of an FI(S)" logged without PIC time (e.g. a student's SPIC
  column) is not counted toward the hour requirements.
