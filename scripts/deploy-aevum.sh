#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ROOT}/.env"
if [[ ! -f "${ENV_FILE}" ]]; then
  echo "Copy .env.example to .env and set PRIVATE_KEY before deploying."
  exit 1
fi

set -a
# shellcheck disable=SC1091
source "${ENV_FILE}"
set +a

KEY="${PRIVATE_KEY:-${PRINCIPAL_PRIVATE_KEY:-}}"
if [[ -z "${KEY}" ]]; then
  echo "Set PRIVATE_KEY or PRINCIPAL_PRIVATE_KEY in .env. No contract was broadcast."
  exit 1
fi

cd "${ROOT}/contracts"
log="$(mktemp)"
set +e
PRIVATE_KEY="${KEY}" TUSD8_ADDRESS="${TUSD8_ADDRESS:-0x456569838f1d43009A91ebA4263147Afd7233483}" \
  forge script script/DeployAevum.s.sol:DeployAevum \
    --rpc-url "${AEVUM_RPC_URL:-https://rpc-test.qday.info}" \
    --broadcast \
    --legacy \
    -vvvv | tee "${log}"
status="${PIPESTATUS[0]}"
set -e
if [[ "${status}" -ne 0 ]]; then
  rm -f "${log}"
  exit "${status}"
fi

address="$(grep -E 'NexusEscrow' "${log}" | grep -oE '0x[a-fA-F0-9]{40}' | tail -n 1 || true)"
rm -f "${log}"
if [[ -z "${address}" ]]; then
  echo "Broadcast finished without a NexusEscrow address in the log. .env was not changed."
  exit 1
fi

python - "${ENV_FILE}" "${address}" <<'PY'
import sys
path, address = sys.argv[1], sys.argv[2]
keys = {
    "NEXUS_ESCROW_ADDRESS": address,
    "NEXT_PUBLIC_NEXUS_ESCROW_ADDRESS": address,
}
lines = open(path, encoding="utf-8").read().splitlines()
seen = set()
out = []
for line in lines:
    replaced = False
    for key, value in keys.items():
        if line.startswith(key + "=") or line.startswith(key + '="'):
            out.append(f'{key}="{value}"')
            seen.add(key)
            replaced = True
            break
    if not replaced:
        out.append(line)
for key, value in keys.items():
    if key not in seen:
        out.append(f'{key}="{value}"')
open(path, "w", encoding="utf-8", newline="\n").write("\n".join(out) + "\n")
PY

echo "NexusEscrow ${address}"
echo "Updated ${ENV_FILE}"
