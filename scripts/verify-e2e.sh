#!/usr/bin/env bash
set -euo pipefail

WORKER_URL="${WORKER_URL:-http://127.0.0.1:8080}"
ORCHESTRATOR_URL="${ORCHESTRATOR_URL:-http://127.0.0.1:8081}"
EXPLORER="${AEVUM_EXPLORER_URL:-https://explorer-test.qday.info}"

echo "1. Unpaid worker request must return HTTP 402"
status="$(curl -sS -o /tmp/nexus402-402.json -w "%{http_code}" \
  -X POST "${WORKER_URL}/api/v1/compute" \
  -H "Content-Type: application/json" \
  -D /tmp/nexus402-402.hdr \
  -d '{"prompt":"verify-e2e","task_type":"hash"}')"
if [[ "${status}" != "402" ]]; then
  echo "expected 402, got ${status}"
  exit 1
fi
grep -qi "Nexus402-v1" /tmp/nexus402-402.hdr
grep -qi "x-payment-amount: 250000" /tmp/nexus402-402.hdr
echo "402 challenge ok"

echo "2. Dispatch through the orchestrator"
dispatch="$(curl -sS -X POST "${ORCHESTRATOR_URL}/api/jobs" \
  -H "Content-Type: application/json" \
  -d '{"prompt":"verify-e2e","task_type":"hash"}')"
echo "${dispatch}"
job_id="$(printf '%s' "${dispatch}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
if [[ -z "${job_id}" ]]; then
  echo "orchestrator did not accept the job"
  exit 1
fi

echo "3. Wait until the job is SETTLED"
lock_tx=""
claim_tx=""
state=""
for _ in $(seq 1 40); do
  jobs="$(curl -sS "${ORCHESTRATOR_URL}/api/jobs")"
  row="$(printf '%s' "${jobs}" | tr '{' '\n' | grep "${job_id}" || true)"
  state="$(printf '%s' "${row}" | sed -n 's/.*"status":"\([^"]*\)".*/\1/p')"
  echo "status=${state:-pending}"
  if [[ "${state}" == "SETTLED" || "${state}" == "FAILED" ]]; then
    lock_tx="$(printf '%s' "${row}" | sed -n 's/.*"lock_tx_hash":"\([^"]*\)".*/\1/p')"
    claim_tx="$(printf '%s' "${row}" | sed -n 's/.*"claim_tx_hash":"\([^"]*\)".*/\1/p')"
    break
  fi
  sleep 3
done

if [[ "${state}" != "SETTLED" ]]; then
  echo "job did not reach SETTLED (last status: ${state:-unknown})"
  exit 1
fi

echo "TaskEscrowLocked: ${EXPLORER}/tx/${lock_tx}"
echo "TaskCompleted: ${EXPLORER}/tx/${claim_tx}"
