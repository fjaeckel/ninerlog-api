#!/usr/bin/env bash
# Verify every commit in a range carries a Developer Certificate of Origin
# sign-off (a "Signed-off-by: Name <email>" trailer matching the author or
# committer). Merge commits and commits authored by bot accounts are skipped.
#
# Usage:
#   ./scripts/check-dco.sh                 checks origin/main..HEAD
#   ./scripts/check-dco.sh BASE..HEAD      checks an explicit range
#
# Sign off with `git commit -s`; fix a whole branch with
# `git rebase --signoff origin/main`. Terms: DCO and docs/LICENSING.md.
set -euo pipefail

RANGE="${1:-origin/main..HEAD}"

commits="$(git rev-list --no-merges "$RANGE")"
if [ -z "$commits" ]; then
    echo "OK: no commits to check in $RANGE."
    exit 0
fi

failed=0
checked=0
for sha in $commits; do
    author_name="$(git log -1 --format='%an' "$sha")"
    case "$author_name" in
        *"[bot]") continue ;;
    esac
    checked=$((checked + 1))

    author_email="$(git log -1 --format='%ae' "$sha" | tr '[:upper:]' '[:lower:]')"
    committer_email="$(git log -1 --format='%ce' "$sha" | tr '[:upper:]' '[:lower:]')"
    signoffs="$(git log -1 --format='%(trailers:key=Signed-off-by,valueonly)' "$sha" \
        | grep -oE '<[^>]+>' | tr -d '<>' | tr '[:upper:]' '[:lower:]' || true)"

    if [ -z "$signoffs" ]; then
        echo "MISSING  $(git log -1 --format='%h %s' "$sha")"
        failed=$((failed + 1))
        continue
    fi
    if ! grep -qxF -e "$author_email" -e "$committer_email" <<<"$signoffs"; then
        echo "MISMATCH $(git log -1 --format='%h %s' "$sha")  (signed off by $(tr '\n' ' ' <<<"$signoffs")but authored by $author_email)"
        failed=$((failed + 1))
    fi
done

if [ "$failed" -gt 0 ]; then
    echo
    echo "FAIL: $failed of $checked commit(s) in $RANGE lack a valid Signed-off-by trailer."
    echo "Every contribution is certified under the Developer Certificate of Origin (see DCO)."
    echo "Sign off with 'git commit -s', or repair the branch with 'git rebase --signoff origin/main'."
    exit 1
fi

echo "OK: $checked commit(s) in $RANGE are signed off."
