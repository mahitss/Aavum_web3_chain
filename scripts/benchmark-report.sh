#!/usr/bin/env bash
# Measure a live settlement. Writes docs/BENCHMARK_REPORT.md from responses that actually return.
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
REPORT="${ROOT}/docs/BENCHMARK_REPORT.md"
mkdir -p "${ROOT}/docs"

now_ms() { date +%s%3N; }

measure_post() {
  local path="$1"
  local body="$2"
  local started
  started="$(now_ms)"
  local response
  response="$(curl -sS -m 20 -X POST "${ORCHESTRATOR_URL}${path}" -H "Content-Type: application/json" -d "${body}" || true)"
  local elapsed=$(( $(now_ms) - started ))
  printf '%s\n%s\n' "${elapsed}" "${response}"
}

{
  echo "# Nexus402 benchmark"
  echo
  echo "Generated $(date -u +%Y-%m-%dT%H:%M:%SZ) against \`${ORCHESTRATOR_URL}\`."
  echo
  echo "This file records only measurements this script received. It does not invent transaction hashes."
  echo
} > "${REPORT}"

if ! curl -sf -m 5 "${ORCHESTRATOR_URL}/health" >/dev/null; then
  cat >> "${REPORT}" <<EOF
## Result

The orchestrator at \`${ORCHESTRATOR_URL}\` did not answer \`/health\`. No HTTP timing, gas, or explorer link was captured.

| Flow | Status |
|---|---|
| SessionAuthorized | Not recorded |
| TaskEscrowLocked and TaskCompleted | Not recorded |
| TaskRefunded | Not recorded |
| SessionRevoked | Not recorded |

Fund the principal, agent, and worker, deploy NexusEscrow, then rerun \`./scripts/benchmark-report.sh\`.
EOF
  echo "benchmark skipped: orchestrator health check failed" >&2
  exit 1
fi

echo "Orchestrator health check passed." >> "${REPORT}"
echo >> "${REPORT}"
echo "## Scenario dispatch latency" >> "${REPORT}"
echo >> "${REPORT}"
echo "| Scenario | HTTP ms | Response |" >> "${REPORT}"
echo "|---|---:|---|" >> "${REPORT}"

for scenario in happy_path worker_timeout_refund velocity_attack kill_switch_test; do
  read -r elapsed response < <(measure_post "/api/scenarios/run" "{\"scenario\":\"${scenario}\"}")
  compact="$(printf '%s' "${response}" | tr '\n' ' ' | cut -c1-180)"
  echo "| ${scenario} | ${elapsed} | ${compact} |" >> "${REPORT}"
done

cat >> "${REPORT}" <<EOF

## On-chain confirmation

Lock, claim, and refund confirmation times are not filled here until a job reaches \`SETTLED\` or \`REFUNDED\` and the script can read \`lock_tx_hash\`, \`claim_tx_hash\`, and \`refund_tx_hash\` from \`GET /api/jobs\`.

| Flow | Explorer |
|---|---|
| SessionAuthorized | Not recorded |
| TaskEscrowLocked and TaskCompleted | Not recorded |
| TaskRefunded | Not recorded |
| SessionRevoked | Not recorded |

Explorer base: ${EXPLORER}/tx/
EOF

echo "wrote ${REPORT}"
