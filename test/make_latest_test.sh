#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_contains() {
  local file="$1"
  local text="$2"
  grep -F -- "$text" "$file" >/dev/null || fail "expected $file to contain: $text"
}

test_latest_target_writes_minimal_yaml() {
  local tmp dist latest
  tmp="$(mktemp -d)"
  dist="$tmp/dist"
  latest="$dist/latest.yaml"

  make -C "$ROOT_DIR" latest DIST_DIR="$dist" VERSION=1.2.3 BUILD_TIME=2026-05-25T14:30:00 >/dev/null

  [[ -f "$latest" ]] || fail "expected latest.yaml to be generated"
  assert_contains "$latest" "name: eget"
  assert_contains "$latest" "version: 1.2.3"
  assert_contains "$latest" "released_at: 2026-05-25T14:30:00"

  if grep -Eq '^(base_url|files):' "$latest"; then
    fail "latest.yaml should not contain base_url or files"
  fi
}

test_build_all_depends_on_latest() {
  local db
  db="$(mktemp)"
  make -C "$ROOT_DIR" -pn build-all > "$db"
  grep -E '^build-all: .* latest( |$)' "$db" >/dev/null || fail "expected build-all to depend on latest"
}

test_latest_target_writes_minimal_yaml
test_build_all_depends_on_latest

echo "make latest tests passed"
