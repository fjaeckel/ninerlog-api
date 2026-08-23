#!/usr/bin/env python3
"""Verify every HTTP route this server registers is declared in the OpenAPI spec.

`api-spec/openapi.yaml` is the single source of truth, but nothing stopped a
route from being registered by hand on a gin group and never appearing in it.
Routes added that way are invisible to every generated client — the React PWA
and the iOS app both build their client from this spec — so the feature exists
on the server and cannot be reached from the app. Custom currency rules lived
that way for three migrations. This script closes that loop.

Checks:
  1. Every route registered outside `internal/api/generated/` is declared in
     the spec. The generated registrations come from the spec by construction,
     so only hand-written ones can drift.
  2. Every API route lives under `/api/v1`. Only the operational endpoints in
     ROOT_ALLOWED may be registered on the bare engine.
  3. The spec has no duplicate keys. PyYAML keeps the last of a repeated key
     and the generator rejects the file outright, so a collision is worth
     catching here with a readable message.

Method: a static scan, because the router is assembled inline in
cmd/api/main.go and cannot be built without a database. It resolves
`x := y.Group("/prefix")` chains within a file, and treats a
`*gin.RouterGroup` parameter of an exported Register* function as the group
that main.go passes in — which the scan confirms by reading the call site.

Usage: python3 scripts/check-routes.py   (exit 1 on failure)
"""

import glob
import os
import re
import sys

SPEC = "api-spec/openapi.yaml"
GO_DIRS = ["cmd/", "internal/"]
SKIP_DIRS = ["internal/api/generated/"]
API_PREFIX = "/api/v1"

# Routes allowed on the bare engine rather than under /api/v1. These are
# operational surfaces for a probe or a scraper, not part of the client-facing
# API, and deliberately carry no spec entry. Anything a client calls belongs in
# the spec instead of here.
ROOT_ALLOWED = {
    "/health": "liveness probe with DB connectivity, read by Docker and uptime checks",
    "/metrics": "Prometheus scrape target, gated on METRICS_ENABLED",
}

METHODS = ("GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS")

ROUTE_RE = re.compile(
    r"\b(\w+)\.(" + "|".join(METHODS) + r")\(\s*\"([^\"]*)\"",
)
GROUP_RE = re.compile(r"\b(\w+)\s*:?=\s*(\w+)\.Group\(\s*\"([^\"]*)\"")
REGISTRAR_DEF_RE = re.compile(
    r"^func (Register\w+)\(\s*(\w+) \*gin\.RouterGroup", re.MULTILINE
)
REGISTRAR_CALL_RE = re.compile(r"\b(?:handlers\.)?(Register\w+)\(\s*(\w+)\s*,")


def go_files():
    for d in GO_DIRS:
        for path in glob.glob(os.path.join(d, "**", "*.go"), recursive=True):
            if any(path.startswith(s) for s in SKIP_DIRS):
                continue
            if path.endswith("_test.go"):
                continue
            yield path


def load_spec_paths():
    """Return the spec's declared paths, and fail loudly on duplicate keys."""
    try:
        import yaml
    except ImportError:
        sys.exit("PyYAML is required: pip install pyyaml")

    class StrictLoader(yaml.SafeLoader):
        pass

    def no_duplicates(loader, node, deep=False):
        seen = set()
        for key_node, _ in node.value:
            key = loader.construct_object(key_node, deep=deep)
            if key in seen:
                raise AssertionError(f"duplicate key {key!r} in {SPEC}")
            seen.add(key)
        return yaml.SafeLoader.construct_mapping(loader, node, deep)

    StrictLoader.add_constructor(
        yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, no_duplicates
    )

    with open(SPEC) as fh:
        spec = yaml.load(fh, Loader=StrictLoader)

    declared = set()
    for path, item in spec.get("paths", {}).items():
        for method in item:
            if method.upper() in METHODS:
                declared.add((method.upper(), normalize(path)))
    return declared


def normalize(path):
    """Reduce a path to a comparable shape: gin ':id' and OpenAPI '{id}' both
    become '{}', since the parameter's name is not part of the route."""
    path = re.sub(r"\{[^}]*\}", "{}", path)
    path = re.sub(r":[A-Za-z_]\w*", "{}", path)
    path = re.sub(r"/\*\w+", "/{}", path)
    if len(path) > 1 and path.endswith("/"):
        path = path[:-1]
    return path


def resolve_prefixes(src, registrar_mounts, path):
    """Map each variable that holds a router or group to its path prefix."""
    prefixes = {"router": "", "r": "", "engine": ""}

    for name, defn in REGISTRAR_DEF_RE.findall(src):
        # The group main.go passes to this registrar.
        prefixes[defn] = registrar_mounts.get(name, API_PREFIX)

    # Resolve group chains; a few passes settle any declaration order.
    for _ in range(5):
        for child, parent, sub in GROUP_RE.findall(src):
            if parent in prefixes:
                prefixes[child] = prefixes[parent] + sub
    return prefixes


def collect_registrar_mounts():
    """Read main.go for `handlers.RegisterX(api, ...)` and record the prefix of
    the group each registrar is mounted on."""
    main = "cmd/api/main.go"
    if not os.path.exists(main):
        return {}
    src = open(main).read()
    prefixes = {"router": "", "r": "", "engine": ""}
    for _ in range(5):
        for child, parent, sub in GROUP_RE.findall(src):
            if parent in prefixes:
                prefixes[child] = prefixes[parent] + sub
    mounts = {}
    for name, group in REGISTRAR_CALL_RE.findall(src):
        if group in prefixes:
            mounts[name] = prefixes[group]
    return mounts


def collect_routes():
    """Return [(method, full_path, file, line)] for every hand-registered route."""
    registrar_mounts = collect_registrar_mounts()
    routes = []
    for path in go_files():
        src = open(path).read()
        prefixes = resolve_prefixes(src, registrar_mounts, path)
        for lineno, line in enumerate(src.split("\n"), 1):
            for recv, method, route in ROUTE_RE.findall(line):
                if recv not in prefixes:
                    # Not a router or group we can resolve (a mock, a client
                    # call, a helper); nothing to check.
                    continue
                routes.append((method, prefixes[recv] + route, path, lineno))
    return routes


def main():
    if not os.path.exists(SPEC):
        sys.exit(f"{SPEC} not found — run from the repository root")

    try:
        declared = load_spec_paths()
    except AssertionError as exc:
        print(f"FAIL: {exc}")
        return 1

    routes = collect_routes()
    missing, outside = [], []

    for method, full, path, lineno in routes:
        if not full.startswith(API_PREFIX):
            if full in ROOT_ALLOWED:
                continue
            outside.append((method, full, path, lineno))
            continue
        route = normalize(full[len(API_PREFIX):])
        if (method, route) not in declared:
            missing.append((method, full, path, lineno))

    if outside:
        print(f"FAIL: {len(outside)} route(s) registered outside {API_PREFIX}.")
        print("Every API endpoint lives under /api/v1. Operational endpoints that")
        print("legitimately do not may be listed in ROOT_ALLOWED with a reason.")
        for method, full, path, lineno in outside:
            print(f"  {method:6} {full}  ({path}:{lineno})")
        print()

    if missing:
        print(f"FAIL: {len(missing)} route(s) not declared in {SPEC}.")
        print("Add the operation to the spec and run `make generate`; a route that")
        print("is not in the spec cannot be reached by any generated client.")
        for method, full, path, lineno in missing:
            print(f"  {method:6} {full}  ({path}:{lineno})")
        print()

    if missing or outside:
        return 1

    checked = len(routes) - sum(1 for _, f, _, _ in routes if f in ROOT_ALLOWED)
    print(
        f"OK: {len(declared)} operations declared in the spec; "
        f"{checked} hand-registered route(s) all declared and under {API_PREFIX}."
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
