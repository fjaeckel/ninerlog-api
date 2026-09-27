# Inventory mapping fragments

Used when several agents port rules in parallel (DESIGN.md 12.8). A porting agent never
edits `inventory/*.yaml`; it writes the `maps_to` it wants to `<family>.yaml` here:

```yaml
code_rules:
  - { code_rule: faa.14cfr61.61-69.towing, maps_to: [faa.14cfr61.61-69.towing] }
articles:
  - { article: 14 CFR 61.69, maps_to: [faa.14cfr61.61-69.towing] }
```

`code_rule` is an `id` in `inventory/code-rules.yaml`, `article` a `cite` in
`inventory/articles-*.yaml`. The integration step appends the ids to the entries' `maps_to`
(never replacing existing ones), checks that every code rule (except `scope: consumer`) and
every article maps to at least one existing catalogue id, then deletes the merged files. This
directory is empty between porting rounds.
