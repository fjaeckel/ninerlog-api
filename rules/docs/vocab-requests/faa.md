# Vocabulary requests: FAA (14 CFR Part 61, Part 68)

## faa.14cfr61.61-56.flight-review: stage condition on validUntil

- Article text (verbatim, 14 CFR 61.56(c)): "no person may act as pilot in command of an aircraft unless, since the beginning of the 24th calendar month before the month in which that pilot acts as pilot in command, that person has—"
- What the rule needs to say: report `expiring` (flight_review.expiring {days, date}) when the review's validUntil is at most 90 days away, as the API does today.
- Proposed vocabulary entry: stage condition `valid_until_within: { days: n }` (the evaluation's validUntil is not after asOf + n days), and param sources `days_to_valid_until` and `valid_until` for the message.
- Why existing entries do not work: `expires_within` reads the subject's expiry; a flight_review subject has none, its end is the moving window's validUntil.
- Cases that would exercise it: a review in September 2024 on 16 August 2026 (validUntil 2026-09-30, 45 days) is expiring; one in March 2025 is current.
- Also useful for: 61.57(c) instrument currency, 61.69 towing, 61.197 instructor recent experience, 61.217.

## faa.14cfr61.61-57-a.passengers: type-rated subjects

- Article text (verbatim, 14 CFR 61.57(a)(1)(ii)): "(ii) The required takeoffs and landings were performed in an aircraft of the same category, class, and type (if a class or type rating is required)"
- What the rule needs to say: count takeoffs and landings per type for a rating that carries a type designator, per class otherwise.
- Proposed vocabulary entry: either an applies_to selector `typeRated: true|false` (rating has a typeDesignator), or filter semantics where `typeDesignators: [$subject]` is ignored when the subject has no type designator.
- Why existing entries do not work: `typeDesignators: [$subject]` with a class rating resolves to an empty list, which excludes (or makes unknown) every flight; applies_to cannot tell type ratings from class ratings.
- Cases that would exercise it: a pilot with a B737 type rating (MEP_LAND, typeDesignator B737) and an MEP_LAND class rating; landings in a PA-34 count for the class, not for the type. Also needed for 61.58 (PIC proficiency check per type).
