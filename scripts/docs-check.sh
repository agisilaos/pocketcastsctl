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

echo "[docs-check] validating shared docs contract"
python3 ./scripts/docs-contract-check.py

echo "[docs-check] validating help/docs snapshots"
./scripts/check-help.sh

echo "[docs-check] validating release command references"
grep -Fq 'make release-check VERSION=vX.Y.Z' README.md || die "README missing make release-check usage"
grep -Fq 'make release-dry-run VERSION=vX.Y.Z' README.md || die "README missing make release-dry-run usage"
grep -Fq 'make release VERSION=vX.Y.Z' README.md || die "README missing make release usage"
grep -Fq 'scripts/release-check.sh' README.md || die "README missing scripts/release-check.sh reference"
grep -Fq 'scripts/release.sh' README.md || die "README missing scripts/release.sh reference"

echo "[docs-check] ok"
