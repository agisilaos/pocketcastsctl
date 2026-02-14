#!/usr/bin/env bash

set -euo pipefail

cd "$(dirname "$0")/.."

update=0
if [[ "${1:-}" == "--update" ]]; then
  update=1
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

root_out="$tmp_dir/help-root.txt"
start_out="$tmp_dir/help-start.txt"

go run ./cmd/pocketcastsctl help > "$root_out"
go run ./cmd/pocketcastsctl help start > "$start_out"

root_doc="docs/cli-help/help-root.txt"
start_doc="docs/cli-help/help-start.txt"

if [[ $update -eq 1 ]]; then
  mkdir -p docs/cli-help
  cp "$root_out" "$root_doc"
  cp "$start_out" "$start_doc"
  echo "Updated docs/cli-help snapshots."
  exit 0
fi

if ! diff -u "$root_doc" "$root_out"; then
  echo "help root output drifted from docs snapshot ($root_doc). Run: scripts/check-help-docs-drift.sh --update" >&2
  exit 1
fi

if ! diff -u "$start_doc" "$start_out"; then
  echo "help start output drifted from docs snapshot ($start_doc). Run: scripts/check-help-docs-drift.sh --update" >&2
  exit 1
fi

echo "Help/docs snapshots are in sync."
