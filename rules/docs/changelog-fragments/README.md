# CHANGELOG fragments

Used when several agents port rules in parallel (DESIGN.md 12.8). A porting agent never
edits `CHANGELOG.md`; it writes one pilot-readable line per divergence to `<family>.md` here,
under `###` headings by topic:

```markdown
### Passenger recency (FAA 14 CFR 61.57(a) and (b))

- Carrying passengers now needs three takeoffs as well as three landings. `faa.14cfr61.61-57-a.passengers#takeoffs`
```

The integration step moves the lines into `CHANGELOG.md` under `## Unreleased`, grouped by
authority (`### EASA`, `### FAA`, `### Germany`, `### Other authorities`) with the topic
headings as `####`, one line per divergence (`<rule id>#<divergence id>` is a stable id:
never rename it once released), then deletes the merged files. `rulescheck` fails on a
divergence without a line, a line naming a divergence that does not exist, and a divergence
listed on more than one line. This directory is empty between porting rounds.
