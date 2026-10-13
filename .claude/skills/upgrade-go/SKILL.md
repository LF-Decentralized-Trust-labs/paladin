---
name: upgrade-go
description: Upgrades the Go toolchain version across the Paladin workspace. Use when bumping the Go version, fixing CVEs in golang.org/x/* packages that require a newer Go version, or when go.work and go.mod files need updated `go` or `toolchain` directives. Covers all go.mod modules, Dockerfiles, GitHub Actions CI setup, and tooling compatibility (controller-gen).
---

# Go Toolchain Upgrade — Paladin Workspace

## Phase 1: Discovery

Run before making any changes to establish full scope:

```bash
# Workspace directives
grep -n "^go \|^toolchain " go.work go.work.base

# All module directives (should all match after upgrade)
grep -rh "^go \|^toolchain " */go.mod */*/go.mod 2>/dev/null | sort -u

# Dockerfiles with hardcoded Go versions
grep -rn "golang:1\.\|GO_VERSION=" --include="Dockerfile*" .

# CI
grep -rn "go-version:" .github/

# Makefile tool versions that may need compatibility checks
grep -rn "CONTROLLER_TOOLS_VERSION\|MOCKERY_VERSION\|GOLANGCI_LINT_VERSION" operator/Makefile build.gradle
```

## Phase 2: go.work and go.work.base

Update both files — use the full patch version in both directives:

```
go 1.26.4
toolchain go1.26.4
```

## Phase 3: All 16 go.mod files

Run all steps from the repo root.

**Step 1 — upgrade target packages** (e.g. CVE packages; omit packages not in a module's graph — go get no-ops them):

```bash
for mod in common/go config core/go domains/integration-test domains/noto domains/zeto \
  operator registries/evm registries/static rpcauth/basicauth sdk/go \
  signingmodules/example test testinfra toolkit/go transports/grpc; do
  echo "=== $mod ==="
  (cd $mod && GOFLAGS=-mod=mod go get golang.org/x/crypto@<ver> golang.org/x/net@<ver> 2>&1)
done
```

**Step 2 — set the go directive explicitly** (`go get` only sets the minimum a dep requires, not the intended target version):

```bash
python3 .claude/skills/upgrade-go/scripts/update_go_directives.py 1.26.0 go1.26.4
```

**Step 3 — tidy** using a fresh `GOMODCACHE` (required — see [reference.md](reference.md#gomodcache-trick)):

```bash
export GOMODCACHE=$(mktemp -d)
for mod in common/go config core/go domains/integration-test domains/noto domains/zeto \
  operator registries/evm registries/static rpcauth/basicauth sdk/go \
  signingmodules/example test testinfra toolkit/go transports/grpc; do
  echo "=== $mod ==="
  (cd $mod && go mod tidy 2>&1) && echo "OK: $mod" || echo "FAILED: $mod"
done
```

## Phase 4: Dockerfiles

Four files with hardcoded Go versions:

| File | Pattern | Example |
|------|---------|---------|
| `Dockerfile` | `ARG GO_VERSION=X.Y.Z` | `1.26.4` (full patch) |
| `operator/Dockerfile` | `FROM golang:X.Y-bookworm` | `1.26` (minor only) |
| `testinfra/besu_bootstrap/Dockerfile` | `FROM golang:X.Y-bookworm` | `1.26` |
| `test/Dockerfile` | `FROM golang:X.Y-bookworm AS go-builder` | `1.26` |

Note: `Dockerfile` installs Go from go.dev and needs the full patch version. The `golang:` base image Dockerfiles use the minor version tag and automatically get the latest patch.

## Phase 5: GitHub Actions CI

Two files to update:

- `.github/actions/setup/action.yaml` — `go-version: 'X.Y'` (shared action used by all build workflows — easy to miss)
- `.github/workflows/docs.yaml` — also has a standalone `go-version:` entry

## Phase 6: Tooling compatibility

After a Go version bump, tools installed via `go install tool@vX.Y.Z` in Makefiles may break due to transitive dependency incompatibilities with the new Go version.

**controller-gen** is the most common case. See [reference.md — controller-gen compatibility](reference.md#controller-gen-compatibility) for the CT/k8s/Go version table and how to pick the right bump for `CONTROLLER_TOOLS_VERSION` in `operator/Makefile`.

**General rule**: if a tool fails with `compile: invalid array length` or `requires newer Go version go1.X`, find the lowest tool version whose own `go` directive is ≥ the target, while remaining compatible with the module's current k8s/dependency versions.

## Phase 7: Verify

```bash
# All modules on consistent version
grep -rh "^go \|^toolchain " */go.mod | sort -u

# Target packages at correct version
grep -rh "golang.org/x/crypto\|golang.org/x/net\|golang.org/x/sys" */go.mod | sort -u

# No stale Dockerfiles
grep -rn "golang:1\." --include="Dockerfile*" .

# No stale CI
grep -rn "go-version:" .github/
```

## Communicating to developers

Include in the PR description or release notes so developers know what to run after pulling:

> After pulling these changes, clear your local caches before building:
>
> **Go + Gradle** (stale stdlib objects cause `compile: version mismatch` errors):
> ```bash
> go clean -cache && go clean -testcache
> ./gradlew --stop && ./gradlew clean
> ```
>
> **Docker** (only if you have previously built the images locally — Docker layer caching can reuse the old `golang:1.X` base even after the Dockerfile is updated):
> ```bash
> docker build --no-cache -f <path/to/Dockerfile> .
> ```

## Additional reference

- GOMODCACHE trick and why `go mod tidy` fails without it: [reference.md](reference.md)
- controller-gen / k8s / Go compatibility table: [reference.md#controller-gen-compatibility](reference.md#controller-gen-compatibility)
- `update_go_directives.py` details: [scripts/update_go_directives.py](scripts/update_go_directives.py)
