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

Upstream sources (see docs/AIRCRAFT_REGISTRATIONS.md):

  1. ICAO's published nationality marks — https://www.icao.int/nationality-marks
     The authority: Annex 7 Standard 3.3 and the hyphen convention.
  2. https://en.wikipedia.org/wiki/List_of_aircraft_registration_prefixes
     (wikitext: .../w/index.php?title=List_of_aircraft_registration_prefixes&action=raw)
     The practical consolidation, and the default for `make prefix-check`.

Either works: the extractor looks for cells that are shaped like a nationality
mark, so wikitext, an HTML table saved as text, or a column pasted out of a PDF
all parse.

Do NOT point this at ITU Radio Regulations Appendix 42. It is a
radiocommunication document that says nothing about aircraft: it allocates
whole call-sign blocks (Germany DAA-DRZ, the United States AAA-ALZ / KAA-KZZ /
NAA-NZZ / WAA-WZZ), not the aircraft marks selected out of them (D, N), and it
has no notion of hyphenation. Feeding it in would report hundreds of spurious
differences.

This is advisory and exits 0 even when it finds differences. That is deliberate:
the extractor reads cells rather than a layout, so its recall against any given
upstream is imperfect, and a check that cannot be calibrated must not gate a
pull request — a permanently red check is one everybody learns to scroll past.
Read the report instead. Pass --fail-on-drift to make it exit 1 once the
extractor has been calibrated against a live upstream and the noise floor is
known to be zero.

Exit status is 2 on a usage or fetch error.
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

    Returns (all marks, the subset written without a hyphen).
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

    A cell counts as a mark only when it carries the trailing hyphen these
    lists write marks with, contains a digit, or is one of the marks that
    genuinely have none. This keeps the adjacent ISO 3166 column out.
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
        # Every mark has a letter; an all-digit cell is a year or a count.
        if not any(c.isalpha() for c in mark):
            continue
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
    parser.add_argument(
        "--fail-on-drift",
        action="store_true",
        help="exit 1 when the two sides differ (default: report and exit 0)",
    )
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
        "ICAO's published nationality marks (https://www.icao.int/nationality-marks) before\n"
        "editing pkg/registration/prefixes.go, then update its LastReviewed date.\n"
        "See docs/AIRCRAFT_REGISTRATIONS.md."
    )
    return 1 if args.fail_on_drift else 0


if __name__ == "__main__":
    sys.exit(main())
