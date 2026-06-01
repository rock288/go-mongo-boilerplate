#!/usr/bin/env bash
# Local equivalent of the GitHub Actions Init Matrix.
#
# Runs each combo against a fresh rsync copy of the working tree under
# /tmp/init-matrix-<combo>/. Does NOT touch the repo it was launched from.
#
# Prereqs in PATH: go, wire, mockery (the latter two usually live in
# $(go env GOPATH)/bin — add that to PATH first).
set -euo pipefail

SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_ROOT="${TMP_ROOT:-/tmp/init-matrix}"

declare -a COMBOS=(
  "full|"
  "minimal|--no-kafka --no-sqs --no-observability --no-samples"
  "no-kafka|--no-kafka"
  "no-sqs|--no-sqs"
  "no-otel|--no-observability"
  "no-samples|--no-samples"
  "no-messaging|--no-kafka --no-sqs"
)

if ! command -v wire >/dev/null || ! command -v mockery >/dev/null; then
  echo "wire or mockery missing from PATH (try: PATH=\$(go env GOPATH)/bin:\$PATH $0)"
  exit 1
fi

fail=0
rm -rf "$TMP_ROOT"
mkdir -p "$TMP_ROOT"

for entry in "${COMBOS[@]}"; do
  name="${entry%%|*}"
  args="${entry#*|}"
  dest="$TMP_ROOT/$name"
  echo
  echo "===== combo: $name ====="
  rsync -a --exclude='.git' --exclude='bin' --exclude='cmd/init/init' \
    "$SRC/" "$dest/"
  (cd "$dest" && git init -q && git add -A && \
     git -c user.email=ci@example.com -c user.name=ci commit -q -m before)

  if ! (cd "$dest/cmd/init" && go run . --root=../.. --non-interactive --force \
        --module="github.com/test/$name" $args); then
    echo "::error::init failed for $name"
    fail=1
    continue
  fi

  if grep -rnE "feature:[a-z]+:(start|end)" \
       --include='*.go' --include='*.yaml' --include='*.yml' \
       --include='*.md' --include='Makefile' --include='Dockerfile' \
       --include='.env.example' "$dest" >/dev/null; then
    echo "::error::residual feature markers in $name"
    fail=1
    continue
  fi

  if [ -d "$dest/cmd/init" ]; then
    echo "::error::cmd/init not removed in $name"
    fail=1
    continue
  fi

  if grep -q "charmbracelet/huh" "$dest/go.mod"; then
    echo "::error::huh leaked into root go.mod in $name"
    fail=1
    continue
  fi

  # air: server config always present; worker config rides the kafka+sqs cascade.
  if [ ! -f "$dest/.air.toml" ]; then
    echo "::error::.air.toml missing in $name"
    fail=1
    continue
  fi
  if [[ "$args" == *"--no-kafka"* && "$args" == *"--no-sqs"* ]] && [ -f "$dest/.air.worker.toml" ]; then
    echo "::error::.air.worker.toml not removed by worker cascade in $name"
    fail=1
    continue
  fi

  if ! (cd "$dest" && go build ./... && go test -short ./...); then
    echo "::error::build/test failed in $name"
    fail=1
    continue
  fi

  echo "===== $name OK ====="
done

if [ "$fail" -ne 0 ]; then
  echo
  echo "Init matrix FAILED — see errors above. Inspect each combo under $TMP_ROOT/"
  exit 1
fi

echo
echo "All combos passed. Cleanup: rm -rf $TMP_ROOT"
