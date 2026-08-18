---
name: terse-comments
description: House comment style — every comment is terse and states only the WHAT, never the WHY. Rationale, history, and justification live in git commit messages and in docs/, not in source. Load before writing any comment, when touching code that contains comments, when reviewing a diff, or when auditing a file or package for comment style.
---

# Terse comments: the what, never the why

A comment in this codebase has exactly one job: label **what** the code next to it does, in
as few words as possible. It never explains **why** the code is the way it is. Rationale has
two homes, and source files are not one of them:

- **the git commit** that introduces or changes the line — `git log`/`git blame` recover it;
- **`docs/`** — when the reason is a durable rule of the system (see the `docs-sync` skill).

The point: a what-comment goes stale the moment the code changes and is caught in review
because it sits next to the code it contradicts. A why-comment goes stale invisibly — the
reason stops being true and nothing in the diff flags it. Commits are immutable and dated;
docs are reviewed as a deliverable. Comments are neither.

## The rule

- A comment states what the following block does, or what a symbol is. One line wherever
  possible; never a paragraph.
- No rationale clauses: because, so that, otherwise, to avoid, this prevents/ensures,
  we need/decided, the reason is.
- No history or archaeology: "used to", "previously", "the old behaviour", "issue #N showed",
  "before the fix".
- No narration of the obvious. Code that reads clearly gets no comment at all — deleting is
  the default, rewriting the fallback.
- Writing a change and the why matters? Put it in the commit message body and, if it is a
  lasting rule (domain invariant, contract, operational constraint), in the right `docs/`
  file — then keep the comment to the what, or drop it.
- Deleting an existing why-comment loses nothing: the rationale stays recoverable in the
  history of the commit that removes it and of the commits that wrote it.

## What stays

- **Go doc comments** on packages and exported symbols (`// Name ...`) — required by
  convention and lint. Write them as contract statements: what it does, parameters, errors
  returned, invariants held. No design rationale, no history.
- **Directives**: `//nolint:...`, `//go:build`, `//go:generate`, `//go:embed`. A `//nolint`
  keeps its required lint-name suffix, nothing more.
- **Generated-file markers** and license headers.
- **TODO/FIXME carrying an issue reference** — the issue holds the why.
- Test names and table-entry names do the explaining in tests; a comment inside a test states
  what is being asserted, not what regression prompted it.

## Rewrites

```go
// Attached after construction because the recorder needs the database and
// the mailer, which don't exist yet at config time.
svc.Recorder = rec
```
→
```go
// Recorder is attached after construction.
svc.Recorder = rec
```
(or delete — the code says it) — the dependency-ordering reason goes in the commit that
introduced the ordering, or `docs/ARCHITECTURE.md` if it is a wiring rule.

```go
// Claim takes the key in a single statement so that two concurrent requests
// cannot both win the race.
```
→
```go
// Claim atomically claims the key; the second caller gets ErrAlreadyClaimed.
```
The contract (atomic, sentinel error) is the what. The race scenario is the why — commit.

```go
// This test exists because pagination used to disagree with the statistics
// endpoint (issue #212): only statistics applied the timeframe filter.
```
→ delete the header; name the test `TestLogbookPagination_TimeframeAppliesToTotals` and let
the assertions speak. The issue reference belongs in the commit that added the test.

## Auditing a file

1. Read every comment (`//` and `/* */`) in the file, including doc comments.
2. For each: does it state anything other than the what? Rewrite to a single what-line or
   delete it. Never alter the code itself while doing so.
3. Skip `internal/api/generated/`, `vendor/`, and applied migration files entirely.
4. `make fmt && make lint && make test` must stay green — comment edits change no behaviour,
   so any failure is a mistake in the edit.
