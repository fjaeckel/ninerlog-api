### German ultralights, three-axis recency (LuftPersV § 45(2), (3))

- The 12 hours now need 12 take-offs as well as 12 landings.
  `de.luftpersv.45-2.three-axis#takeoffs`
- Ultralight flights without an ultralight kind no longer count, even when you hold only one
  ultralight kind; if they are all you flew, your recency shows as unknown and asks you to
  set the aircraft's kind. `de.luftpersv.45-2.three-axis#kindless-ul`
- SEP, TMG and ultralight flights count whatever their launch method (winch, aerotow, car or
  bungee). `de.luftpersv.45-2.three-axis#towed-launches`

### German ultralights, helicopter recency (LuftPersV § 45(2a), (3))

- The 6 hours now need 6 take-offs as well as 6 landings.
  `de.luftpersv.45-2a.helicopter#takeoffs`
- Ultralight flights without an ultralight kind no longer count toward UL helicopter
  recency. `de.luftpersv.45-2a.helicopter#kindless-ul`

### German ultralights, association recency rules (DULV, DAeC under LuftPersV § 45(4))

- Ultralight flights without an ultralight kind no longer count toward gyroplane, trike,
  powered paraglider or UL sailplane recency; if they are all you flew, your recency shows as
  unknown. `de.dulv.ul-recency.gyroplane#kindless-ul`,
  `de.dulv.ul-recency.weight-shift#kindless-ul`, `de.daec.ul-recency.weight-shift#kindless-ul`,
  `de.dulv.ul-recency.powered-paraglider#kindless-ul`, `de.daec.ul-recency.sailplane#kindless-ul`

### German ultralights, passengers (LuftPersV § 45a, § 84a)

- Only take-offs you logged count toward the 3 take-offs and 3 landings; a flight with no
  take-off logged no longer counts as one. `de.luftpersv.45a.passengers-three-axis#logged-takeoffs`,
  `de.luftpersv.45a.passengers#logged-takeoffs`
- Winch-, aerotow-, car- and bungee-launched flights count toward passenger recency for every
  ultralight kind. `de.luftpersv.45a.passengers-three-axis#towed-launches`,
  `de.luftpersv.45a.passengers#towed-launches`
- Ultralight flights without an ultralight kind no longer count toward passenger recency.
  `de.luftpersv.45a.passengers-three-axis#kindless-ul`, `de.luftpersv.45a.passengers#kindless-ul`
- If you fly three-axis ultralights and hold a valid PPL(A) or sailplane licence (SPL), your
  passenger authorisation counts as granted; you no longer need to record it.
  `de.luftpersv.45a.passengers-three-axis#deemed-authorisation`
- The progress toward the passenger authorisation counts only cross-country flights with an
  instructor flown after your ultralight licence was issued.
  `de.luftpersv.45a.passengers-three-axis#xc-after-licence`, `de.luftpersv.45a.passengers#xc-after-licence`
- The 200 km now apply to each of the two cross-country flights with an intermediate landing,
  not to all flights added up. `de.luftpersv.45a.passengers-three-axis#xc-distance-per-flight`,
  `de.luftpersv.45a.passengers#xc-distance-per-flight`

### German ultralights, towing rating (LuftPersV § 84(5))

- The 10 tows in 24 months are the statutory rule of § 84(5), and every tow you flew on an
  ultralight counts, whatever ultralight kind is stored with the rating.
  `de.luftpersv.84-5.ul-towing#tows-any-ultralight`

### Ratings of other authorities (expiry only)

- A rating tracked by its expiry date only is valid through that date and shows as expired
  from the next day. `other.ninerlog.expiry-only#valid-through-expiry`,
  `other.ninerlog.expiry-only.faa-ultralight#valid-through-expiry`
- A licence whose authority is entered in unusual case or with spaces (" easa ", "Faa") now
  gets its authority's rules instead of expiry-only tracking.
  `other.ninerlog.expiry-only#authority-case`

### New

- Ultralight instructor ratings (LuftPersV § 96, privilege UL_INSTRUCTOR) are now evaluated:
  valid for three years, extended with two of instruction given, a refresher course and an
  assessment of competence.
