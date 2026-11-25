#!/usr/bin/env bash
# Live judge preflight. Exits 0 only when every check below is true.
set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -f "${ROOT}/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "${ROOT}/.env"
  set +a
fi

RPC="${AEVUM_RPC_URL:-https://rpc-test.qday.info}"
TUSD8="${TUSD8_ADDRESS:-0x456569838f1d43009A91ebA4263147Afd7233483}"
MIN_TUSD8=20000000
FAIL=0

if command -v cast >/dev/null 2>&1; then
  CAST=(cast)
elif [[ -x "${HOME}/.foundry/bin/cast" ]]; then
  CAST=("${HOME}/.foundry/bin/cast")
elif [[ -x "${USERPROFILE}/.foundry/bin/cast.exe" ]]; then
  CAST=("${USERPROFILE}/.foundry/bin/cast.exe")
else
  CAST=()
fi

pass() { printf 'PASS  %s\n' "$1"; }
fail() { printf 'FAIL  %s\n' "$1"; FAIL=1; }

need() {
  if [[ -z "${2:-}" ]]; then
    fail "$1 is unset"
    return 1
  fi
  return 0
}

first_word() { printf '%s\n' "$1" | awk 'NR==1 { print $1 }'; }

at_least() {
  node -e "process.exit(BigInt(process.argv[1]) >= BigInt(process.argv[2]) ? 0 : 1)" "$1" "$2"
}

if [[ ${#CAST[@]} -eq 0 ]]; then
  fail "cast is not installed"
else
  if need "NEXUS_ESCROW_ADDRESS" "${NEXUS_ESCROW_ADDRESS:-}"; then
    code="$("${CAST[@]}" code "${NEXUS_ESCROW_ADDRESS}" --rpc-url "${RPC}" 2>/dev/null || true)"
    if [[ -n "${code}" && "${code}" != "0x" ]]; then
      pass "NexusEscrow bytecode is deployed"
    else
      fail "NexusEscrow has no bytecode at ${NEXUS_ESCROW_ADDRESS:-unset}"
    fi
  fi

  if need "PRINCIPAL_ADDRESS" "${PRINCIPAL_ADDRESS:-}" && [[ -n "${NEXUS_ESCROW_ADDRESS:-}" ]]; then
    balance="$(first_word "$("${CAST[@]}" call "${TUSD8}" "balanceOf(address)(uint256)" "${PRINCIPAL_ADDRESS}" --rpc-url "${RPC}" 2>/dev/null || true)")"
    if [[ -n "${balance}" ]] && at_least "${balance}" "${MIN_TUSD8}"; then
      pass "principal holds at least 20 tUSD8"
    else
      fail "principal tUSD8 balance is ${balance:-unread}; need at least 20 tUSD8"
    fi
    allowance="$(first_word "$("${CAST[@]}" call "${TUSD8}" "allowance(address,address)(uint256)" "${PRINCIPAL_ADDRESS}" "${NEXUS_ESCROW_ADDRESS}" --rpc-url "${RPC}" 2>/dev/null || true)")"
    if [[ -n "${allowance}" ]] && at_least "${allowance}" "${MIN_TUSD8}"; then
      pass "principal allowance covers at least 20 tUSD8"
    else
      fail "tUSD8 allowance for NexusEscrow is ${allowance:-unread}"
    fi
  fi

  gas_price="$(first_word "$("${CAST[@]}" gas-price --rpc-url "${RPC}" 2>/dev/null || true)")"
  if [[ -z "${gas_price}" ]]; then
    fail "could not read the Aevum gas price"
  else
    minimum="$(node -e "console.log((BigInt(process.argv[1]) * 250000n * 50n).toString())" "${gas_price}")"
    for label in ORCHESTRATOR_AGENT_ADDRESS WORKER_ADDRESS; do
      value="${!label:-}"
      if [[ -z "${value}" ]]; then
        derived=""
        if [[ "${label}" == "ORCHESTRATOR_AGENT_ADDRESS" && -n "${ORCHESTRATOR_AGENT_PRIVATE_KEY:-}" ]]; then
          derived="$("${CAST[@]}" wallet address --private-key "${ORCHESTRATOR_AGENT_PRIVATE_KEY}" 2>/dev/null || true)"
        fi
        if [[ "${label}" == "WORKER_ADDRESS" && -n "${WORKER_PRIVATE_KEY:-}" ]]; then
          derived="$("${CAST[@]}" wallet address --private-key "${WORKER_PRIVATE_KEY}" 2>/dev/null || true)"
        fi
        value="${derived}"
      fi
      if [[ -z "${value}" ]]; then
        fail "${label} is unset"
        continue
      fi
      held="$(first_word "$("${CAST[@]}" balance "${value}" --rpc-url "${RPC}" 2>/dev/null || true)")"
      if [[ -n "${held}" ]] && at_least "${held}" "${minimum}"; then
        pass "${label} can fund 50 transactions at the current gas price"
      else
        fail "${label} tQDAY balance is ${held:-unread}; need at least ${minimum} wei"
      fi
    done
  fi

  agent="${ORCHESTRATOR_AGENT_ADDRESS:-}"
  if [[ -z "${agent}" && -n "${ORCHESTRATOR_AGENT_PRIVATE_KEY:-}" && ${#CAST[@]} -gt 0 ]]; then
    agent="$("${CAST[@]}" wallet address --private-key "${ORCHESTRATOR_AGENT_PRIVATE_KEY}" 2>/dev/null || true)"
  fi
  if [[ -n "${NEXUS_ESCROW_ADDRESS:-}" && -n "${PRINCIPAL_ADDRESS:-}" && -n "${agent}" ]]; then
    session="$("${CAST[@]}" call "${NEXUS_ESCROW_ADDRESS}" "getSession(address,address)(address,uint256,uint256,uint256,uint256,uint256,bool)" "${PRINCIPAL_ADDRESS}" "${agent}" --rpc-url "${RPC}" 2>/dev/null || true)"
    expires="$(printf '%s\n' "${session}" | sed -n '4p' | awk '{print $1}')"
    active="$(printf '%s\n' "${session}" | awk 'NF { line=$1 } END { print line }')"
    now="$(date +%s)"
    horizon="$((now + 14 * 24 * 3600))"
    if [[ "${active}" == "true" && -n "${expires}" ]] && at_least "${expires}" "${horizon}"; then
      pass "sandbox session is active for at least 14 days"
    else
      fail "session active=${active:-unread} expiresAt=${expires:-unread}"
    fi
  else
    fail "session check needs the escrow, principal, and orchestrator agent"
  fi
fi

check_url() {
  local name="$1"
  local url="$2"
  if [[ -z "${url}" ]]; then
    fail "${name} URL is unset"
    return
  fi
  local status
  status="$(curl -sS -o /dev/null -w '%{http_code}' -m 20 "${url}" || true)"
  if [[ "${status}" == "200" ]]; then
    pass "${name} returned HTTP 200"
  else
    fail "${name} returned HTTP ${status:-none} (${url})"
  fi
}

check_url "worker health" "${RENDER_WORKER_URL:+${RENDER_WORKER_URL%/}/health}"
check_url "worker catalog" "${RENDER_WORKER_URL:+${RENDER_WORKER_URL%/}/.well-known/nexus402.json}"
check_url "orchestrator health" "${RENDER_ORCHESTRATOR_URL:+${RENDER_ORCHESTRATOR_URL%/}/health}"
check_url "sandbox status" "${RENDER_ORCHESTRATOR_URL:+${RENDER_ORCHESTRATOR_URL%/}/api/sandbox/status}"
check_url "Vercel frontend" "${VERCEL_FRONTEND_URL:-}"

if [[ "${FAIL}" -ne 0 ]]; then
  printf 'preflight failed\n'
  exit 1
fi
printf 'preflight passed\n'
exit 0
