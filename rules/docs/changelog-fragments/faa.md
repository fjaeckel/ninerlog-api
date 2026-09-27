### Passenger recency (FAA 14 CFR 61.57(a) and (b))

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
- Night passenger recency now counts only landings to a full stop; if your flights do not record full-stop night landings, it shows as unknown instead of current. `faa.14cfr61.61-57-b.night-passengers#full-stop`
- Night passenger recency now needs three night takeoffs as well. `faa.14cfr61.61-57-b.night-passengers#night-takeoffs`
- Only night takeoffs and landings you made as sole manipulator count. `faa.14cfr61.61-57-b.night-passengers#sole-manipulator`
- Sport pilots with the 61.329 night endorsement now get night passenger recency. `faa.14cfr61.61-57-b.night-passengers#sport-night`
- For recreational pilots, and sport pilots without the night endorsement, night passenger recency is shown as not applicable (no night privilege) instead of unknown. `faa.14cfr61.61-57-b.night-passengers#no-night-privilege`
- Unmet night passenger recency is shown as lapsed. `faa.14cfr61.61-57-b.night-passengers#status-lapsed`

### Flight review (FAA 14 CFR 61.56)

- A pilot proficiency check or practical test with an examiner, or a completed WINGS phase, within the 24 calendar months now counts in place of a flight review. `faa.14cfr61.61-56.flight-review#substitutes`
- Three instructional glider flights no longer make a glider rating current on their own; they replace only the flight-training hour within a flight review, and the review itself must be logged. `faa.14cfr61.61-56.flight-review#glider-alternative`
- The flight review is shown once per pilot, no longer again for each glider rating. `faa.14cfr61.61-56.flight-review#per-pilot`
- Student pilots no longer see a flight review requirement. `faa.14cfr61.61-56.flight-review#student`
- A flight review logged on a flight simulator or flight training device now counts. `faa.14cfr61.61-56.flight-review#simulator`
- The flight review no longer shows an "expiring" warning in its last 90 days; the valid-until date still shows when it runs out. `faa.14cfr61.61-56.flight-review#no-expiring`

### Glider towing (FAA 14 CFR 61.69)

- Tows count toward towing recency only when flown accompanied by a qualified tow pilot. `faa.14cfr61.61-69.towing#accompanied`
- Only tows of gliders or unpowered ultralights count, not banner tows; a tow flight that does not say what it towed is not counted. `faa.14cfr61.61-69.towing#tow-kind`

### Instrument rating on sport and recreational certificates (FAA 14 CFR 61.315, 61.101)

- An instrument rating on a sport or recreational certificate is shown as not applicable instead of unknown. `faa.14cfr61.61-315.ir-not-applicable#status`

### Medical certificates and authorizations (FAA 14 CFR 61.23, 61.21)

- Your first-class medical's duration for ATP privileges is now worked out from the examination date and your age (12 months under 40, 6 months from 40), valid through the last day of the month. `faa.14cfr61.61-23-d.first-class-atp#derived-expiry`
- Your first- or second-class medical's duration for commercial privileges is now worked out from the examination date (12 months). `faa.14cfr61.61-23-d.commercial#derived-expiry`
- Your medical's duration for private, recreational, student, sport and instructor privileges is now worked out from the examination date and your age (60 months under 40, 24 months from 40). `faa.14cfr61.61-23-d.private#derived-expiry`
- Category II and III authorizations now expire at the end of the sixth calendar month after issue or renewal, and are valid through that day. `faa.14cfr61.61-21.cat-ii-iii#derived-expiry`

### Instructors (FAA 14 CFR 61.197, 61.217)

- Flight instructor certificates issued since 1 December 2024 have no expiration date, but you may instruct only with a flight instructor practical test or refresher course within the preceding 24 calendar months; NinerLog now shows this. `faa.14cfr61.61-197.cfi-recent-experience#recent-experience`
- Ground instructors now need instructing activity, a refresher course or an instructor's endorsement within the preceding 12 calendar months. `faa.14cfr61.61-217.ground-instructor#recent-experience`
