#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${ROOT_DIR}/docs/help"
SNAPSHOT_LIST="${ROOT_DIR}/scripts/help-snapshots.txt"
check=0

usage() {
  cat <<USAGE
Usage: $(basename "$0") [--check] [--out-dir <path>]

Generate help snapshots listed in scripts/help-snapshots.txt into docs/help.
  --check           Check for stale, missing, or unregistered snapshots without writing.
  --out-dir <path>  Use another snapshot directory (relative to the caller's directory).
USAGE
}

die() {
  echo "error: $*" >&2
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --check)
      check=1
      shift
      ;;
    --out-dir)
      [[ -n "${2:-}" ]] || { echo "error: --out-dir requires a path" >&2; usage >&2; exit 2; }
      OUT_DIR="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown arg: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

# Validate the entire registry before building or writing any snapshot.
files=()
commands=()
while IFS= read -r line || [[ -n "${line:-}" ]]; do
  [[ -z "${line}" || "${line}" =~ ^# ]] && continue
  [[ "${line}" == *$'\t'* ]] || die "invalid help snapshot entry: expected filename<TAB>command"
  file="${line%%$'\t'*}"
  command="${line#*$'\t'}"
  [[ "${file}" =~ ^[a-zA-Z0-9_-]+\.txt$ ]] || die "invalid help snapshot filename: ${file}"
  [[ -n "${command//[[:space:]]/}" && "${command}" != *$'\t'* ]] || die "invalid help snapshot command for ${file}"
  for ((j=0; j<${#files[@]}; j++)); do
    [[ "${file}" != "${files[$j]}" ]] || die "duplicate help snapshot filename: ${file}"
  done
  files+=("${file}")
  commands+=("${command}")
done < "${SNAPSHOT_LIST}"
[[ ${#files[@]} -gt 0 ]] || die "help snapshot registry is empty"

# Do not silently preserve or delete files outside the registry, even on update.
for path in "${OUT_DIR}"/* "${OUT_DIR}"/.[!.]* "${OUT_DIR}"/..?*; do
  [[ -e "${path}" || -L "${path}" ]] || continue
  registered=0
  for file in "${files[@]}"; do
    [[ "${path##*/}" != "${file}" ]] || registered=1
  done
  [[ "${registered}" -eq 1 ]] || die "unregistered help snapshot: ${path}; add it to scripts/help-snapshots.txt or remove it"
  [[ -f "${path}" ]] || die "help snapshot is not a file: ${path}"
done

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "${WORK_DIR}"' EXIT
BIN_PATH="${WORK_DIR}/pocketcastsctl-help"
(cd "${ROOT_DIR}" && go build -o "${BIN_PATH}" ./cmd/pocketcastsctl)

for ((i=0; i<${#files[@]}; i++)); do
  read -r -a cmd_args <<< "${commands[$i]}"
  "${BIN_PATH}" "${cmd_args[@]}" > "${WORK_DIR}/${files[$i]}"
done

if [[ ${check} -eq 1 ]]; then
  for file in "${files[@]}"; do
    if ! diff -u "${OUT_DIR}/${file}" "${WORK_DIR}/${file}"; then
      echo "help output drift detected in ${file}; snapshot is stale or missing" >&2
      echo "run: scripts/update-help.sh --out-dir \"${OUT_DIR}\"" >&2
      exit 1
    fi
  done
  echo "help snapshots are up to date"
else
  mkdir -p "${OUT_DIR}"
  for file in "${files[@]}"; do
    cp "${WORK_DIR}/${file}" "${OUT_DIR}/${file}"
  done
  echo "Updated help snapshots in ${OUT_DIR}"
fi
