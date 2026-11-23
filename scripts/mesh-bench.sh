#!/usr/bin/env bash
# Five paid settlements through the orchestrator, then one revoke.
# Task 6 must fail on-chain with SessionInactive.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -f "${ROOT}/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "${ROOT}/.env"
  set +a
fi

WORKER_URL="${WORKER_URL:-http://127.0.0.1:8080}"
ORCHESTRATOR_URL="${ORCHESTRATOR_URL:-http://127.0.0.1:8081}"
EXPLORER="${AEVUM_EXPLORER_URL:-https://explorer-test.qday.info}"
RPC="${AEVUM_RPC_URL:-https://rpc-test.qday.info}"

now_ms() {
  date +%s%3N
}

json_string() {
  printf '%s' "$1" | sed -n "s/.*\"$2\":\"\\([^\"]*\\)\".*/\\1/p" | head -n 1
}

json_number() {
  printf '%s' "$1" | sed -n "s/.*\"$2\":\\([0-9][0-9]*\\).*/\\1/p" | head -n 1
}

job_row() {
  local jobs="$1"
  local id="$2"
  printf '%s' "${jobs}" | tr '{' '\n' | grep "${id}" | head -n 1 || true
}

# Direct unpaid POST. This is the 402 negotiation, before the orchestrator locks.
measure_402() {
  local prompt="$1"
  local start end code
  start="$(now_ms)"
  code="$(curl -sS -o /tmp/nexus402-bench-body.json -D /tmp/nexus402-bench.hdr -w "%{http_code}" \
    -X POST "${WORKER_URL}/api/v1/compute" \
    -H "Content-Type: application/json" \
    --data-binary "{\"prompt\":\"${prompt}\",\"task_type\":\"hash\"}")"
  end="$(now_ms)"
  if [[ "${code}" != "402" ]]; then
    echo "task probe expected HTTP 402, got ${code}" >&2
    exit 1
  fi
  echo $((end - start))
}

wait_job() {
  local id="$1"
  local started="$2"
  local row="" state="" lock_tx="" claim_tx="" task_id="" lock_block="" latency=""
  local lock_seen_ms="" 
  local i
  for i in $(seq 1 90); do
    local jobs
    jobs="$(curl -sS "${ORCHESTRATOR_URL}/api/jobs")"
    row="$(job_row "${jobs}" "${id}")"
    state="$(json_string "${row}" status)"
    lock_tx="$(json_string "${row}" lock_tx_hash)"
    if [[ -n "${lock_tx}" && -z "${lock_seen_ms}" ]]; then
      lock_seen_ms="$(( $(now_ms) - started ))"
    fi
    if [[ "${state}" == "SETTLED" || "${state}" == "FAILED" ]]; then
      claim_tx="$(json_string "${row}" claim_tx_hash)"
      task_id="$(json_string "${row}" task_id)"
      lock_block="$(json_number "${row}" lock_block)"
      latency="$(json_number "${row}" latency_ms)"
      break
    fi
    sleep 2
  done
  printf '%s\n' "${state}" "${task_id}" "${lock_tx}" "${lock_block}" "${lock_seen_ms}" "${claim_tx}" "${latency}" "${row}"
}

run_settlement() {
  local n="$1"
  local prompt="mesh-bench-${n}-$(date +%s)"
  local negotiate dispatch id started
  negotiate="$(measure_402 "${prompt}")"
  started="$(now_ms)"
  dispatch="$(curl -sS -X POST "${ORCHESTRATOR_URL}/api/jobs" \
    -H "Content-Type: application/json" \
    --data-binary "{\"prompt\":\"${prompt}\",\"task_type\":\"hash\"}")"
  id="$(json_string "${dispatch}" id)"
  if [[ -z "${id}" ]]; then
    echo "orchestrator rejected task ${n}: ${dispatch}" >&2
    exit 1
  fi

  local state task_id lock_tx lock_block lock_ms claim_tx latency row
  mapfile -t fields < <(wait_job "${id}" "${started}")
  state="${fields[0]:-}"
  task_id="${fields[1]:-}"
  lock_tx="${fields[2]:-}"
  lock_block="${fields[3]:-}"
  lock_ms="${fields[4]:-}"
  claim_tx="${fields[5]:-}"
  latency="${fields[6]:-}"
  row="${fields[7]:-}"

  echo "task ${n}"
  echo "  derived_task_id=${task_id:-<none>}"
  echo "  http_402_ms=${negotiate}"
  echo "  escrow_lock_tx=${lock_tx:-<none>}"
  echo "  escrow_lock_block=${lock_block:-<none>}"
  echo "  escrow_confirm_ms=${lock_ms:-<none>}"
  echo "  claim_tx=${claim_tx:-<none>}"
  echo "  e2e_settlement_ms=${latency:-<none>}"
  if [[ -n "${lock_tx}" ]]; then
    echo "  TaskEscrowLocked: ${EXPLORER}/tx/${lock_tx}"
  fi
  if [[ -n "${claim_tx}" ]]; then
    echo "  TaskCompleted: ${EXPLORER}/tx/${claim_tx}"
  fi

  if [[ "${state}" != "SETTLED" ]]; then
    echo "task ${n} did not settle (${state:-unknown}): ${row}" >&2
    exit 1
  fi
}

echo "Nexus402 mesh bench — five settlements, then revoke"
for n in 1 2 3 4 5; do
  run_settlement "${n}"
done

echo "task 6 — emergencyRevokeSession, then one more dispatch"
if [[ -z "${PRINCIPAL_PRIVATE_KEY:-}" || -z "${NEXUS_ESCROW_ADDRESS:-}" || -z "${ORCHESTRATOR_AGENT_ADDRESS:-}" ]]; then
  echo "Set PRINCIPAL_PRIVATE_KEY, NEXUS_ESCROW_ADDRESS, and ORCHESTRATOR_AGENT_ADDRESS before task 6." >&2
  exit 1
fi
if ! command -v cast >/dev/null 2>&1; then
  echo "cast is required to send emergencyRevokeSession." >&2
  exit 1
fi

cast send "${NEXUS_ESCROW_ADDRESS}" \
  "emergencyRevokeSession(address)" "${ORCHESTRATOR_AGENT_ADDRESS}" \
  --private-key "${PRINCIPAL_PRIVATE_KEY}" \
  --rpc-url "${RPC}" \
  --legacy

prompt="mesh-bench-6-$(date +%s)"
started="$(now_ms)"
dispatch="$(curl -sS -X POST "${ORCHESTRATOR_URL}/api/jobs" \
  -H "Content-Type: application/json" \
  --data-binary "{\"prompt\":\"${prompt}\",\"task_type\":\"hash\"}")"
id="$(json_string "${dispatch}" id)"
if [[ -z "${id}" ]]; then
  echo "orchestrator rejected the post-revoke dispatch: ${dispatch}" >&2
  exit 1
fi

mapfile -t fields < <(wait_job "${id}" "${started}")
state="${fields[0]:-}"
row="${fields[7]:-}"
echo "task 6 status=${state:-unknown}"
if [[ "${state}" != "FAILED" ]] || ! printf '%s' "${row}" | grep -q "SessionInactive"; then
  echo "expected FAILED with SessionInactive, got: ${row}" >&2
  exit 1
fi
echo "task 6 reverted with SessionInactive"
