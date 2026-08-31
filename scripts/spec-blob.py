#!/usr/bin/env python3
"""Decode the OpenAPI spec embedded in a generated spec.go.

Writes the decoded spec to stdout as raw bytes. Used to compare two spec.go
files by content rather than by their compressed encoding.
"""

import base64
import re
import sys
import zlib


def decode(path: str) -> bytes:
    source = open(path, encoding="utf-8").read()
    match = re.search(r"swaggerSpec\s*=\s*\[\]string\{(.*?)\n\}", source, re.S)
    if match is None:
        raise SystemExit(f"{path}: no swaggerSpec literal found")
    chunks = re.findall(r'"((?:[^"\\]|\\.)*)"', match.group(1))
    if not chunks:
        raise SystemExit(f"{path}: swaggerSpec literal is empty")
    # oapi-codegen emits base64 over a raw DEFLATE stream (no zlib/gzip header)
    return zlib.decompressobj(-zlib.MAX_WBITS).decompress(base64.b64decode("".join(chunks)))


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("usage: spec-blob.py <path-to-spec.go>")
    sys.stdout.buffer.write(decode(sys.argv[1]))
