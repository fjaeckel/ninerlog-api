# Changelog

Written for pilots: one entry per change in what NinerLog tells you about your currency.
Each catalogue divergence from the API's former behaviour is listed as
`<rule id>#<divergence id>` (DESIGN.md decision 6); `rulescheck` fails when one is missing.

## Unreleased

### SEP and TMG class ratings (EASA FCL.740.A)

- A proficiency check with an examiner in the 3 months before expiry now revalidates your
  SEP or TMG rating on its own. `easa.part-fcl.fcl-740-a.sep#proficiency-check`
- The experience route now needs 12 take-offs as well as 12 landings.
  `easa.part-fcl.fcl-740-a.sep#takeoffs`
- A class or type rating proficiency check, skill test, EBT practical assessment or
  assessment of competence in any aeroplane in the 12 months before expiry now exempts you
  from the 1 hour of refresher training. `easa.part-fcl.fcl-740-a.sep#refresher-exemption`
- Ultralight (Annex I) hours still count toward the 12 hours and 6 hours as PIC, but no
  longer toward take-offs, landings or the refresher training.
  `easa.part-fcl.fcl-740-a.sep#ul-credit-time-only`
- Your rating is valid through its expiry date; it shows as expired from the next day.
  `easa.part-fcl.fcl-740-a.sep#valid-through-expiry`
- TMG and SEP flights count whatever their launch method.
  `easa.part-fcl.fcl-740-a.sep#towed-launches`
- SEP and TMG ratings recorded on a DULV or DAeC ultralight licence are no longer evaluated
  as Part-FCL class ratings. `easa.part-fcl.fcl-740-a.sep#authorities`

### Instrument recency (FAA 14 CFR 61.57(c) and (d))

- Instrument tasks now count in calendar months: tasks flown in January keep you current
  through 31 July. `faa.14cfr61.61-57-c.instrument#calendar-months`
- Intercepting and tracking courses is now a required task. If your flights do not record
  it, your instrument currency shows as unknown instead of current.
  `faa.14cfr61.61-57-c.instrument#intercept-track`
- The "lapsed, recoverable with a safety pilot" status now lasts six calendar months from
  the day you were last current, and only if you were current.
  `faa.14cfr61.61-57-c.instrument#grace-period`
- A logged instrument proficiency check now makes you current again, through the end of
  the sixth calendar month after the check. `faa.14cfr61.61-57-c.instrument#ipc-restores`
- Approaches, holds and tracking flown in a full flight simulator, flight training device
  or aviation training device now count. `faa.14cfr61.61-57-c.instrument#fstd`
- Only tasks in an airplane, helicopter, powered-lift or airship (or a device representing
  one) count; ultralight flights no longer do. `faa.14cfr61.61-57-c.instrument#category`
- An expiry date recorded on an FAA instrument rating is ignored; FAA ratings do not expire.
  `faa.14cfr61.61-57-c.instrument#no-expiry`
