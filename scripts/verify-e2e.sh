#!/usr/bin/env bash
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${CYAN}================================================================${NC}"
echo -e "${CYAN}     NEXUS402: END-TO-END VERIFICATION & SETTLEMENT HARNESS${NC}"
echo -e "${CYAN}================================================================${NC}"

WORKER_URL="${WORKER_URL:-http://localhost:8080}"
ORCHESTRATOR_URL="${ORCHESTRATOR_URL:-http://localhost:8081}"
EXPLORER="${AEVUM_EXPLORER_URL:-https://explorer-test.qday.info}"

echo -e "\n${YELLOW}[1/4] Verifying Service Healthchecks...${NC}"
WORKER_HEALTH=$(curl -sf "${WORKER_URL}/health" || echo "FAIL")
if [[ "${WORKER_HEALTH}" == *"healthy"* ]]; then
  echo -e "${GREEN}✓ Rust Worker is healthy (Port 8080)${NC}"
else
  echo -e "${RED}✗ Rust Worker failed healthcheck${NC}"
  exit 1
fi

ORCH_HEALTH=$(curl -sf "${ORCHESTRATOR_URL}/health" || echo "FAIL")
if [[ "${ORCH_HEALTH}" == *"healthy"* ]]; then
  echo -e "${GREEN}✓ Go Orchestrator is healthy (Port 8081)${NC}"
else
  echo -e "${RED}✗ Go Orchestrator failed healthcheck${NC}"
  exit 1
fi

echo -e "\n${YELLOW}[2/4] Testing Direct HTTP 402 Paywall Interception...${NC}"
PAYLOAD='{"prompt":"Verify hash computation","task_type":"hash"}'
HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${WORKER_URL}/api/v1/compute" \
  -H "Content-Type: application/json" \
  -d "${PAYLOAD}")
if [[ "${HTTP_STATUS}" -eq 402 ]]; then
  echo -e "${GREEN}✓ Worker successfully challenged unpaid request with HTTP 402 Payment Required${NC}"
else
  echo -e "${RED}✗ Expected HTTP 402, received: ${HTTP_STATUS}${NC}"
  exit 1
fi

echo -e "\n${YELLOW}[3/4] Dispatching Autonomous Micro-Task to Go Orchestrator...${NC}"
DISPATCH_RESP=$(curl -sf -X POST "${ORCHESTRATOR_URL}/api/jobs" \
  -H "Content-Type: application/json" \
  -d "${PAYLOAD}")
JOB_ID=$(printf '%s' "${DISPATCH_RESP}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
if [[ -z "${JOB_ID}" ]]; then
  echo -e "${RED}✗ Orchestrator did not accept the job${NC}"
  echo "${DISPATCH_RESP}"
  exit 1
fi
echo -e "Enqueued Job ID: ${CYAN}${JOB_ID}${NC}"
echo "Awaiting autonomous on-chain escrow lock and worker settlement..."

SETTLED=false
CURRENT_STATUS=""
for i in $(seq 1 40); do
  sleep 3
  STATUS_RESP=$(curl -sf "${ORCHESTRATOR_URL}/api/jobs")
  CURRENT_STATUS=$(printf '%s' "${STATUS_RESP}" | tr '{' '\n' | grep "${JOB_ID}" | head -n 1 || true)
  if [[ "${CURRENT_STATUS}" == *'"status":"FAILED"'* ]]; then
    echo -e "${RED}✗ Task execution failed on-chain${NC}"
    echo "${CURRENT_STATUS}"
    exit 1
  fi
  if [[ "${CURRENT_STATUS}" == *'"status":"SETTLED"'* ]]; then
    SETTLED=true
    break
  fi
  echo "Polling settlement status... (Attempt ${i}/40)"
done

if [[ "${SETTLED}" == true ]]; then
  echo -e "${GREEN}✓ Task successfully settled with zero human intervention!${NC}"
else
  echo -e "${RED}✗ Settlement timed out${NC}"
  exit 1
fi

echo -e "\n${YELLOW}[4/4] Verifiable Testnet Artifacts:${NC}"
LOCK_TX=$(printf '%s' "${CURRENT_STATUS}" | sed -n 's/.*"lock_tx_hash":"\([^"]*\)".*/\1/p')
CLAIM_TX=$(printf '%s' "${CURRENT_STATUS}" | sed -n 's/.*"claim_tx_hash":"\([^"]*\)".*/\1/p')
TASK_ID=$(printf '%s' "${CURRENT_STATUS}" | sed -n 's/.*"task_id":"\([^"]*\)".*/\1/p')
LOCK_TX="${LOCK_TX:-N/A}"
CLAIM_TX="${CLAIM_TX:-N/A}"
TASK_ID="${TASK_ID:-N/A}"

echo -e "Task ID:        ${CYAN}${TASK_ID}${NC}"
echo -e "Lock Escrow Tx: ${CYAN}${EXPLORER}/tx/${LOCK_TX}${NC}"
echo -e "Claim Proof Tx: ${CYAN}${EXPLORER}/tx/${CLAIM_TX}${NC}"

echo -e "\n${GREEN}================================================================${NC}"
echo -e "${GREEN}   ALL SYSTEMS OPERATIONAL: M2M 402 PROTOCOL FULLY VERIFIED${NC}"
echo -e "${GREEN}================================================================${NC}"
