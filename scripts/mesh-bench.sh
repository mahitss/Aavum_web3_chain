#!/usr/bin/env bash
# Ten parallel dispatches, then the hourly cap, then a revoke.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -f "${ROOT}/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "${ROOT}/.env"
  set +a
fi

ORCHESTRATOR_URL="${ORCHESTRATOR_URL:-http://127.0.0.1:8081}"
EXPLORER="${AEVUM_EXPLORER_URL:-https://explorer-test.qday.info}"
RPC="${AEVUM_RPC_URL:-https://rpc-test.qday.info}"

dispatch() {
  local prompt="$1"
  curl -sf -X POST "${ORCHESTRATOR_URL}/api/jobs" \
    -H "Content-Type: application/json" \
    -d "{\"prompt\":\"${prompt}\",\"task_type\":\"hash\"}"
}

echo "1. Dispatch 10 compute tasks in parallel"
ids=()
tmpdir="$(mktemp -d)"
for n in $(seq 1 10); do
  (
    dispatch "mesh-parallel-${n}-$(date +%s%N)" > "${tmpdir}/${n}.json"
  ) &
done
wait
for n in $(seq 1 10); do
  id="$(sed -n 's/.*"id":"\([^"]*\)".*/\1/p' "${tmpdir}/${n}.json")"
  if [[ -z "${id}" ]]; then
    echo "parallel dispatch ${n} was not accepted" >&2
    cat "${tmpdir}/${n}.json" >&2 || true
    exit 1
  fi
  ids+=("${id}")
  echo "  accepted ${id}"
done

echo "2. Wait for the parallel wave to reach a terminal state"
terminal=0
for _ in $(seq 1 40); do
  jobs="$(curl -sf "${ORCHESTRATOR_URL}/api/jobs?limit=100")"
  terminal=0
  for id in "${ids[@]}"; do
    row="$(printf '%s' "${jobs}" | tr '{' '\n' | grep "${id}" | head -n 1 || true)"
    if [[ "${row}" == *'"status":"SETTLED"'* || "${row}" == *'"status":"FAILED"'* ]]; then
      terminal=$((terminal + 1))
    fi
  done
  echo "  terminal ${terminal}/10"
  if [[ "${terminal}" -eq 10 ]]; then
    break
  fi
  sleep 3
done

echo "3. Keep dispatching until the hourly cap reverts a lock"
found_cap=false
for n in $(seq 1 45); do
  response="$(dispatch "mesh-cap-${n}-$(date +%s)")"
  id="$(printf '%s' "${response}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
  row=""
  for _ in $(seq 1 20); do
    jobs="$(curl -sf "${ORCHESTRATOR_URL}/api/jobs?limit=100")"
    row="$(printf '%s' "${jobs}" | tr '{' '\n' | grep "${id}" | head -n 1 || true)"
    if [[ "${row}" == *'"status":"SETTLED"'* || "${row}" == *'"status":"FAILED"'* ]]; then
      break
    fi
    sleep 2
  done
  if [[ "${row}" == *"ExceedsHourlyLimit"* ]]; then
    found_cap=true
    echo "  hourly cap reverted task ${id}"
    break
  fi
done
if [[ "${found_cap}" != true ]]; then
  echo "No job reverted with ExceedsHourlyLimit. A live session and funded locks are required before this assertion can pass." >&2
  exit 1
fi

echo "4. Revoke the agent key and confirm SessionInactive"
if [[ -z "${PRINCIPAL_PRIVATE_KEY:-}" || -z "${NEXUS_ESCROW_ADDRESS:-}" || -z "${ORCHESTRATOR_AGENT_ADDRESS:-}" ]]; then
  echo "Set PRINCIPAL_PRIVATE_KEY, NEXUS_ESCROW_ADDRESS, and ORCHESTRATOR_AGENT_ADDRESS before the revoke step." >&2
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
  --legacy >/dev/null

response="$(dispatch "mesh-revoked-$(date +%s)")"
id="$(printf '%s' "${response}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
row=""
for _ in $(seq 1 20); do
  jobs="$(curl -sf "${ORCHESTRATOR_URL}/api/jobs?limit=100")"
  row="$(printf '%s' "${jobs}" | tr '{' '\n' | grep "${id}" | head -n 1 || true)"
  if [[ "${row}" == *'"status":"FAILED"'* ]]; then
    break
  fi
  sleep 2
done
if [[ "${row}" != *"SessionInactive"* ]]; then
  echo "expected SessionInactive, got: ${row}" >&2
  exit 1
fi
echo "task 11 reverted with SessionInactive"
echo "Explorer: ${EXPLORER}"
