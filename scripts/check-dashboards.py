#!/usr/bin/env python3
"""Validate the Grafana dashboards in docs/metrics/dashboards/.

The dashboards are hand-maintained JSON that references metric names defined in
Go. Nothing links the two, so a renamed or removed metric leaves a silently
empty panel — which is how the airport-database metrics ended up charted
nowhere and rate_limit_hits_total kept a `path` label the docs described
incorrectly. This script closes that loop.

Checks:
  1. Every file is valid JSON with the fields Grafana's importer needs.
  2. Panels do not overlap and stay inside the 24-column grid.
  3. Every metric referenced in a PromQL expression is declared in the Go
     source (or is a known Prometheus/Go-collector built-in).
  4. Every metric declared in Go appears on at least one dashboard — reported
     as a warning, not a failure, since not everything deserves a panel.

Usage: python3 scripts/check-dashboards.py   (exit 1 on failure)
"""

import json
import glob
import os
import re
import sys

DASHBOARD_GLOB = "docs/metrics/dashboards/*.json"
GO_DIRS = ["internal/", "cmd/", "pkg/"]

# Metrics that come from the default Prometheus Go/process collectors rather
# than from this codebase.
BUILTIN = {
    "up",
    "go_goroutines",
    "go_gc_duration_seconds",
    "go_memstats_alloc_bytes",
    "go_threads",
    "process_cpu_seconds_total",
    "process_resident_memory_bytes",
    "process_open_fds",
    "scrape_duration_seconds",
}

# Histogram/summary suffixes Prometheus appends to the declared base name.
SUFFIXES = ("_bucket", "_count", "_sum")

# PromQL keywords and functions that tokenize like metric names.
PROMQL_WORDS = {
    "abs", "and", "avg", "avg_over_time", "bool", "by", "ceil", "changes",
    "clamp", "clamp_max", "clamp_min", "count", "count_over_time", "delta",
    "deriv", "floor", "group_left", "group_right", "histogram_quantile",
    "hour", "idelta", "ignoring", "increase", "irate", "label_replace",
    "last_over_time", "le", "max", "max_over_time", "min", "min_over_time",
    "offset", "on", "or", "predict_linear", "quantile", "rate", "resets",
    "round", "scalar", "sort", "sort_desc", "stddev", "sum", "sum_over_time",
    "time", "timestamp", "topk", "unless", "vector", "without",
}

# Metrics that are deliberately not on a dashboard.
UNCHARTED_OK = {
    # Covered by the "Notification Check Health" panel via its _bucket series.
    "notification_check_duration_seconds",
    # Redundant with airport_db_age_seconds, which the Airport Database
    # dashboard charts: age is time() minus this timestamp.
    "airport_db_last_success_timestamp_seconds",
}


# prometheus.CounterOpts{ ... Name: "foo_total" ... } — the Name field can sit
# several lines into the literal, so match the opening brace then the first
# Name: within it. Anchoring on the Opts type keeps unrelated Go structs that
# happen to have a Name field (config keys, OpenAPI types) out of the results.
OPTS_NAME_RE = re.compile(
    r'prometheus\.\w*Opts\s*\{[^}]*?\bName:\s*"([a-z_][a-z_0-9]*)"',
    re.DOTALL,
)
# prometheus.NewDesc("foo_total", ... — name is the first argument, often on
# its own line.
NEW_DESC_RE = re.compile(r'NewDesc\(\s*"([a-z_][a-z_0-9]*)"')


def declared_metrics():
    """Metric names this codebase registers with Prometheus."""
    names = set()
    for go_dir in GO_DIRS:
        if not os.path.isdir(go_dir):
            continue
        for root, _, files in os.walk(go_dir):
            for filename in files:
                if not filename.endswith(".go") or filename.endswith("_test.go"):
                    continue
                with open(os.path.join(root, filename), encoding="utf-8") as fh:
                    source = fh.read()
                if "prometheus" not in source:
                    continue
                names.update(OPTS_NAME_RE.findall(source))
                names.update(NEW_DESC_RE.findall(source))
    return names


def referenced_metrics(expr, grouping_labels):
    """Metric names used in a PromQL expression, minus functions, label names,
    and string literals."""
    # Label matchers inside {...} and the contents of string literals.
    label_names = set(re.findall(r'[{,]\s*([a-z_][a-z_0-9]*)\s*[=!~]+', expr))
    label_names |= grouping_labels
    literals = " ".join(re.findall(r'"([^"]*)"', expr))

    found = set()
    for tok in re.findall(r'\b[a-z_][a-z_0-9]*\b', expr):
        if tok in PROMQL_WORDS or tok in label_names:
            continue
        if re.search(r'\b' + re.escape(tok) + r'\b', literals):
            continue
        found.add(tok)
    return found


def base_name(metric):
    for suffix in SUFFIXES:
        if metric.endswith(suffix):
            return metric[: -len(suffix)]
    return metric


def main():
    paths = sorted(glob.glob(DASHBOARD_GLOB))
    if not paths:
        print(f"no dashboards found at {DASHBOARD_GLOB}", file=sys.stderr)
        return 1

    declared = declared_metrics()
    if not declared:
        print("could not extract any metric names from the Go source "
              "(run from the repository root)", file=sys.stderr)
        return 1

    errors = []
    charted = set()
    uids = {}

    for path in paths:
        name = os.path.basename(path)
        try:
            with open(path) as fh:
                dash = json.load(fh)
        except json.JSONDecodeError as exc:
            errors.append(f"{name}: invalid JSON: {exc}")
            continue

        for field in ("title", "uid", "panels", "schemaVersion"):
            if field not in dash:
                errors.append(f"{name}: missing required field {field!r}")

        uid = dash.get("uid")
        if uid in uids:
            errors.append(f"{name}: uid {uid!r} already used by {uids[uid]}")
        uids[uid] = name

        # --- layout: no overlaps, nothing past column 24
        occupied = {}
        for panel in dash.get("panels", []):
            title = panel.get("title", "<untitled>")
            grid = panel.get("gridPos")
            if not grid:
                errors.append(f"{name}: panel {title!r} has no gridPos")
                continue
            if grid["x"] + grid["w"] > 24:
                errors.append(
                    f"{name}: panel {title!r} spans past column 24 "
                    f"(x={grid['x']} w={grid['w']})")
            for y in range(grid["y"], grid["y"] + grid["h"]):
                for x in range(grid["x"], grid["x"] + grid["w"]):
                    if (x, y) in occupied:
                        errors.append(
                            f"{name}: panel {title!r} overlaps "
                            f"{occupied[(x, y)]!r} at x={x} y={y}")
                        occupied[(x, y)] = title
                        continue
                    occupied[(x, y)] = title

        # --- metric names resolve
        for panel in dash.get("panels", []):
            title = panel.get("title", "<untitled>")
            for target in panel.get("targets") or []:
                expr = target.get("expr", "")
                if not expr:
                    continue
                grouping = set()
                for group in re.findall(r'(?:by|without)\s*\(([^)]*)\)', expr):
                    grouping.update(t.strip() for t in group.split(",") if t.strip())
                for metric in referenced_metrics(expr, grouping):
                    base = base_name(metric)
                    if base in declared or base in BUILTIN:
                        charted.add(base)
                        continue
                    errors.append(
                        f"{name}: panel {title!r} references unknown metric "
                        f"{metric!r}")

        print(f"  {name}: {len(dash.get('panels', []))} panels")

    uncharted = sorted(declared - charted - UNCHARTED_OK)
    if uncharted:
        print("\nwarning: declared but on no dashboard:")
        for metric in uncharted:
            print(f"  - {metric}")

    if errors:
        print(f"\n{len(errors)} problem(s):", file=sys.stderr)
        for err in errors:
            print(f"  {err}", file=sys.stderr)
        return 1

    print("\ndashboards OK")
    return 0


if __name__ == "__main__":
    sys.exit(main())
