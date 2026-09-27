### Multi-engine class ratings (EASA FCL.740.A(a))

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

### Single-engine turbo-prop class ratings (EASA FCL.740.A(b)(3))

- An SET rating is revalidated only by a proficiency check with an examiner; route sectors and refresher training no longer count. `easa.part-fcl.fcl-740-a.set#proficiency-check-only`
- The proficiency check counts only within the 3 months before the expiry date. `easa.part-fcl.fcl-740-a.set#check-window`
- Your SET rating is valid through its expiry date; it shows as expired from the next day. `easa.part-fcl.fcl-740-a.set#valid-through-expiry`
- SET ratings recorded on a DULV or DAeC licence are no longer evaluated. `easa.part-fcl.fcl-740-a.set#authorities`

### Instrument rating (EASA FCL.625, FCL.625.A, FCL.625.H)

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

### LAPL(A) recency (EASA FCL.140.A)

- Only time as PIC, dual or supervised solo counts toward the 12 hours; co-pilot, PICUS and examiner time no longer do. `easa.part-fcl.fcl-140-a.lapl-a#pic-dual-spic-time`
- The 12 take-offs and landings now need take-offs as well as landings. `easa.part-fcl.fcl-140-a.lapl-a#takeoffs`
- The 1 hour of refresher training may now be flown over several flights. `easa.part-fcl.fcl-140-a.lapl-a#refresher-cumulative`
- Ultralight hours still count toward the 12 hours, but no longer toward take-offs and landings. `easa.part-fcl.fcl-140-a.lapl-a#ul-credit-time-only`
- With both SEP(land) and SEP(sea), each class now needs 1 hour and 6 take-offs and landings. `easa.part-fcl.fcl-140-a.lapl-a#land-sea-takeoffs`
- Flights count whatever their launch method. `easa.part-fcl.fcl-140-a.lapl-a#towed-launches`
- LAPL(A) privileges recorded on a DULV or DAeC licence are no longer evaluated. `easa.part-fcl.fcl-140-a.lapl-a#authorities`

### Passenger recency (EASA FCL.060(b))

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

### Class and type rating validity (EASA FCL.740)

- Ultralight ratings recorded on an EASA licence no longer get a Part-FCL expiry result. `easa.part-fcl.fcl-740.validity#ultralight`
- Helicopter, powered-lift and airship ratings are now revalidated by their own rules instead of by expiry date only. `easa.part-fcl.fcl-740.validity#rotorcraft-and-airships`
- Type ratings are valid through their expiry date; they show as expired from the next day. `easa.part-fcl.fcl-740.validity#valid-through-expiry`
- Type ratings recorded on a DULV or DAeC licence are no longer evaluated. `easa.part-fcl.fcl-740.validity#authorities`
- New: helicopter type ratings need 2 hours in the type within the validity period and a proficiency check in the 3 months before expiry. `easa.part-fcl.fcl-740-h.helicopter#revalidation`
- Helicopter type ratings are valid through their expiry date. `easa.part-fcl.fcl-740-h.helicopter#valid-through-expiry`
- New: powered-lift type ratings need a proficiency check in the 3 months before expiry and 10 route sectors (or 1 with an examiner). `easa.part-fcl.fcl-740-pl.powered-lift#revalidation`
- Powered-lift type ratings are valid through their expiry date. `easa.part-fcl.fcl-740-pl.powered-lift#valid-through-expiry`
- New: airship type ratings need a proficiency check in the 3 months before expiry and 2 hours in the type within the validity period. `easa.part-fcl.fcl-740-as.airship#revalidation`
- Airship type ratings are valid through their expiry date. `easa.part-fcl.fcl-740-as.airship#valid-through-expiry`

### Towing ratings (EASA FCL.805)

- The sailplane towing rating of an aeroplane licence now follows FCL.805 (5 tows in 24 months), not the sailplane rules. `easa.part-fcl.fcl-805.sailplane-towing#fcl-805`
- Only sailplane tows count toward the sailplane towing rating; a tow flight that does not say what it towed shows as unknown. `easa.part-fcl.fcl-805.sailplane-towing#tow-kind`
- Only tows flown in an aeroplane or TMG count. `easa.part-fcl.fcl-805.sailplane-towing#aeroplanes-only`
- The banner towing rating of an aeroplane licence now follows FCL.805. `easa.part-fcl.fcl-805.banner-towing#fcl-805`
- Only banner tows count toward the banner towing rating. `easa.part-fcl.fcl-805.banner-towing#tow-kind`
- Only banner tows flown in an aeroplane or TMG count. `easa.part-fcl.fcl-805.banner-towing#aeroplanes-only`

### Medical certificates (EASA MED.A.045)

- The validity of a class 1 medical now follows from your age (12 months, 6 from age 60); an entered expiry date later than that is capped. `easa.part-med.med-a-045.class-1#derived-validity`
- A class 1 medical is valid through its expiry date. `easa.part-med.med-a-045.class-1#valid-through-expiry`
- The validity of a class 2 medical now follows from your age (60, 24 or 12 months, ending at 42 or 51 at the latest); an entered expiry date later than that is capped. `easa.part-med.med-a-045.class-2#derived-validity`
- A class 2 medical is valid through its expiry date. `easa.part-med.med-a-045.class-2#valid-through-expiry`
- The validity of a LAPL medical now follows from your age (60 months, ending at 42 at the latest, or 24 months from 40); an entered expiry date later than that is capped. `easa.part-med.med-a-045.lapl#derived-validity`
- A LAPL medical is valid through its expiry date. `easa.part-med.med-a-045.lapl#valid-through-expiry`
