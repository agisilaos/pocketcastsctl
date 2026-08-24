#!/usr/bin/env bash

set -euo pipefail

cd "$(dirname "$0")/.."

die() {
  echo "error: $*" >&2
  exit 1
}

allow_dirty=0
version=""
for arg in "$@"; do
  case "$arg" in
    --allow-dirty)
      allow_dirty=1
      ;;
    *)
      if [[ -z "$version" ]]; then
        version="$arg"
      else
        die "usage: scripts/release_preflight.sh [--allow-dirty] vX.Y.Z"
      fi
      ;;
  esac
done

if [[ -z "$version" ]]; then
  die "usage: scripts/release_preflight.sh [--allow-dirty] vX.Y.Z"
fi
if [[ "$version" != v* ]]; then
  version="v$version"
fi

if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  die "version must be semantic vX.Y.Z (got: $version)"
fi

if [[ "$(uname -s)" != "Darwin" ]]; then
  die "release preflight must run on macOS (Darwin)"
fi

for tool in go python3 git; do
  command -v "$tool" >/dev/null 2>&1 || die "$tool is required"
done

# Mirror release script safety assumptions.
git rev-parse --is-inside-work-tree >/dev/null 2>&1 || die "not inside a git work tree"
if [[ $allow_dirty -ne 1 ]]; then
  git diff --quiet || die "working tree has unstaged changes"
  git diff --cached --quiet || die "index has staged changes"
fi

git rev-parse "$version" >/dev/null 2>&1 && die "tag already exists: $version"

[[ -f CHANGELOG.md ]] || die "CHANGELOG.md not found"
if grep -Fq "## [$version]" CHANGELOG.md; then
  die "$version already exists in CHANGELOG.md"
fi

# Validate release notes generation range used by release script.
prev_tag="$(git describe --tags --abbrev=0 2>/dev/null || true)"
if [[ -n "$prev_tag" ]]; then
  git log --no-merges --pretty=format:'%H%x1f%s%x1f%b%x1e' "${prev_tag}..HEAD" >/dev/null
else
  git log --no-merges --pretty=format:'%H%x1f%s%x1f%b%x1e' HEAD >/dev/null
fi

# Ensure help contracts and docs snapshots are stable before release.
go test ./cmd/pocketcastsctl -run 'TestGoldenHelpRoot|TestGoldenHelpStart' >/dev/null
./scripts/check-help-docs-drift.sh >/dev/null

echo "release preflight passed for $version"
