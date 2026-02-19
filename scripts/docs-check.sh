#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

die() {
  echo "error: $*" >&2
  exit 1
}

if [[ "$(uname -s)" != "Darwin" ]]; then
  die "docs-check.sh must be run on macOS (Darwin)"
fi

[[ -f README.md ]] || die "README.md not found"
[[ -f CHANGELOG.md ]] || die "CHANGELOG.md not found"

echo "[docs-check] validating help/docs snapshots"
./scripts/check-help-docs-drift.sh

echo "[docs-check] validating release command references"
rg -q 'make release-check VERSION=vX.Y.Z' README.md || die "README missing make release-check usage"
rg -q 'make release-dry-run VERSION=vX.Y.Z' README.md || die "README missing make release-dry-run usage"
rg -q 'make release VERSION=vX.Y.Z' README.md || die "README missing make release usage"
rg -q 'scripts/release-check.sh' README.md || die "README missing scripts/release-check.sh reference"
rg -q 'scripts/release.sh' README.md || die "README missing scripts/release.sh reference"

echo "[docs-check] ok"
