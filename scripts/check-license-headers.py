#!/usr/bin/env python3
"""Verify every Go source file carries the project's licence header.

The repository is licensed under the AGPL-3.0-only, and a file copied out of
it on its own carries no notice unless the file itself does. Every hand-written
Go file therefore starts with the header below, and this script keeps that
true. Generated code (internal/api/generated/ and any file marked
"Code generated ... DO NOT EDIT") is exempt.

Expected header (first three lines of the file):

    // Copyright (C) The NinerLog Authors
    // SPDX-License-Identifier: AGPL-3.0-only
    <blank line>

Usage:
  python3 scripts/check-license-headers.py          check, exit 1 on failure
  python3 scripts/check-license-headers.py --fix    insert the header where missing
"""

import glob
import os
import re
import sys

HEADER_LINES = [
    "// Copyright (C) The NinerLog Authors",
    "// SPDX-License-Identifier: AGPL-3.0-only",
]
HEADER = "\n".join(HEADER_LINES) + "\n\n"

GO_GLOB = "**/*.go"
SKIP_DIRS = ["internal/api/generated/", "vendor/"]
GENERATED_RE = re.compile(r"^// Code generated .* DO NOT EDIT\.$", re.MULTILINE)


def go_files():
    for path in sorted(glob.glob(GO_GLOB, recursive=True)):
        if any(path.startswith(s) for s in SKIP_DIRS):
            continue
        yield path


def is_generated(src):
    head = "\n".join(src.split("\n", 10)[:10])
    return GENERATED_RE.search(head) is not None


def has_header(src):
    lines = src.split("\n", 3)
    return (
        len(lines) >= 3
        and lines[0] == HEADER_LINES[0]
        and lines[1] == HEADER_LINES[1]
        and lines[2] == ""
    )


def main(argv):
    fix = "--fix" in argv
    if not os.path.exists("go.mod"):
        sys.exit("go.mod not found — run from the repository root")

    missing = []
    checked = 0
    for path in go_files():
        with open(path, encoding="utf-8") as fh:
            src = fh.read()
        if is_generated(src):
            continue
        checked += 1
        if has_header(src):
            continue
        missing.append(path)
        if fix:
            with open(path, "w", encoding="utf-8") as fh:
                fh.write(HEADER + src)

    if missing and fix:
        print(f"Added the licence header to {len(missing)} file(s).")
        return 0

    if missing:
        print(f"FAIL: {len(missing)} Go file(s) lack the licence header.")
        print("Run `make license-headers` to insert it. Expected first lines:")
        for line in HEADER_LINES:
            print(f"  {line}")
        print()
        for path in missing:
            print(f"  {path}")
        return 1

    print(f"OK: {checked} Go file(s) carry the licence header.")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
