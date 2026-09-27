# Vocabulary requests

Porting agents who cannot express an article with `vocabulary.yaml` append a request to
`<family>.md` here instead of editing the vocabulary. The format is in
[../porting-guide.md](../porting-guide.md#7-when-the-vocabulary-is-not-enough). The
integration step either implements a request as one vocabulary change (vocabulary, schema,
engine, generator output and at least one case, DESIGN.md section 5) or declines it with a
written reason in DESIGN.md section 13, and removes it from this directory. This directory is
empty between porting rounds.

The requests of the first porting round (2026-09-27) and how each was resolved are listed in
DESIGN.md section 13.
