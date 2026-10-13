#!/usr/bin/env python3
"""
Update the `go` and `toolchain` directives in all Paladin go.mod files.

Usage:
  python3 update_go_directives.py <go-version> <toolchain-version>

Examples:
  python3 update_go_directives.py 1.26.0 go1.26.4
  python3 update_go_directives.py 1.27.0 go1.27.1

Run from the repo root. Inserts a `toolchain` line if one does not exist.
"""

import re
import os
import sys

MODULES = [
    "common/go",
    "config",
    "core/go",
    "domains/integration-test",
    "domains/noto",
    "domains/zeto",
    "operator",
    "registries/evm",
    "registries/static",
    "rpcauth/basicauth",
    "sdk/go",
    "signingmodules/example",
    "test",
    "testinfra",
    "toolkit/go",
    "transports/grpc",
]


def update_go_mod(path: str, go_version: str, toolchain_version: str) -> None:
    with open(path) as f:
        content = f.read()

    content = re.sub(
        r"^go \d+\.\d+(\.\d+)?$",
        f"go {go_version}",
        content,
        flags=re.MULTILINE,
    )

    if re.search(r"^toolchain ", content, re.MULTILINE):
        content = re.sub(
            r"^toolchain go.*$",
            f"toolchain {toolchain_version}",
            content,
            flags=re.MULTILINE,
        )
    else:
        content = re.sub(
            r"^(go " + re.escape(go_version) + r")$",
            r"\1" + f"\n\ntoolchain {toolchain_version}",
            content,
            flags=re.MULTILINE,
        )

    with open(path, "w") as f:
        f.write(content)


def main() -> None:
    if len(sys.argv) != 3:
        print(__doc__)
        sys.exit(1)

    go_version = sys.argv[1]
    toolchain_version = sys.argv[2]

    if not re.match(r"^\d+\.\d+\.\d+$", go_version):
        print(f"ERROR: go-version should be X.Y.Z (e.g. 1.26.0), got: {go_version}")
        sys.exit(1)

    if not re.match(r"^go\d+\.\d+\.\d+$", toolchain_version):
        print(f"ERROR: toolchain-version should be goX.Y.Z (e.g. go1.26.4), got: {toolchain_version}")
        sys.exit(1)

    updated = []
    skipped = []
    for mod in MODULES:
        path = f"{mod}/go.mod"
        if os.path.exists(path):
            update_go_mod(path, go_version, toolchain_version)
            updated.append(path)
        else:
            skipped.append(path)

    for p in updated:
        print(f"  updated: {p}")
    for p in skipped:
        print(f"  skipped (not found): {p}")

    print(f"\n{len(updated)} files updated, {len(skipped)} skipped.")


if __name__ == "__main__":
    main()
