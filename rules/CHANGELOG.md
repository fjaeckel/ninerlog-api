# Changelog

Written for pilots: one entry per change in what NinerLog tells you about your currency,
grouped by authority. Each catalogue divergence from the API's former behaviour has exactly
one line ending in its stable id `<rule id>#<divergence id>` (DESIGN.md decision 6 and
section 13); `rulescheck` fails when a divergence has no line, when a line names a
divergence that does not exist, and when a divergence is listed twice. Lines without an id
announce rules the API does not evaluate today.

## Unreleased

### EASA (Part-FCL, Part-SFCL, Part-MED)

#### SEP and TMG class ratings (EASA FCL.740.A)

- A proficiency check with an examiner in the 3 months before expiry now revalidates your SEP or TMG rating on its own. `easa.part-fcl.fcl-740-a.sep#proficiency-check`
- The experience route now needs 12 take-offs as well as 12 landings. `easa.part-fcl.fcl-740-a.sep#takeoffs`
- A class or type rating proficiency check, skill test, EBT practical assessment or assessment of competence in any aeroplane in the 12 months before expiry now exempts you from the 1 hour of refresher training. `easa.part-fcl.fcl-740-a.sep#refresher-exemption`
- Ultralight (Annex I) hours still count toward the 12 hours and 6 hours as PIC, but no longer toward take-offs, landings or the refresher training. `easa.part-fcl.fcl-740-a.sep#ul-credit-time-only`
- Your rating is valid through its expiry date; it shows as expired from the next day. `easa.part-fcl.fcl-740-a.sep#valid-through-expiry`
- TMG and SEP flights count whatever their launch method. `easa.part-fcl.fcl-740-a.sep#towed-launches`
- SEP and TMG ratings recorded on a DULV or DAeC ultralight licence are no longer evaluated as Part-FCL class ratings. `easa.part-fcl.fcl-740-a.sep#authorities`

#### Multi-engine class ratings (EASA FCL.740.A(a))

- An MEP rating now always needs a proficiency check (or EBT practical assessment) in the 3 months before expiry; 10 route sectors alone no longer revalidate it. `easa.part-fcl.fcl-740-a.mep#proficiency-check-required`
- The 1 hour of refresher training is no longer asked for an MEP rating; it is not in the regulation. `easa.part-fcl.fcl-740-a.mep#no-refresher`
- A proficiency check counts only within the 3 months before the expiry date, not anywhere in the last 12 months. `easa.part-fcl.fcl-740-a.mep#check-window`
- A proficiency check taken in a simulator representing the class now counts. `easa.part-fcl.fcl-740-a.mep#check-in-fstd`
- Only flights with at least 15 minutes of cruise count as route sectors; if your flights do not record cruise time, the route sectors show as unknown. `easa.part-fcl.fcl-740-a.mep#route-sector-definition`
- One route sector flown with an examiner (it may be the check flight, or in a full flight simulator) replaces the 10 route sectors. `easa.part-fcl.fcl-740-a.mep#examiner-sector`
- SET ratings are no longer evaluated together with MEP ratings; they have their own rule. `easa.part-fcl.fcl-740-a.mep#set-separate`
- Your MEP rating is valid through its expiry date; it shows as expired from the next day. `easa.part-fcl.fcl-740-a.mep#valid-through-expiry`
- Route sectors count whatever the launch method of the flight. `easa.part-fcl.fcl-740-a.mep#towed-launches`
- MEP ratings recorded on a DULV or DAeC ultralight licence are no longer evaluated as Part-FCL class ratings. `easa.part-fcl.fcl-740-a.mep#authorities`

#### Single-engine turbo-prop class ratings (EASA FCL.740.A(b)(3))

- An SET rating is revalidated only by a proficiency check with an examiner; route sectors and refresher training no longer count. `easa.part-fcl.fcl-740-a.set#proficiency-check-only`
- The proficiency check counts only within the 3 months before the expiry date. `easa.part-fcl.fcl-740-a.set#check-window`
- Your SET rating is valid through its expiry date; it shows as expired from the next day. `easa.part-fcl.fcl-740-a.set#valid-through-expiry`
- SET ratings recorded on a DULV or DAeC licence are no longer evaluated. `easa.part-fcl.fcl-740-a.set#authorities`

#### Instrument rating (EASA FCL.625, FCL.625.A, FCL.625.H)

- The 10 hours of IFR flight time are no longer required to revalidate an IR(A); the IR proficiency check is what counts. `easa.part-fcl.fcl-625-a.ir#no-ifr-hours`
- Only a proficiency check recorded as an IR check revalidates your IR(A); a class rating check alone no longer does. `easa.part-fcl.fcl-625-a.ir#ir-check-only`
- The IR(A) check counts only within the 3 months before the expiry date. `easa.part-fcl.fcl-625-a.ir#check-window`
- An IR(A) check in a simulator (FNPT II or FFS) now counts. `easa.part-fcl.fcl-625-a.ir#fstd`
- Your IR(A) is valid through its expiry date; it shows as expired from the next day. `easa.part-fcl.fcl-625-a.ir#valid-through-expiry`
- IRs recorded on a DULV or DAeC licence are no longer evaluated. `easa.part-fcl.fcl-625-a.ir#authorities`
- An IR on a helicopter licence is now evaluated as an IR(H), with a check in a helicopter. `easa.part-fcl.fcl-625-a.ir#helicopter-licences`
- The 10 hours of IFR flight time are no longer required to revalidate an IR(H). `easa.part-fcl.fcl-625-h.ir#no-ifr-hours`
- Only a proficiency check recorded as an IR check in a helicopter revalidates your IR(H). `easa.part-fcl.fcl-625-h.ir#ir-check-only`
- The IR(H) check counts only within the 3 months before the expiry date. `easa.part-fcl.fcl-625-h.ir#check-window`
- An IR(H) check in a simulator (FTD 2/3 or FFS) now counts. `easa.part-fcl.fcl-625-h.ir#fstd`
- Your IR(H) is valid through its expiry date; it shows as expired from the next day. `easa.part-fcl.fcl-625-h.ir#valid-through-expiry`
- IRs recorded on a DULV or DAeC helicopter licence are no longer evaluated. `easa.part-fcl.fcl-625-h.ir#authorities`

#### LAPL(A) recency (EASA FCL.140.A)

- Only time as PIC, dual or supervised solo counts toward the 12 hours; co-pilot, PICUS and examiner time no longer do. `easa.part-fcl.fcl-140-a.lapl-a#pic-dual-spic-time`
- The 12 take-offs and landings now need take-offs as well as landings. `easa.part-fcl.fcl-140-a.lapl-a#takeoffs`
- The 1 hour of refresher training may now be flown over several flights. `easa.part-fcl.fcl-140-a.lapl-a#refresher-cumulative`
- Ultralight hours still count toward the 12 hours, but no longer toward take-offs and landings. `easa.part-fcl.fcl-140-a.lapl-a#ul-credit-time-only`
- With both SEP(land) and SEP(sea), each class now needs 1 hour and 6 take-offs and landings. `easa.part-fcl.fcl-140-a.lapl-a#land-sea-takeoffs`
- Flights count whatever their launch method. `easa.part-fcl.fcl-140-a.lapl-a#towed-launches`
- LAPL(A) privileges recorded on a DULV or DAeC licence are no longer evaluated. `easa.part-fcl.fcl-140-a.lapl-a#authorities`
- A flight on the first day of the 2 years now counts; before, the period started at noon on that day. `easa.part-fcl.fcl-140-a.lapl-a#window-first-day`

#### Passenger recency (EASA FCL.060(b))

- Carrying passengers now needs three take-offs as well as three landings in the class within 90 days. `easa.part-fcl.fcl-060-b.passengers#takeoffs`
- Only take-offs and landings you made as pilot flying count; if your flights do not record that, passenger recency shows as unknown instead of current. `easa.part-fcl.fcl-060-b.passengers#pilot-flying`
- Take-offs and landings in a full flight simulator of the class now count; other simulators do not. `easa.part-fcl.fcl-060-b.passengers#ffs`
- Day and night passenger recency are now two separate entries; the day entry says only whether you may carry passengers by day. `easa.part-fcl.fcl-060-b.passengers#night-separate`
- Classes recorded on a DULV or DAeC licence are no longer evaluated under FCL.060. `easa.part-fcl.fcl-060-b.passengers#authorities`
- Night passenger recency now needs a night take-off as well as a night landing. `easa.part-fcl.fcl-060-b.night-passengers#takeoffs`
- Only a night take-off and landing as pilot flying count. `easa.part-fcl.fcl-060-b.night-passengers#pilot-flying`
- A night take-off and landing in a full flight simulator of the class now counts. `easa.part-fcl.fcl-060-b.night-passengers#ffs`
- Night passenger recency is now shown for every licence, including LAPL; whether you may fly at night at all (night rating) is shown separately. `easa.part-fcl.fcl-060-b.night-passengers#night-privilege`
- When NinerLog evaluates another date, your IR's validity on that date decides the night waiver, not today's. `easa.part-fcl.fcl-060-b.night-passengers#ir-validity-date`
- An IR recorded without an expiry date now waives the night requirement. `easa.part-fcl.fcl-060-b.night-passengers#ir-without-expiry`
- Classes recorded on a DULV or DAeC licence are no longer evaluated under FCL.060. `easa.part-fcl.fcl-060-b.night-passengers#authorities`

#### LAPL(H) recency (EASA FCL.140.H)

- Helicopter types on a LAPL(H) now show recency: 6 hours in the type with 6 take-offs and landings plus 1 hour of refresher training, or a proficiency check on the type, in the last 12 months. The LAPL(H) and its types do not expire. `easa.part-fcl.fcl-140-h.lapl-h#recency`

#### Class and type rating validity (EASA FCL.740)

- Ultralight ratings recorded on an EASA licence no longer get a Part-FCL expiry result. `easa.part-fcl.fcl-740.validity#ultralight`
- Helicopter, powered-lift and airship ratings are now revalidated by their own rules instead of by expiry date only. `easa.part-fcl.fcl-740.validity#rotorcraft-and-airships`
- Type ratings are valid through their expiry date; they show as expired from the next day. `easa.part-fcl.fcl-740.validity#valid-through-expiry`
- Type ratings recorded on a DULV or DAeC licence are no longer evaluated. `easa.part-fcl.fcl-740.validity#authorities`
- New: helicopter type ratings need 2 hours in the type within the validity period and a proficiency check in the 3 months before expiry; for single-engine types up to 3 175 kg, 6 hours as PIC and 1 hour of refresher training in the 3 months before expiry also revalidate (record the engine count and maximum take-off mass of your helicopter). Helicopter types on a LAPL(H) follow the LAPL(H) recency instead. `easa.part-fcl.fcl-740-h.helicopter#revalidation`
- Helicopter type ratings are valid through their expiry date. `easa.part-fcl.fcl-740-h.helicopter#valid-through-expiry`
- New: powered-lift type ratings need a proficiency check in the 3 months before expiry and 10 route sectors (or 1 with an examiner). `easa.part-fcl.fcl-740-pl.powered-lift#revalidation`
- Powered-lift type ratings are valid through their expiry date. `easa.part-fcl.fcl-740-pl.powered-lift#valid-through-expiry`
- New: airship type ratings need a proficiency check in the 3 months before expiry and 2 hours in the type within the validity period. `easa.part-fcl.fcl-740-as.airship#revalidation`
- Airship type ratings are valid through their expiry date. `easa.part-fcl.fcl-740-as.airship#valid-through-expiry`

#### Towing ratings (EASA FCL.805)

- The sailplane towing rating of an aeroplane licence now follows FCL.805 (5 tows in 24 months), not the sailplane rules. `easa.part-fcl.fcl-805.sailplane-towing#fcl-805`
- Only sailplane tows count toward the sailplane towing rating; a tow flight that does not say what it towed shows as unknown. `easa.part-fcl.fcl-805.sailplane-towing#tow-kind`
- Only tows flown in an aeroplane or TMG count. `easa.part-fcl.fcl-805.sailplane-towing#aeroplanes-only`
- The banner towing rating of an aeroplane licence now follows FCL.805. `easa.part-fcl.fcl-805.banner-towing#fcl-805`
- Only banner tows count toward the banner towing rating. `easa.part-fcl.fcl-805.banner-towing#tow-kind`
- Only banner tows flown in an aeroplane or TMG count. `easa.part-fcl.fcl-805.banner-towing#aeroplanes-only`
- A tow on the first day of the 24 months now counts; before, the period started at noon on that day. *Banner towing rating recency (Part-FCL).* `easa.part-fcl.fcl-805.banner-towing#window-first-day`
- A tow on the first day of the 24 months now counts; before, the period started at noon on that day. *Sailplane towing rating recency (Part-FCL).* `easa.part-fcl.fcl-805.sailplane-towing#window-first-day`

#### Medical certificates (EASA MED.A.045)

- The validity of a class 1 medical now follows from your age (12 months, 6 from age 60); an entered expiry date later than that is capped. `easa.part-med.med-a-045.class-1#derived-validity`
- A class 1 medical is valid through its expiry date. `easa.part-med.med-a-045.class-1#valid-through-expiry`
- The validity of a class 2 medical now follows from your age (60, 24 or 12 months, ending at 42 or 51 at the latest); an entered expiry date later than that is capped. `easa.part-med.med-a-045.class-2#derived-validity`
- A class 2 medical is valid through its expiry date. `easa.part-med.med-a-045.class-2#valid-through-expiry`
- The validity of a LAPL medical now follows from your age (60 months, ending at 42 at the latest, or 24 months from 40); an entered expiry date later than that is capped. `easa.part-med.med-a-045.lapl#derived-validity`
- A LAPL medical is valid through its expiry date. `easa.part-med.med-a-045.lapl#valid-through-expiry`

#### Sailplane recency (EASA SFCL.160(a))

- Hours on ultralight sailplanes and ultralight motorgliders no longer count toward the 5 hours; Part-SFCL has no rule crediting Annex I aircraft. `easa.part-sfcl.sfcl-160-a.sailplane#no-ul-credit`
- Only GLIDER ratings are evaluated as sailplane privileges; an SEP or other rating recorded on an SPL is no longer treated as a sailplane rating. `easa.part-sfcl.sfcl-160-a.sailplane#glider-class-only`
- GLIDER ratings recorded on a DULV or DAeC ultralight licence are no longer evaluated as SPL privileges. `easa.part-sfcl.sfcl-160-a.sailplane#authorities`
- A flight on the first day of the 24 months now counts; before, the period started at noon on that day. `easa.part-sfcl.sfcl-160-a.sailplane#window-first-day`

#### SPL TMG recency (EASA SFCL.160(b), (c))

- The 12 take-offs and landings on TMGs now need 12 take-offs as well as 12 landings. `easa.part-sfcl.sfcl-160-b.tmg#takeoffs`
- Ultralight sailplane and motorglider hours no longer count toward the 12 hours or the 6 hours on TMGs. `easa.part-sfcl.sfcl-160-b.tmg#no-ul-credit`
- The exemption for pilots who also hold a Part-FCL TMG rating now needs that rating to be within its expiry date and on a PPL(A), LAPL(A), CPL(A), ATPL(A) or MPL. `easa.part-sfcl.sfcl-160-b.tmg#exemption-valid-rating`
- TMG ratings on sailplane licences recorded under DULV or DAeC are no longer evaluated. `easa.part-sfcl.sfcl-160-b.tmg#authorities`
- A flight on the first day of the 24 months now counts; before, the period started at noon on that day. `easa.part-sfcl.sfcl-160-b.tmg#window-first-day`

#### SPL passengers (EASA SFCL.160(e), SFCL.115(a)(2))

- Sailplane passenger recency now counts your launches as PIC, not your landings: a winch series logged as one row counts every launch. `easa.part-sfcl.sfcl-160-e-1.sailplane-passengers#launches`
- Sailplane passenger recency is no longer evaluated for GLIDER ratings on DULV or DAeC licences. `easa.part-sfcl.sfcl-160-e-1.sailplane-passengers#authorities`
- TMG passenger recency now needs three take-offs as well as three landings as PIC, and for night passengers one take-off and one landing at night. `easa.part-sfcl.sfcl-160-e-2.tmg-passengers#takeoffs`
- TMG passenger recency is no longer evaluated for TMG ratings on sailplane licences recorded under DULV or DAeC. `easa.part-sfcl.sfcl-160-e-2.tmg-passengers#authorities`
- The 10 hours or 30 launches as PIC since your SPL was issued are now required before you carry passengers, not only shown. `easa.part-sfcl.sfcl-115-a-2.passengers#enforced`
- An FI(S) certificate satisfies that prerequisite on its own. `easa.part-sfcl.sfcl-115-a-2.passengers#fi-s-alternative`

#### Launch methods (EASA SFCL.155)

- Launch methods of GLIDER ratings on DULV or DAeC licences are no longer listed. *SPL launch method recency, winch, car and aerotow.* `easa.part-sfcl.sfcl-155-c.launch-method#authorities`
- Launch methods of GLIDER ratings on DULV or DAeC licences are no longer listed. *SPL launch method recency, self-launch.* `easa.part-sfcl.sfcl-155-c.self-launch#authorities`
- Launch methods of GLIDER ratings on DULV or DAeC licences are no longer listed. *SPL launch method recency, bungee.* `easa.part-sfcl.sfcl-155-c.bungee#authorities`
- A launch on the first day of the two years now counts; before, the period started at noon on that day. *SPL launch method recency, winch, car and aerotow.* `easa.part-sfcl.sfcl-155-c.launch-method#window-first-day`
- A launch on the first day of the two years now counts; before, the period started at noon on that day. *SPL launch method recency, self-launch.* `easa.part-sfcl.sfcl-155-c.self-launch#window-first-day`
- A launch on the first day of the two years now counts; before, the period started at noon on that day. *SPL launch method recency, bungee.* `easa.part-sfcl.sfcl-155-c.bungee#window-first-day`

#### Gyroplanes (EASA FCL.240.G, FCL.205.G)

- The 12 hours now count only time as PIC, dual or solo under supervision. `easa.part-fcl.fcl-240-g.gpl#pic-dual-spic`
- The 12 take-offs and landings now need 12 take-offs as well as 12 landings. `easa.part-fcl.fcl-240-g.gpl#takeoffs`
- GYROPLANE ratings on DULV or DAeC licences are no longer evaluated as GPL recency. `easa.part-fcl.fcl-240-g.gpl#authorities`
- New: each SPG variant you extended your GPL to shows whether you flew it in the last 2 years (FCL.240.G(b)).
- A flight on the first day of the 2 years now counts; before, the period started at noon on that day. `easa.part-fcl.fcl-240-g.gpl#window-first-day`

#### Towing ratings (EASA SFCL.205)

- Towing recency now cites SFCL.205(f). *Sailplane towing rating recency.* `easa.part-sfcl.sfcl-205-f.sailplane-towing#article`
- Towing recency now cites SFCL.205(f). *Banner towing rating recency.* `easa.part-sfcl.sfcl-205-f.banner-towing#article`
- SFCL.205 now applies only to towing ratings on an SPL; towing ratings on a PPL(A) or LAPL(A) follow FCL.805. *Sailplane towing rating recency.* `easa.part-sfcl.sfcl-205-f.sailplane-towing#licence-scope`
- SFCL.205 now applies only to towing ratings on an SPL; towing ratings on a PPL(A) or LAPL(A) follow FCL.805. *Banner towing rating recency.* `easa.part-sfcl.sfcl-205-f.banner-towing#licence-scope`
- A lapsed banner towing rating asks for the missing tows, without the instructor that SFCL.205(g) requires only for sailplane towing. `easa.part-sfcl.sfcl-205-f.banner-towing#remedy`
- A tow on the first day of the two years now counts; before, the period started at noon on that day. *Sailplane towing rating recency.* `easa.part-sfcl.sfcl-205-f.sailplane-towing#window-first-day`
- A tow on the first day of the two years now counts; before, the period started at noon on that day. *Banner towing rating recency.* `easa.part-sfcl.sfcl-205-f.banner-towing#window-first-day`
- A tow flight that records several towed sailplanes now counts one tow for each; one without a count is one tow. *Sailplane towing rating recency.* `easa.part-sfcl.sfcl-205-f.sailplane-towing#towed-gliders`
- A tow flight that records several towed sailplanes now counts one tow for each; one without a count is one tow. *Banner towing rating recency.* `easa.part-sfcl.sfcl-205-f.banner-towing#towed-gliders`

#### Cloud flying (EASA SFCL.215)

- A cloud flying proficiency check with an FE(S) now restores the privilege for 24 months. `easa.part-sfcl.sfcl-215.cloud-flying#proficiency-check`
- Cloud flights with an FI(S) (dual) now count, as the way back after a lapse. `easa.part-sfcl.sfcl-215.cloud-flying#dual-with-instructor`
- A valid BIR or IR(A) now fully credits the cloud flying recency. `easa.part-sfcl.sfcl-215.cloud-flying#bir-ir-credit`
- Cloud flying privileges on FAA or ultralight licences are no longer evaluated. `easa.part-sfcl.sfcl-215.cloud-flying#authorities`
- A cloud flight on the first day of the two years now counts; before, the period started at noon on that day. `easa.part-sfcl.sfcl-215.cloud-flying#window-first-day`

#### FI(S) and FE(S) certificates (EASA SFCL.360, SFCL.460)

- Instructor refresher training within the last 3 years is now required for your FI(S) privileges, not only shown. `easa.part-sfcl.sfcl-360.fi-s#refresher-required`
- The demonstration of instructional ability within the last 9 years (or an assessment of competence) is now checked. `easa.part-sfcl.sfcl-360.fi-s#nine-year-demonstration`
- Your hours as FE(S) now count toward the 30 hours of instruction. `easa.part-sfcl.sfcl-360.fi-s#examiner-hours`
- Refresher training plus an assessment of competence now restores lapsed FI(S) privileges. `easa.part-sfcl.sfcl-360.fi-s#resumption`
- FI_S privileges on FAA or ultralight licences are no longer evaluated. `easa.part-sfcl.sfcl-360.fi-s#authorities`
- An FE(S) certificate is now valid for five years from its validity start, even without a recorded expiry date. `easa.part-sfcl.sfcl-460.fe-s#five-year-validity`
- The FE(S) revalidation (examiner refresher course and a demonstration within 24 months before expiry) is now shown. `easa.part-sfcl.sfcl-460.fe-s#revalidation`

#### Training progress (EASA SFCL.130, SFCL.150)

- SPL training progress now counts TMG instruction toward the 15 hours, the 10 hours dual, the 2 hours supervised solo and the 45 launches, and a dual cross-country flight of 100 km in a TMG; at least 7 hours of instruction, including 3 hours dual, must still be flown in sailplanes other than TMGs. `easa.part-sfcl.sfcl-130.spl-training#tmg-instruction`
- A cross-country flight without a recorded distance no longer completes the SPL training or the TMG extension; the programme shows as unknown until you record the distance. *SPL training course flight instruction.* `easa.part-sfcl.sfcl-130.spl-training#distance-unknown`
- A cross-country flight without a recorded distance no longer completes the SPL training or the TMG extension; the programme shows as unknown until you record the distance. *SPL extension to TMGs.* `easa.part-sfcl.sfcl-150-b.tmg-extension#distance-unknown`
- The TMG extension now also needs a skill test in a TMG. `easa.part-sfcl.sfcl-150-b.tmg-extension#skill-test`
- A Part-FCL TMG class rating now fully credits the TMG extension. `easa.part-sfcl.sfcl-150-b.tmg-extension#part-fcl-credit`

### FAA (14 CFR Parts 61 and 68)

#### Instrument recency (FAA 14 CFR 61.57(c) and (d))

- Instrument tasks now count in calendar months: tasks flown in January keep you current through 31 July. `faa.14cfr61.61-57-c.instrument#calendar-months`
- Intercepting and tracking courses is now a required task. If your flights do not record it, your instrument currency shows as unknown instead of current. `faa.14cfr61.61-57-c.instrument#intercept-track`
- The "lapsed, recoverable with a safety pilot" status now lasts six calendar months from the day you were last current, and only if you were current. `faa.14cfr61.61-57-c.instrument#grace-period`
- A logged instrument proficiency check now makes you current again, through the end of the sixth calendar month after the check. `faa.14cfr61.61-57-c.instrument#ipc-restores`
- Approaches, holds and tracking flown in a full flight simulator, flight training device or aviation training device now count. `faa.14cfr61.61-57-c.instrument#fstd`
- Only tasks in an airplane, helicopter, powered-lift or airship (or a device representing one) count; ultralight flights no longer do. `faa.14cfr61.61-57-c.instrument#category`
- An expiry date recorded on an FAA instrument rating is ignored; FAA ratings do not expire. `faa.14cfr61.61-57-c.instrument#no-expiry`

#### Passenger recency (FAA 14 CFR 61.57(a) and (b))

- Carrying passengers now needs three takeoffs as well as three landings in the class within 90 days. `faa.14cfr61.61-57-a.passengers#takeoffs`
- Only takeoffs and landings you made as sole manipulator of the controls count; if your flights do not record that, passenger recency shows as unknown instead of current. `faa.14cfr61.61-57-a.passengers#sole-manipulator`
- Missing passenger landings no longer mark your FAA class rating itself as expired or expiring; FAA ratings have no recency of their own, only carrying passengers is restricted. `faa.14cfr61.61-57-a.passengers#rating-status`
- Day and night passenger recency are now two separate entries; the day entry says only whether you may carry passengers by day. `faa.14cfr61.61-57-a.passengers#night-separate`
- Unmet passenger recency is shown as lapsed rather than expired: your certificate and ratings stay valid. `faa.14cfr61.61-57-a.passengers#status-lapsed`
- Student pilot certificates are no longer evaluated for passenger recency; student pilots may not carry passengers. `faa.14cfr61.61-57-a.passengers#student`
- Gliders: you now need three launches as well as three landings in 90 days to carry passengers. `faa.14cfr61.61-57-a.glider-passengers#launches`
- Gliders: only launches and landings you flew as sole manipulator count (PIC time alone no longer does); without that information the status is unknown. `faa.14cfr61.61-57-a.glider-passengers#sole-manipulator`
- A powered class on a certificate recorded as "GLIDER" is now evaluated like any powered class, not like a glider. `faa.14cfr61.61-57-a.glider-passengers#glider-licence-classes`
- Unmet glider passenger recency is shown as lapsed rather than expired. `faa.14cfr61.61-57-a.glider-passengers#status-lapsed`
- New: to carry passengers in a tailwheel airplane you need three full-stop landings in a tailwheel airplane within 90 days; NinerLog shows this for each airplane class in which you have flown a tailwheel airplane. `faa.14cfr61.61-57-a.tailwheel#tailwheel`
- For a type-rated aircraft (a rating that carries a type designator), passenger recency is now shown per type: only takeoffs and landings in that type count, not those in other aircraft of the same class. `faa.14cfr61.61-57-a.passengers-type#per-type`
- For a type-rated aircraft, night passenger recency is now shown per type: only night takeoffs and full-stop landings in that type count. `faa.14cfr61.61-57-b.night-passengers-type#per-type`
- Night passenger recency now counts only landings to a full stop; if your flights do not record full-stop night landings, it shows as unknown instead of current. `faa.14cfr61.61-57-b.night-passengers#full-stop`
- Night passenger recency now needs three night takeoffs as well. `faa.14cfr61.61-57-b.night-passengers#night-takeoffs`
- Only night takeoffs and landings you made as sole manipulator count. `faa.14cfr61.61-57-b.night-passengers#sole-manipulator`
- Sport pilots with the 61.329 night endorsement now get night passenger recency. `faa.14cfr61.61-57-b.night-passengers#sport-night`
- For recreational pilots, and sport pilots without the night endorsement, night passenger recency is shown as not applicable (no night privilege) instead of unknown. `faa.14cfr61.61-57-b.night-passengers#no-night-privilege`
- Unmet night passenger recency is shown as lapsed. `faa.14cfr61.61-57-b.night-passengers#status-lapsed`

#### Flight review (FAA 14 CFR 61.56)

- A pilot proficiency check or practical test with an examiner, or a completed WINGS phase, within the 24 calendar months now counts in place of a flight review. `faa.14cfr61.61-56.flight-review#substitutes`
- Three instructional glider flights no longer make a glider rating current on their own; they replace only the flight-training hour within a flight review, and the review itself must be logged. `faa.14cfr61.61-56.flight-review#glider-alternative`
- The flight review is shown once per pilot, no longer again for each glider rating. `faa.14cfr61.61-56.flight-review#per-pilot`
- Student pilots no longer see a flight review requirement. `faa.14cfr61.61-56.flight-review#student`
- A flight review logged on a flight simulator or flight training device now counts. `faa.14cfr61.61-56.flight-review#simulator`

#### Glider towing (FAA 14 CFR 61.69)

- Tows count toward towing recency only when flown accompanied by a qualified tow pilot. `faa.14cfr61.61-69.towing#accompanied`
- Only tows of gliders or unpowered ultralights count, not banner tows; a tow flight that does not say what it towed is not counted. `faa.14cfr61.61-69.towing#tow-kind`
- A tow flight that records several towed gliders now counts one tow for each; one without a count is one tow. `faa.14cfr61.61-69.towing#towed-gliders`

#### Instrument rating on sport and recreational certificates (FAA 14 CFR 61.315, 61.101)

- An instrument rating on a sport or recreational certificate is shown as not applicable instead of unknown. `faa.14cfr61.61-315.ir-not-applicable#status`

#### Medical certificates and authorizations (FAA 14 CFR 61.23, 61.21)

- Your first-class medical's duration for ATP privileges is now worked out from the examination date and your age (12 months under 40, 6 months from 40), valid through the last day of the month. `faa.14cfr61.61-23-d.first-class-atp#derived-expiry`
- Your first- or second-class medical's duration for commercial privileges is now worked out from the examination date (12 months). `faa.14cfr61.61-23-d.commercial#derived-expiry`
- Your medical's duration for private, recreational, student, sport and instructor privileges is now worked out from the examination date and your age (60 months under 40, 24 months from 40). `faa.14cfr61.61-23-d.private#derived-expiry`
- Category II and III authorizations now expire at the end of the sixth calendar month after issue or renewal, and are valid through that day. `faa.14cfr61.61-21.cat-ii-iii#derived-expiry`

#### Instructors (FAA 14 CFR 61.197, 61.217)

- Flight instructor certificates issued since 1 December 2024 have no expiration date, but you may instruct only with a flight instructor practical test or refresher course within the preceding 24 calendar months; NinerLog now shows this. `faa.14cfr61.61-197.cfi-recent-experience#recent-experience`
- Ground instructors now need instructing activity, a refresher course or an instructor's endorsement within the preceding 12 calendar months; NinerLog does not record ground instruction, so without logged flight instruction, a refresher course or an endorsement the status shows as unknown. `faa.14cfr61.61-217.ground-instructor#recent-experience`

### Germany (LuftPersV; DULV and DAeC rules)

#### German ultralights, three-axis recency (LuftPersV § 45(2), (3))

- The 12 hours now need 12 take-offs as well as 12 landings. `de.luftpersv.45-2.three-axis#takeoffs`
- Ultralight flights without an ultralight kind no longer count, even when you hold only one ultralight kind; if they are all you flew, your recency shows as unknown and asks you to set the aircraft's kind. `de.luftpersv.45-2.three-axis#kindless-ul`
- SEP, TMG and ultralight flights count whatever their launch method (winch, aerotow, car or bungee). `de.luftpersv.45-2.three-axis#towed-launches`
- A flight on the first day of the 24 months now counts; before, the period started at noon on that day. `de.luftpersv.45-2.three-axis#window-first-day`

#### German ultralights, helicopter recency (LuftPersV § 45(2a), (3))

- The 6 hours now need 6 take-offs as well as 6 landings. `de.luftpersv.45-2a.helicopter#takeoffs`
- Ultralight flights without an ultralight kind no longer count toward UL helicopter recency. `de.luftpersv.45-2a.helicopter#kindless-ul`
- A flight on the first day of the 12 months now counts; before, the period started at noon on that day. `de.luftpersv.45-2a.helicopter#window-first-day`

#### German ultralights, association recency rules (DULV, DAeC under LuftPersV § 45(4))

- Ultralight flights without an ultralight kind no longer count toward gyroplane, trike, powered paraglider or UL sailplane recency; if they are all you flew, your recency shows as unknown. *Ultralight recency, gyroplanes (DULV).* `de.dulv.ul-recency.gyroplane#kindless-ul`
- Ultralight flights without an ultralight kind no longer count toward gyroplane, trike, powered paraglider or UL sailplane recency; if they are all you flew, your recency shows as unknown. *Ultralight recency, weight-shift trikes (DULV).* `de.dulv.ul-recency.weight-shift#kindless-ul`
- Ultralight flights without an ultralight kind no longer count toward gyroplane, trike, powered paraglider or UL sailplane recency; if they are all you flew, your recency shows as unknown. *Ultralight recency, weight-shift trikes (DAeC).* `de.daec.ul-recency.weight-shift#kindless-ul`
- Ultralight flights without an ultralight kind no longer count toward gyroplane, trike, powered paraglider or UL sailplane recency; if they are all you flew, your recency shows as unknown. *Ultralight recency, powered paragliders.* `de.dulv.ul-recency.powered-paraglider#kindless-ul`
- Ultralight flights without an ultralight kind no longer count toward gyroplane, trike, powered paraglider or UL sailplane recency; if they are all you flew, your recency shows as unknown. *Ultralight recency, ultralight sailplanes (DAeC).* `de.daec.ul-recency.sailplane#kindless-ul`
- A flight on the first day of the 12 or 24 months now counts; before, the period started at noon on that day. *Ultralight recency, gyroplanes (DULV).* `de.dulv.ul-recency.gyroplane#window-first-day`
- A flight on the first day of the 12 or 24 months now counts; before, the period started at noon on that day. *Ultralight recency, weight-shift trikes (DULV).* `de.dulv.ul-recency.weight-shift#window-first-day`
- A flight on the first day of the 12 or 24 months now counts; before, the period started at noon on that day. *Ultralight recency, weight-shift trikes (DAeC).* `de.daec.ul-recency.weight-shift#window-first-day`
- A flight on the first day of the 12 or 24 months now counts; before, the period started at noon on that day. *Ultralight recency, powered paragliders.* `de.dulv.ul-recency.powered-paraglider#window-first-day`
- A flight on the first day of the 12 or 24 months now counts; before, the period started at noon on that day. *Ultralight recency, ultralight sailplanes (DAeC).* `de.daec.ul-recency.sailplane#window-first-day`

#### German ultralights, passengers (LuftPersV § 45a, § 84a)

- Only take-offs you logged count toward the 3 take-offs and 3 landings; a flight with no take-off logged no longer counts as one. *Ultralight passengers, three-axis (recency and passenger authorisation).* `de.luftpersv.45a.passengers-three-axis#logged-takeoffs`
- Only take-offs you logged count toward the 3 take-offs and 3 landings; a flight with no take-off logged no longer counts as one. *Ultralight passengers, other ultralight kinds (recency and passenger authorisation).* `de.luftpersv.45a.passengers#logged-takeoffs`
- Winch-, aerotow-, car- and bungee-launched flights count toward passenger recency for every ultralight kind. *Ultralight passengers, three-axis (recency and passenger authorisation).* `de.luftpersv.45a.passengers-three-axis#towed-launches`
- Winch-, aerotow-, car- and bungee-launched flights count toward passenger recency for every ultralight kind. *Ultralight passengers, other ultralight kinds (recency and passenger authorisation).* `de.luftpersv.45a.passengers#towed-launches`
- Ultralight flights without an ultralight kind no longer count toward passenger recency. *Ultralight passengers, three-axis (recency and passenger authorisation).* `de.luftpersv.45a.passengers-three-axis#kindless-ul`
- Ultralight flights without an ultralight kind no longer count toward passenger recency. *Ultralight passengers, other ultralight kinds (recency and passenger authorisation).* `de.luftpersv.45a.passengers#kindless-ul`
- If you fly three-axis ultralights and hold a valid PPL(A) or sailplane licence (SPL), your passenger authorisation counts as granted; you no longer need to record it. `de.luftpersv.45a.passengers-three-axis#deemed-authorisation`
- The progress toward the passenger authorisation counts only cross-country flights with an instructor flown after your ultralight licence was issued. *Ultralight passengers, three-axis (recency and passenger authorisation).* `de.luftpersv.45a.passengers-three-axis#xc-after-licence`
- The progress toward the passenger authorisation counts only cross-country flights with an instructor flown after your ultralight licence was issued. *Ultralight passengers, other ultralight kinds (recency and passenger authorisation).* `de.luftpersv.45a.passengers#xc-after-licence`
- The 200 km now apply to each of the two cross-country flights with an intermediate landing, not to all flights added up. *Ultralight passengers, three-axis (recency and passenger authorisation).* `de.luftpersv.45a.passengers-three-axis#xc-distance-per-flight`
- The 200 km now apply to each of the two cross-country flights with an intermediate landing, not to all flights added up. *Ultralight passengers, other ultralight kinds (recency and passenger authorisation).* `de.luftpersv.45a.passengers#xc-distance-per-flight`

#### German ultralights, towing rating (LuftPersV § 84(5))

- The 10 tows in 24 months are the statutory rule of § 84(5), and every tow you flew on an ultralight counts, whatever ultralight kind is stored with the rating. `de.luftpersv.84-5.ul-towing#tows-any-ultralight`
- A flight on the first day of the 24 months now counts; before, the period started at noon on that day. `de.luftpersv.84-5.ul-towing#window-first-day`
- A tow flight that records several towed gliders now counts one tow for each; one without a count is one tow. `de.luftpersv.84-5.ul-towing#towed-gliders`

#### German ultralights, instructor rating (LuftPersV § 96(4))

- Ultralight instructor ratings (LuftPersV § 96, privilege UL_INSTRUCTOR) are now evaluated: valid for three years, extended with two of instruction given, a refresher course and an assessment of competence.

### Other authorities (NinerLog policy)

#### Ratings of other authorities (expiry only)

- A rating tracked by its expiry date only is valid through that date and shows as expired from the next day. *Rating expiry only (authorities without rules).* `other.ninerlog.expiry-only#valid-through-expiry`
- A rating tracked by its expiry date only is valid through that date and shows as expired from the next day. *Rating expiry only (ultralight ratings on FAA certificates).* `other.ninerlog.expiry-only.faa-ultralight#valid-through-expiry`
- A licence whose authority is entered in unusual case or with spaces (" easa ", "Faa") now gets its authority's rules instead of expiry-only tracking. `other.ninerlog.expiry-only#authority-case`
