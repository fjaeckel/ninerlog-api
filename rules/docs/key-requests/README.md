# Message key requests

Used when several agents port rules in parallel (DESIGN.md 12.8). A porting agent never
edits `messages/keys.yaml`; it writes the keys its rules need to `<family>.yaml` here:

```yaml
keys:
  - { key: pax.example, kind: message, params: [], origin: catalogue, emitted: false, documented: false, notes: "What the message says and which article it serves." }
```

Until the integration step merges them, `rulescheck -report` lists the missing keys for the
family's rules; that is expected.

The integration step merges every file into `messages/keys.yaml` below the family's anchor
comment, removes duplicates (two families asking for the same key get one entry whose
`notes` covers both uses; differing `params` are reconciled to the union, optional ones
marked `?`), then deletes the merged files. This directory is empty between porting rounds.
