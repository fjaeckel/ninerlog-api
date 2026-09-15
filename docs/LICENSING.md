# Licensing

NinerLog API is free software under the **GNU Affero General Public License,
version 3** (`AGPL-3.0-only`). This document is the single place that says what
that means for operators, contributors and dependencies, and which checks keep
it true. Everything described here is enforced by `make license-check`,
`make dco-check` and the `Licence Check` and `DCO Check` jobs in CI.

## The licence

The full text is in [`LICENSE`](../LICENSE). In short:

- Anyone may run, study, modify and redistribute the software, for any purpose.
- A modified version must be distributed under the same licence, with its
  complete corresponding source.
- **Section 13**: a modified version that users interact with over a network
  must offer those users its source, even when nothing is "distributed". This
  is the clause that distinguishes the AGPL from the GPL, and it is why the
  project uses it: a hosted fork of the logbook cannot keep its improvements to
  itself.

The identifier is `AGPL-3.0-only`, not `-or-later`: the terms are fixed at
version 3 and do not change if the Free Software Foundation publishes a later
version. Changing the licence of the project, in either direction, would need
the agreement of every copyright holder (see [Contributions](#contributions)).

## What an operator owes

An **unmodified** deployment, such as the published `ghcr.io` image or a build
of an upstream tag, has nothing to do. The source is upstream, and the server
says so.

A deployment running **modified** code must:

1. Publish the modified source, for example as a public fork.
2. Set `SOURCE_URL` to where it lives. Startup fails on anything that is not
   an http(s) URL.

The server surfaces the offer in three places, so no operator has to build it:

| Surface | What it shows |
| --- | --- |
| `GET /api/v1/about` (unauthenticated) | `name`, `version`, `commit`, `license`, `licenseUrl`, `sourceUrl` |
| `GET /api/v1/admin/config` | `sourceUrl`, the effective value, next to `appVersion` |
| Docker image | `/app/LICENSE` and, under `/app/licenses/`, the licence of every dependency linked into the binary |

Clients show the source link wherever they show the version; the frontend's
counterpart lives in `ninerlog-frontend`.

## Third-party code

Everything linked into the binary is distributed under the AGPL together with
this project's code, so every dependency must carry a licence that permits
that. The policy is an allow-list, checked against the licence file of every
module in the build graph (test dependencies included):

| Allowed | Why it is compatible |
| --- | --- |
| `Apache-2.0`, `MIT`, `BSD-2-Clause`, `BSD-3-Clause`, `ISC`, `0BSD`, `Unlicense`, `CC0-1.0` | Permissive; only require that notices are preserved, which the image does |
| `MPL-2.0` | Explicitly allows combination with (A)GPL works |
| `GPL-3.0`, `LGPL-2.1`, `LGPL-3.0`, `AGPL-3.0` | Copyleft licences the AGPLv3 permits combining with (section 13 of the AGPL and section 13 of the GPLv3) |

Anything else fails the check. That includes `GPL-2.0-only` (incompatible with
version 3 of the (A)GPL), source-available licences such as SSPL, BUSL or the
Commons Clause, `CC-BY-NC-*`, any "non-commercial" or "no derivatives" term,
and a module with no licence file at all. A dependency with a licence outside
the list is not added until it has been reviewed; if it is genuinely
compatible, its identifier joins the allow-list in
`scripts/check-dependency-licenses.sh` in the same commit, with the reason in
the commit message.

One module is exempted by name in that script: `github.com/DATA-DOG/go-sqlmock`,
a test-only dependency whose `LICENSE` is BSD-3-Clause with a non-standard
preamble the classifier does not recognise. It was reviewed by hand.

The check is `go-licenses` (pinned in the script), run by `make license-check`
and by CI. The Docker build runs the same script with `--save`, which exports
every dependency's licence file into the image under `/app/licenses/`. That is
what satisfies the "preserve this notice" clause of the permissive licences
when the binary is redistributed.

### Data the server uses but does not ship

- **Airports** are fetched at runtime from [OurAirports](https://ourairports.com/)
  (public domain) and [mwgg/Airports](https://github.com/mwgg/Airports) (MIT).
  Neither is embedded in the binary or the image.
- The **aircraft nationality mark table** in `pkg/registration` records facts
  published by ICAO and the states of registry. Wikipedia's list is used only as
  a review reference (`make prefix-check`); see
  [AIRCRAFT_REGISTRATIONS.md](./AIRCRAFT_REGISTRATIONS.md).
- **Import samples** under `internal/api/handlers/testdata/importsamples/` are
  anonymised export files contributed under the DCO like any other change.

## Source file headers

Every hand-written Go file starts with:

```go
// Copyright (C) The NinerLog Authors
// SPDX-License-Identifier: AGPL-3.0-only
```

followed by a blank line. "The NinerLog Authors" are the people in the git
history; each keeps the copyright in what they wrote. A file copied out of the
repository on its own would otherwise carry no licence at all.

`python3 scripts/check-license-headers.py` verifies this and
`make license-headers` inserts the header where it is missing. Generated code
(`internal/api/generated/` and any file marked `Code generated ... DO NOT EDIT`)
is exempt: the generator owns its first line, and the repository licence covers
it.

## Contributions

Contributions are accepted on **inbound = outbound** terms: by submitting a
change you license it under `AGPL-3.0-only`, exactly as the project is
licensed, and certify the [Developer Certificate of Origin](../DCO) by signing
off every commit:

```bash
git commit -s                              # adds "Signed-off-by: Name <email>"
git rebase --signoff origin/main           # repair a whole branch
git config --global format.signOff true    # sign off by default
```

The sign-off's email must match the commit's author or committer.
`make dco-check` verifies the current branch and the `DCO Check` job verifies
every commit of a pull request; commits by bot accounts (Dependabot and the
like) are skipped.

There is **no contributor licence agreement and no copyright assignment**.
Contributors keep the copyright in their work and grant it only under the
AGPL. The consequence is deliberate: nobody, the maintainer included, can
relicense the project, close it, or move it to a source-available licence
without the agreement of every person who ever contributed. That is what
keeps NinerLog AGPL.

## Tooling

| Command | Script | CI job |
| --- | --- | --- |
| `make license-check` | `scripts/check-license-headers.py`, `scripts/check-dependency-licenses.sh` | `Licence Check` (ci.yml), required by the image publish |
| `make license-headers` | `scripts/check-license-headers.py --fix` | none (fixes locally) |
| `make dco-check` (`BASE=origin/main`) | `scripts/check-dco.sh` | `DCO Check` (ci.yml, pull requests) |
| Docker build | `scripts/check-dependency-licenses.sh --save` | `Publish Docker Image` |
