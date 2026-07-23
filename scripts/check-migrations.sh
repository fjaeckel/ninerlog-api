#!/usr/bin/env bash
#
# check-migrations.sh — guard against malformed or duplicate database migrations.
#
# golang-migrate refuses to open the migration source if two files share the
# same version number ("duplicate migration file"), which crashes the API on
# startup before it can serve a single request. This script catches that (and a
# couple of related mistakes) in CI, on a pull request, instead of in prod.
#
# Checks:
#   1. Every migration file matches  NNNNNN_name.(up|down).sql
#   2. No version number (the NNNNNN prefix) is used by more than one migration
#   3. Every version has both an .up.sql and a .down.sql
#
# Portable: no associative arrays, so it runs on the bash 3.2 shipped with
# macOS as well as CI. Exit code is non-zero if any check fails.

set -euo pipefail

MIGRATIONS_DIR="${1:-db/migrations}"

if [[ ! -d "$MIGRATIONS_DIR" ]]; then
    echo "error: migrations directory not found: $MIGRATIONS_DIR" >&2
    exit 2
fi

fail=0
name_re='^[0-9]{6}_[a-z0-9_]+\.(up|down)\.sql$'

# Collected as newline-separated "version<TAB>name<TAB>direction" records.
records=""

shopt -s nullglob
for path in "$MIGRATIONS_DIR"/*.sql; do
    file="$(basename "$path")"

    # test_init.sql (and any other non-migration helper) is intentionally exempt.
    if [[ "$file" == "test_init.sql" ]]; then
        continue
    fi

    if [[ ! "$file" =~ $name_re ]]; then
        echo "malformed migration filename: $file" >&2
        echo "    expected NNNNNN_name.(up|down).sql (6-digit version, lowercase snake_case name)" >&2
        fail=1
        continue
    fi

    version="${file:0:6}"
    rest="${file:7}"            # strip "NNNNNN_"
    if [[ "$file" == *.up.sql ]]; then
        direction="up"
        name="${rest%.up.sql}"
    else
        direction="down"
        name="${rest%.down.sql}"
    fi

    records+="${version}	${name}	${direction}"$'\n'
done
shopt -u nullglob

# 2. Duplicate version numbers — one version mapped to more than one name.
dupes="$(
    printf '%s' "$records" \
        | awk -F'\t' 'NF>=2 {print $1"\t"$2}' \
        | sort -u \
        | awk -F'\t' '{count[$1]++; names[$1]=names[$1]" "$2}
                      END {for (v in count) if (count[v] > 1) print v names[v]}' \
        | sort
)"
if [[ -n "$dupes" ]]; then
    while IFS= read -r line; do
        version="${line%% *}"
        echo "duplicate migration version $version used by multiple migrations:" >&2
        for n in ${line#* }; do
            echo "      $n" >&2
        done
        echo "    → renumber one of them to the next unused version." >&2
    done <<< "$dupes"
    fail=1
fi

# 3. Every version must have a matching up/down pair.
unpaired="$(
    printf '%s' "$records" \
        | awk -F'\t' 'NF>=3 {seen[$1"\t"$2]=seen[$1"\t"$2]" "$3}
                      END {for (k in seen) {
                               s=seen[k]
                               if (s !~ /up/)   print k"\tmissing .up.sql"
                               if (s !~ /down/) print k"\tmissing .down.sql"
                           }}' \
        | sort
)"
if [[ -n "$unpaired" ]]; then
    while IFS=$'\t' read -r version name problem; do
        [[ -z "$version" ]] && continue
        echo "migration $version ($name) is $problem" >&2
    done <<< "$unpaired"
    fail=1
fi

if [[ "$fail" -ne 0 ]]; then
    echo "" >&2
    echo "Migration check failed. See errors above." >&2
    exit 1
fi

total="$(printf '%s' "$records" | awk -F'\t' 'NF>=1 {print $1}' | sort -u | grep -c .)"
echo "migrations OK — $total versions, no duplicates, all up/down pairs present."
