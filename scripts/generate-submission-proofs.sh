#!/usr/bin/env bash
# Capture live scenario receipts. Writes only hashes the orchestrator returned and the explorer confirmed.
set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -f "${ROOT}/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "${ROOT}/.env"
  set +a
fi

ORCH="${RENDER_ORCHESTRATOR_URL:-${ORCHESTRATOR_URL:-http://127.0.0.1:8081}}"
ORCH="${ORCH%/}"
EXPLORER="${AEVUM_EXPLORER_URL:-https://explorer-test.qday.info}"
EXPLORER="${EXPLORER%/}"
OUT="${ROOT}/docs/VERIFIED_ONCHAIN_PROOFS.md"
ESCROW="${NEXUS_ESCROW_ADDRESS:-}"
IDENTITY="${NEXUS_IDENTITY_REGISTRY_ADDRESS:-${NEXUS_IDENTITY_ADDRESS:-}}"
STAMP="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
FAIL=0
ROWS=""

mkdir -p "${ROOT}/docs"

now_ms() { node -e "console.log(Date.now())"; }

format_units() {
  node -e "const v=process.argv[1]; if(!v||v==='null'||v==='undefined'){console.log('not returned'); process.exit(0)} try { const n=BigInt(v); const w=n/1000000n; const f=(n%1000000n).toString().padStart(6,'0'); console.log(w.toString()+'.'+f) } catch { console.log('not returned') }" "$1"
}

confirm_tx() {
  local hash="$1"
  local code
  if [[ -z "${hash}" || "${hash}" == "null" || "${hash}" == "not returned" ]]; then
    printf 'not captured'
    return 1
  fi
  local tmp
  tmp="$(mktemp)"
  code="$(curl -sS -m 25 -o "${tmp}" -w '%{http_code}' "${EXPLORER}/api/v2/transactions/${hash}" || true)"
  rm -f "${tmp}"
  if [[ "${code}" == "200" ]]; then
    printf 'confirmed'
    return 0
  fi
  printf 'unconfirmed (HTTP %s)' "${code:-none}"
  return 1
}

link_for() {
  local hash="$1"
  local label="$2"
  if [[ -z "${hash}" || "${hash}" == "null" || "${hash}" == "not returned" ]]; then
    printf 'not captured'
    return
  fi
  printf '[%s](%s/tx/%s)' "${label}" "${EXPLORER}" "${hash}"
}

pick_job() {
  local started="$1"
  local want_id="$2"
  local body_file="$3"
  node -e '
const fs = require("fs");
const started = Number(process.argv[1]);
const want = process.argv[2];
const jobs = JSON.parse(fs.readFileSync(process.argv[3], "utf8"));
const list = Array.isArray(jobs) ? jobs : [];
const terminal = new Set(["SETTLED","REFUNDED","FAILED","BLOCKED_HOURLY_CAP","BLOCKED_REVOKED","REJECTED","SUBMITTED"]);
let job = want ? list.find((item) => item.id === want) : null;
if (!job) {
  const fresh = list.filter((item) => Date.parse(item.created_at || "") >= started - 2000);
  job = fresh.length ? fresh[fresh.length - 1] : null;
}
const done = Boolean(job && (terminal.has(job.status) || job.claim_tx_hash || job.refund_tx_hash || job.error));
const out = {
  done,
  id: job && job.id || "",
  status: job && job.status || "",
  task_id: job && job.task_id || "",
  lock_tx_hash: job && job.lock_tx_hash || "",
  claim_tx_hash: job && job.claim_tx_hash || "",
  refund_tx_hash: job && job.refund_tx_hash || "",
  result_hash: job && job.result_hash || "",
  amount: job && job.amount != null ? String(job.amount) : "",
  actual_amount: job && job.actual_amount || "",
  refunded_change: job && job.refunded_change || "",
  error: job && job.error || ""
};
process.stdout.write(JSON.stringify(out));
' "${started}" "${want_id}" "${body_file}"
}

capture() {
  local name="$1"
  local scheme="$2"
  local started response want_id body_file picked elapsed
  local task_id lock_tx claim_tx refund_tx result_hash amount actual refunded status erro
  started="$(now_ms)"
  response="$(curl -sS -m 30 -X POST "${ORCH}/api/scenarios/run" -H 'Content-Type: application/json' -d "{\"scenario\":\"${name}\"}" || true)"
  printf '%s' "${response}" > "${ROOT}/docs/.proof-response.json"
  want_id="$(node -e "const fs=require('fs'); try{const b=JSON.parse(fs.readFileSync(process.argv[1],'utf8')); process.stdout.write(b.job_id||'')}catch{process.stdout.write('')}" "${ROOT}/docs/.proof-response.json")"
  body_file="$(mktemp)"
  local picked_file="${ROOT}/docs/.proof-job.json"
  printf '%s' '{"done":false}' > "${picked_file}"
  local deadline=$(( $(now_ms) + 180000 ))
  while [[ "$(now_ms)" -lt "${deadline}" ]]; do
    curl -sS -m 20 "${ORCH}/api/jobs?limit=60" > "${body_file}" || printf '[]' > "${body_file}"
    pick_job "${started}" "${want_id}" "${body_file}" > "${picked_file}"
    if [[ "$(node -e "const fs=require('fs'); process.stdout.write(JSON.parse(fs.readFileSync(process.argv[1],'utf8')).done?'yes':'no')" "${picked_file}")" == "yes" ]]; then
      break
    fi
    sleep 2
  done
  rm -f "${body_file}"
  elapsed=$(( $(now_ms) - started ))
  read_field() { node -e "const fs=require('fs'); const j=JSON.parse(fs.readFileSync(process.argv[1],'utf8')); const v=j[process.argv[2]]; process.stdout.write(v==null||v===''?process.argv[3]:String(v))" "${picked_file}" "$1" "$2"; }
  task_id="$(read_field task_id 'not returned')"
  lock_tx="$(read_field lock_tx_hash 'not returned')"
  claim_tx="$(read_field claim_tx_hash 'not returned')"
  refund_tx="$(read_field refund_tx_hash 'not returned')"
  result_hash="$(read_field result_hash 'not returned')"
  amount="$(format_units "$(read_field amount '')")"
  actual="$(format_units "$(read_field actual_amount '')")"
  if [[ "${actual}" == "not returned" ]]; then
    actual="$(format_units "$(read_field amount '')")"
  fi
  refunded="$(format_units "$(read_field refunded_change '')")"
  status="$(read_field status 'not returned')"
  error="$(read_field error '')"
  rm -f "${picked_file}" "${ROOT}/docs/.proof-response.json"
  local lock_state claim_state refund_state
  lock_state="$(confirm_tx "${lock_tx}")" || FAIL=1
  claim_state="$(confirm_tx "${claim_tx}")" || true
  refund_state="$(confirm_tx "${refund_tx}")" || true
  if [[ "${lock_state}" != confirmed && "${claim_state}" != confirmed && "${refund_state}" != confirmed ]]; then
    FAIL=1
  fi
  local when
  when="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  ROWS="${ROWS}| ${name} | ${scheme} | ${amount} | ${actual} | ${refunded} | ${status} | ${elapsed} ms | ${when} | ${task_id} | $(link_for "${lock_tx}" "lock") (${lock_state}) | $(link_for "${claim_tx}" "claim") (${claim_state}) | $(link_for "${refund_tx}" "refund") (${refund_state}) | ${result_hash} | ${error} |
"
}

{
  echo "# Verified on-chain proofs"
  echo
  echo "Generated ${STAMP} from \`${ORCH}\`."
  echo
  echo "This file lists only values returned by the orchestrator. A row is confirmed when \`GET ${EXPLORER}/api/v2/transactions/{hash}\` returns HTTP 200. Missing hashes are left as \`not captured\`."
  echo
  echo "- NEXUS_ESCROW_ADDRESS: ${ESCROW:-unset}"
  echo "- NEXUS_IDENTITY_REGISTRY_ADDRESS: ${IDENTITY:-unset}"
  echo
} > "${OUT}"

if ! curl -sf -m 8 "${ORCH}/health" >/dev/null; then
  cat >> "${OUT}" <<EOF
## Result

The orchestrator at \`${ORCH}\` did not answer \`/health\`. No scenario was dispatched and no Aevum transaction hash was recorded.

| Scenario | Scheme | Locked tUSD8 | Settled tUSD8 | Refunded tUSD8 | Status | Latency | Timestamp (UTC) | taskId | Lock | Claim | Refund | resultHash | Error |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| happy_path | exact | not captured | not captured | not captured | not captured | not captured | ${STAMP} | not captured | not captured | not captured | not captured | not captured | orchestrator offline |
| metered_batch | upto | not captured | not captured | not captured | not captured | not captured | ${STAMP} | not captured | not captured | not captured | not captured | not captured | orchestrator offline |
| voucher_burst | batch-settlement | not captured | not captured | not captured | not captured | not captured | ${STAMP} | not captured | not captured | not captured | not captured | not captured | orchestrator offline |
| worker_timeout_refund | exact | not captured | not captured | not captured | not captured | not captured | ${STAMP} | not captured | not captured | not captured | not captured | not captured | orchestrator offline |
| evaluator_dispute_slash | auth-capture | not captured | not captured | not captured | not captured | not captured | ${STAMP} | not captured | not captured | not captured | not captured | not captured | orchestrator offline |
| multi_hop_swarm | exact | not captured | not captured | not captured | not captured | not captured | ${STAMP} | not captured | not captured | not captured | not captured | not captured | orchestrator offline |
| velocity_attack | exact | not captured | not captured | not captured | not captured | not captured | ${STAMP} | not captured | not captured | not captured | not captured | not captured | orchestrator offline |
EOF
  printf 'proof capture failed: orchestrator health check failed\n' >&2
  exit 1
fi

capture happy_path exact
capture metered_batch upto
capture voucher_burst batch-settlement
capture worker_timeout_refund exact
capture evaluator_dispute_slash auth-capture
capture multi_hop_swarm exact
capture velocity_attack exact

{
  echo "## Scenarios"
  echo
  echo "| Scenario | Scheme | Locked tUSD8 | Settled tUSD8 | Refunded tUSD8 | Status | Latency | Timestamp (UTC) | taskId | Lock | Claim | Refund | resultHash | Error |"
  echo "|---|---|---|---|---|---|---|---|---|---|---|---|---|---|"
  printf '%s' "${ROWS}"
} >> "${OUT}"

if [[ "${FAIL}" -ne 0 ]]; then
  echo >> "${OUT}"
  echo "Capture finished with at least one scenario that has no explorer-confirmed transaction." >> "${OUT}"
  printf 'proof capture incomplete: see %s\n' "${OUT}" >&2
  exit 1
fi

printf 'proof capture wrote %s\n' "${OUT}"
exit 0
