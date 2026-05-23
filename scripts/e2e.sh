#!/usr/bin/env bash
# scripts/e2e.sh — double-apply idempotency harness for examples/working/
#
# For every directory under examples/working/, runs:
#   tofu init → apply -auto-approve → plan -detailed-exitcode (must be 0)
#   → destroy -auto-approve
#
# The plan-after-apply step is the idempotency gate: detailed-exitcode 2
# means the second apply would have detected drift, which is a regression.
#
# This script is invoked by `make test-e2e`, which is itself gated by
# FAKEGCP_ENABLE_E2E=1; the script does not re-check that gate.
#
# Usage:
#   ./scripts/e2e.sh                       # test every examples/working/* dir
#   ./scripts/e2e.sh basic_instance cloud_run   # test specific dirs by name
#
# Requirements: tofu or terraform in PATH; fakegcp on PATH or buildable.
#
# Exit codes: 0 = all passed, 1 = one or more failed.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
EXAMPLES_DIR="${REPO_ROOT}/examples/working"

# Pick the IaC binary. Prefer tofu (the upstream s/w in CI), fall back to terraform.
if command -v tofu &>/dev/null; then
  BIN=tofu
elif command -v terraform &>/dev/null; then
  BIN=terraform
else
  echo "ERROR: neither tofu nor terraform found in PATH" >&2
  exit 1
fi

# Pick a free TCP port for fakegcp.
PORT=$(python3 -c "import socket; s=socket.socket(); s.bind(('',0)); print(s.getsockname()[1]); s.close()" 2>/dev/null || \
       ruby -e "require 'socket'; s=TCPServer.new(0); puts s.addr[1]; s.close" 2>/dev/null || \
       echo 18080)

FAKEGCP_PID=""
cleanup() {
  if [[ -n "${FAKEGCP_PID}" ]]; then
    kill "${FAKEGCP_PID}" 2>/dev/null || true
  fi
}
trap cleanup EXIT

# Build or use installed fakegcp.
if command -v fakegcp &>/dev/null; then
  FAKEGCP_BIN=fakegcp
else
  echo "==> Building fakegcp..."
  go build -o /tmp/fakegcp-e2e "${REPO_ROOT}/cmd/fakegcp"
  FAKEGCP_BIN=/tmp/fakegcp-e2e
fi

echo "==> Starting fakegcp on port ${PORT}..."
"${FAKEGCP_BIN}" --port "${PORT}" &
FAKEGCP_PID=$!
sleep 1  # give it a moment to bind

# Fake credentials. The google provider needs *something* that looks like an
# access token; fakegcp itself does not validate. GOOGLE_PROJECT is consulted
# by the provider for resources that don't set `project` explicitly.
export GOOGLE_PROJECT="fake-project"
export GOOGLE_REGION="us-central1"
export GOOGLE_ZONE="us-central1-a"
export GOOGLE_OAUTH_ACCESS_TOKEN="fake-token"
export GOOGLE_CREDENTIALS=""  # explicitly blank: providers.tf supplies access_token
export TF_IN_AUTOMATION="1"

# Substitute the localhost:8080 endpoint baked into providers.tf with our
# random port. We do this on the copied tree (not the source), so the repo
# stays clean.
endpoint_port_rewrite() {
  local dir="$1"
  find "${dir}" -name '*.tf' -type f -print0 | \
    xargs -0 sed -i.bak "s|localhost:8080|localhost:${PORT}|g"
  find "${dir}" -name '*.tf.bak' -type f -delete
}

# Determine which examples to test.
DIRS=()
if [[ $# -gt 0 ]]; then
  for name in "$@"; do
    DIRS+=("${EXAMPLES_DIR}/${name}")
  done
else
  while IFS= read -r dir; do
    DIRS+=("$dir")
  done < <(find "${EXAMPLES_DIR}" -mindepth 1 -maxdepth 1 -type d | sort)
fi

PASSED=()
FAILED=()

for dir in "${DIRS[@]}"; do
  name="$(basename "${dir}")"
  echo ""
  echo "════════════════════════════════════════"
  echo "  ${name}"
  echo "════════════════════════════════════════"

  tmp=$(mktemp -d)
  cp -r "${dir}/." "${tmp}/"
  endpoint_port_rewrite "${tmp}"

  run_ok=true

  # Reset fakegcp state between examples for hermetic runs.
  curl -s -X POST "http://localhost:${PORT}/mock/reset" >/dev/null || true

  if ! "${BIN}" -chdir="${tmp}" init -input=false -no-color -reconfigure 2>&1; then
    echo "FAIL: init failed for ${name}" >&2
    run_ok=false
  fi

  if $run_ok && ! "${BIN}" -chdir="${tmp}" apply -auto-approve -input=false -no-color 2>&1; then
    echo "FAIL: apply failed for ${name}" >&2
    run_ok=false
  fi

  if $run_ok; then
    # -detailed-exitcode: 0 = no changes, 1 = error, 2 = drift detected.
    set +e
    "${BIN}" -chdir="${tmp}" plan -detailed-exitcode -input=false -no-color 2>&1
    plan_exit=$?
    set -e
    if [[ ${plan_exit} -eq 2 ]]; then
      echo "FAIL: second apply is not idempotent (drift detected) for ${name}" >&2
      run_ok=false
    elif [[ ${plan_exit} -eq 1 ]]; then
      echo "FAIL: plan error for ${name}" >&2
      run_ok=false
    fi
  fi

  if $run_ok && ! "${BIN}" -chdir="${tmp}" destroy -auto-approve -input=false -no-color 2>&1; then
    echo "FAIL: destroy failed for ${name}" >&2
    run_ok=false
  fi

  rm -rf "${tmp}"

  if $run_ok; then
    PASSED+=("${name}")
    echo "PASS: ${name}"
  else
    FAILED+=("${name}")
  fi
done

echo ""
echo "════════════════════════════════════════"
echo "Results: ${#PASSED[@]} passed, ${#FAILED[@]} failed"
for p in "${PASSED[@]+"${PASSED[@]}"}"; do echo "  PASS ${p}"; done
for f in "${FAILED[@]+"${FAILED[@]}"}"; do echo "  FAIL ${f}"; done
echo "════════════════════════════════════════"

[[ ${#FAILED[@]} -eq 0 ]]
