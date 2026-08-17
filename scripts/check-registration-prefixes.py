#!/usr/bin/env python3
"""check-registration-prefixes.py — flag drift in the aircraft nationality mark table.

pkg/registration/prefixes.go vendors the ICAO nationality marks (the "country
code" at the front of a registration) because they change on the order of once
every few years — a new state, a state changing its mark — and a runtime fetch
would buy nothing but a new failure mode.

"Every few years" is exactly the interval at which a table quietly goes stale,
so this script compares the vendored table against an upstream list and reports
marks that appear on one side but not the other. It is a *review aid*, not an
auto-updater: it deliberately over-reports rather than parsing a specific
upstream layout, because the layouts change more often than the data does. A
human decides what each difference means and edits the Go table by hand.

Usage:
    scripts/check-registration-prefixes.py --file upstream.txt
    scripts/check-registration-prefixes.py --url  <upstream url>

Upstream sources, in order of authority (see docs/AIRCRAFT_REGISTRATIONS.md):

  1. ITU Radio Regulations Appendix 42, Table of International Call Sign Series
     https://www.itu.int/en/ITU-R/terrestrial/fmd/Pages/call_sign_series.aspx
  2. ICAO Annex 7 / https://www.icao.int/nationality-marks
  3. https://en.wikipedia.org/wiki/List_of_aircraft_registration_prefixes
     (wikitext: .../w/api.php?action=raw&title=List_of_aircraft_registration_prefixes)

Any of them works: the extractor looks for cells that are shaped like a
nationality mark, so wikitext, an HTML table saved as text, or a column pasted
out of a PDF all parse.

Exit status is 0 when the two sides agree, 1 when they differ (so a scheduled
CI job fails visibly), or 2 on a usage or fetch error. --warn-only always
exits 0.
"""

import argparse
import re
import sys
import urllib.request

DEFAULT_TABLE = "pkg/registration/prefixes.go"

# A nationality mark in the Go table: Prefix: "9XR"
TABLE_MARK = re.compile(r'Prefix:\s*"([0-9A-Z]{1,4})"(.*)')

# A cell that is nothing but a mark, with or without the trailing hyphen most
# lists write it with: "D-", "N", "9XR-", "RDPL-".
CANDIDATE_MARK = re.compile(r"^([0-9A-Z]{1,4})-?$")

# Wiki and HTML noise stripped before a cell is considered.
WIKI_LINK = re.compile(r"\[\[(?:[^\]|]*\|)?([^\]]*)\]\]")
HTML_TAG = re.compile(r"<[^>]+>")
REF = re.compile(r"<ref.*?</ref>|<ref[^>]*/>", re.DOTALL)

def marks_from_table(path):
    """Extract the nationality marks the Go table declares.

    Returns (all marks, the subset written without a hyphen). The second set
    seeds the upstream extractor below, so the two stay in step without a
    second hand-maintained list.
    """
    try:
        with open(path, encoding="utf-8") as fh:
            source = fh.read()
    except OSError as exc:
        sys.exit(f"error: cannot read {path}: {exc}")

    marks, no_hyphen = set(), set()
    for mark, rest in TABLE_MARK.findall(source):
        marks.add(mark)
        if "NoHyphen: true" in rest:
            no_hyphen.add(mark)
    if not marks:
        sys.exit(f"error: no nationality marks found in {path} — has the table format changed?")
    return marks, no_hyphen


def marks_from_upstream(text, no_hyphen):
    """Extract candidate nationality marks from an upstream list.

    Splits on every separator a table might use rather than parsing a layout,
    then keeps the cells that are shaped like a mark.

    Upstream lists sit a country column and usually an ISO 3166 column right
    next to the mark column, and the same split sees all three. An ISO alpha-2
    code is two letters with no hyphen, so a cell only counts as a mark when it
    carries the trailing hyphen these lists write marks with, contains a digit
    (no ISO code does), or is one of the handful of marks that genuinely have
    none. Without that test every run reports "DE", "GB" and "US" as missing
    marks and the real signal drowns.
    """
    text = REF.sub(" ", text)
    text = WIKI_LINK.sub(r"\1", text)
    text = HTML_TAG.sub(" ", text)

    marks = set()
    for cell in re.split(r"\|\||[|\n\t;,]", text):
        cell = cell.strip().strip("!*'\" ").upper()
        if not cell:
            continue
        found = CANDIDATE_MARK.match(cell)
        if not found:
            continue
        mark = found.group(1)
        if cell.endswith("-") or any(c.isdigit() for c in mark) or mark in no_hyphen:
            marks.add(mark)
    return marks


def fetch(url):
    request = urllib.request.Request(url, headers={"User-Agent": "ninerlog-prefix-check"})
    try:
        with urllib.request.urlopen(request, timeout=60) as response:
            return response.read().decode("utf-8", errors="replace")
    except Exception as exc:  # noqa: BLE001 — any failure here is just "no upstream"
        sys.exit(f"error: cannot fetch {url}: {exc}")


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--file", help="upstream list saved to disk")
    source.add_argument("--url", help="upstream list to fetch")
    parser.add_argument("--table", default=DEFAULT_TABLE, help=f"Go table to check (default: {DEFAULT_TABLE})")
    parser.add_argument("--warn-only", action="store_true", help="report differences but exit 0")
    args = parser.parse_args()

    if args.file:
        try:
            with open(args.file, encoding="utf-8") as fh:
                text = fh.read()
        except OSError as exc:
            sys.exit(f"error: cannot read {args.file}: {exc}")
    else:
        text = fetch(args.url)

    ours, no_hyphen = marks_from_table(args.table)
    theirs = marks_from_upstream(text, no_hyphen)
    if not theirs:
        sys.exit("error: no nationality marks found in the upstream list — wrong source, or a layout this script cannot read")

    missing = sorted(theirs - ours)
    extra = sorted(ours - theirs)

    print(f"vendored table: {len(ours)} marks ({args.table})")
    print(f"upstream list:  {len(theirs)} marks")

    if missing:
        print(f"\nIn upstream but not in the table ({len(missing)}) — candidates to add:")
        for mark in missing:
            print(f"  {mark}")
    if extra:
        print(f"\nIn the table but not in upstream ({len(extra)}) — candidates to remove, or upstream noise:")
        for mark in extra:
            print(f"  {mark}")

    if not missing and not extra:
        print("\nNo drift.")
        return 0

    print(
        "\nThis is a review aid and over-reports by design. Confirm each difference against\n"
        "ITU Appendix 42 or ICAO Annex 7 before editing pkg/registration/prefixes.go, then\n"
        "update its LastReviewed date. See docs/AIRCRAFT_REGISTRATIONS.md."
    )
    return 0 if args.warn_only else 1


if __name__ == "__main__":
    sys.exit(main())
