#!/usr/bin/env bash
# Exit 0 only when submission addresses, live links, and tracked text are consistent.
set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -f "${ROOT}/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "${ROOT}/.env"
  set +a
fi

FAIL=0
pass() { printf 'PASS  %s\n' "$1"; }
fail() { printf 'FAIL  %s\n' "$1"; FAIL=1; }

ESCROW="${NEXUS_ESCROW_ADDRESS:-}"
IDENTITY="${NEXUS_IDENTITY_REGISTRY_ADDRESS:-${NEXUS_IDENTITY_ADDRESS:-}}"
ORCH="${RENDER_ORCHESTRATOR_URL:-${ORCHESTRATOR_URL:-http://127.0.0.1:8081}}"
ORCH="${ORCH%/}"

is_addr() {
  [[ "$1" =~ ^0x[0-9a-fA-F]{40}$ ]]
}

DOCS=(
  "${ROOT}/README.md"
  "${ROOT}/docs/SUBMISSION.md"
  "${ROOT}/docs/VERIFIED_ONCHAIN_PROOFS.md"
)

check_address() {
  local label="$1"
  local value="$2"
  if ! is_addr "${value}"; then
    fail "${label} is unset or not a 20-byte address"
    return
  fi
  local doc
  for doc in "${DOCS[@]}"; do
    if [[ ! -f "${doc}" ]]; then
      fail "${label} missing file ${doc#"${ROOT}/"}"
      continue
    fi
    if grep -q -F "${value}" "${doc}"; then
      pass "${label} appears in ${doc#"${ROOT}/"}"
    else
      fail "${label} is missing from ${doc#"${ROOT}/"}"
    fi
  done
  local body code
  body="$(mktemp)"
  code="$(curl -sS -m 20 -o "${body}" -w '%{http_code}' "${ORCH}/api/telemetry" || true)"
  if [[ "${code}" != "200" ]]; then
    fail "${label} telemetry returned HTTP ${code:-none}"
  elif grep -q -F "${value}" "${body}"; then
    pass "${label} matches ${ORCH}/api/telemetry"
  else
    fail "${label} is missing from ${ORCH}/api/telemetry"
  fi
  rm -f "${body}"
}

check_address "NEXUS_ESCROW_ADDRESS" "${ESCROW}"
check_address "NEXUS_IDENTITY_REGISTRY_ADDRESS" "${IDENTITY}"

url_file="$(mktemp)"
grep -h -oE 'https://[^[:space:]")<>]+' "${ROOT}/README.md" "${ROOT}/docs/SUBMISSION.md" \
  | sed 's/[.,;:]*$//' \
  | sort -u > "${url_file}"

if [[ ! -s "${url_file}" ]]; then
  fail "no https URLs found in README.md or docs/SUBMISSION.md"
else
  while IFS= read -r url; do
    code="$(curl -sS -L -m 25 -o /dev/null -w '%{http_code}' "${url}" || true)"
    if [[ "${code}" == "200" ]]; then
      pass "HTTP 200 ${url}"
    else
      fail "HTTP ${code:-none} ${url}"
    fi
  done < "${url_file}"
fi
rm -f "${url_file}"

marker='TO''DO|FIX''ME|\[INSERT_'
key_re='(PRIVATE_KEY|private_key|MNEMONIC|mnemonic)[[:space:]]*[:=][[:space:]]*.{0,3}[0-9a-fA-F]{64}'

scan_file="$(mktemp)"
git -C "${ROOT}" ls-files -z \
  | tr '\0' '\n' \
  | grep -v -E '^(contracts/lib/|node_modules/|frontend/node_modules/)' \
  > "${scan_file}"

while IFS= read -r rel; do
  [[ -f "${ROOT}/${rel}" ]] || continue
  if grep -n -E "${marker}" "${ROOT}/${rel}" >/dev/null; then
    fail "placeholder marker in ${rel}"
  fi
  if grep -n -E "${key_re}" "${ROOT}/${rel}" >/dev/null; then
    fail "unredacted private key in ${rel}"
  fi
done < "${scan_file}"
rm -f "${scan_file}"

if git -C "${ROOT}" ls-files --error-unmatch .env >/dev/null 2>&1; then
  fail ".env is tracked"
else
  pass ".env is not tracked"
fi

if [[ "${FAIL}" -ne 0 ]]; then
  printf 'submission audit failed\n'
  exit 1
fi
printf 'submission audit passed\n'
exit 0
