# Sources

Verbatim texts of the provisions the catalogue quotes. Every rule's `source.quote` and
`related_sources[].quote` must appear in the file it names (`rulescheck` checks this after
collapsing whitespace). Each file's header records the official URL, how it was retrieved,
the consolidation it comes from, the retrieval date, its origin and its attribution.

Only what is safe to reuse and does not infringe anyone's copyright is kept with the
catalogue. This is not legal advice, and none of these files is an authentic text.

## Allowed origins (enforced)

`vocabulary.yaml` `source_origins` is the allow-list. Every file here declares one origin in
its header, and `rulescheck` fails (section "Sources") unless:

- the file is Markdown (no raw XML, HTML or PDF downloads are stored);
- its header has `- Origin: <name>` with an allowed name, and the file lives in that
  origin's directory;
- its header has an `- Attribution:` line containing the origin's attribution text;
- every URL in its header is on one of the origin's hosts (so a text fetched from EASA's
  Easy Access Rules, ICAO or an association website cannot pass);
- neither its header nor any heading names material that is never stored (the markers in
  `source_forbidden_markers`: AMC, GM, Easy Access Rules, ICAO, DULV, DAeC, association).

| Origin | Directory | Hosts | Attribution the header carries | Why it may be stored |
| --- | --- | --- | --- | --- |
| `us-federal` | `faa/` | www.ecfr.gov, www.govinfo.gov, www.federalregister.gov | 17 U.S.C. § 105 | Works of the US federal government are not subject to copyright in the United States. |
| `de-amtliches-werk` | `de/` | www.gesetze-im-internet.de | § 5(1) UrhG | German statutes and ordinances are official works not protected by copyright. |
| `eu-legal-act` | `easa/` | eur-lex.europa.eu, publications.europa.eu | © European Union, https://eur-lex.europa.eu | EU legal texts may be reused under Commission Decision 2011/833/EU with this attribution and without distorting their meaning. |

Counts on 2026-09-27: `us-federal` 32, `de-amtliches-werk` 15, `eu-legal-act` 66.

## Status by origin

| Directory | Origin | Copyright status | Authoritative text |
| --- | --- | --- | --- |
| `faa/` | Title 14 CFR Parts 61 and 68, from the eCFR (www.ecfr.gov) | Works of the US federal government are not subject to copyright in the United States (17 U.S.C. § 105). | The Federal Register and the annual edition of the Code of Federal Regulations. The eCFR is an editorial compilation, not an official legal edition. |
| `de/` | LuftPersV and LuftVZO, from the official XML at gesetze-im-internet.de (Bundesministerium der Justiz) | Statutes and ordinances are official works (amtliche Werke) and are not protected by copyright (§ 5(1) UrhG). | The Bundesgesetzblatt. gesetze-im-internet.de publishes consolidated, non-authoritative versions. |
| `easa/` | Regulations (EU) No 1178/2011 (Part-FCL, Part-MED), 2018/1976 (Part-SFCL) and 2018/395 (Part-BFCL), consolidated texts from EUR-Lex via the Publications Office Cellar | © European Union, https://eur-lex.europa.eu. Reused under Commission Decision 2011/833/EU on the reuse of Commission documents: the source is acknowledged in every file and in NOTICE, and the texts are reproduced without changes that distort their meaning (the only editorial handling, recorded in each header, is removing consolidation markers and joining paragraph labels to their text). | The Official Journal of the European Union. Consolidated versions on EUR-Lex are documentation tools and are not authentic. |

## What is never stored here

- **EASA AMC and GM** (Acceptable Means of Compliance, Guidance Material, ED Decisions) and
  EASA's Easy Access Rules compilations. Where AMC or GM matter, `docs/amc-gm-notes.md`
  summarises them in our own words and names the decision; they are never copied. (These
  summaries used to sit in the headers of `easa/*.md`; they moved out on 2026-09-27 so that
  this directory holds only allowed-origin text.)
- **Association documents** (DULV, DAeC). The rules these associations set as Beauftragte
  under § 31c LuftVG are cited by title in the rule's `source` (`source_kind: association`),
  never stored. A rule should not quote them at all; the gate caps any such quote at 300
  characters. No rule quotes one today. The delegating statute (e.g. LuftPersV § 45(4)) is
  stored here and quoted under `related_sources`.
- **ICAO documents** (Annexes, PANS, manuals). They are cited by name only.
- **NinerLog policy** (`source_kind: app_policy`): the fallback rules under
  `catalogue/other/` have no regulation behind them and cite the policy only.

## Adding a source

Fetch the official text (eCFR versioner API, the gesetze-im-internet.de XML, the EUR-Lex
consolidated text from the Publications Office Cellar), keep the header format of the
existing files of the same directory including the `Origin` and `Attribution` lines, and
never paraphrase or retype a provision from memory. If a text cannot be fetched, the rule quotes
`TODO: verify` until it can.
