# Deployment runbook

## Local stack

```bash
cp .env.example .env
# Fill keys and NEXUS_ESCROW_ADDRESS. Empty hex crashes the worker.
docker compose up --build -d
curl -s http://localhost:8080/health
curl -s http://localhost:8081/health
```

If the worker logs `HexError(InvalidStringLength)`, the key or the escrow address is empty or not a hex string. `docker compose down` and fix `.env`. Do not leave the restart loop running.

## Contract

```bash
# PRIVATE_KEY or PRINCIPAL_PRIVATE_KEY must be a tQDAY-funded testnet key.
make deploy-aevum
```

The script runs `forge script script/DeployAevum.s.sol:DeployAevum --rpc-url https://rpc-test.qday.info --broadcast --legacy -vvvv` and writes the logged address into `.env`.

Source verification, after that address exists:

```bash
forge verify-contract <ADDRESS> src/NexusEscrow.sol:NexusEscrow \
  --chain-id 44005 \
  --verifier-url https://explorer-test.qday.info/api \
  --verifier blockscout \
  --constructor-args $(cast abi-encode "constructor(address)" 0x456569838f1d43009A91ebA4263147Afd7233483)
```

This verification command has not been run. Status: **UNVERIFIED**.

## Rollback

The contract is not upgradeable. Rollback of a bad deploy is to stop using that address and deploy a new one. Tokens already in an old escrow stay there until claim or refund. There is no admin withdraw.

Application rollback is the previous git commit and `docker compose up --build -d`.

## Not in this runbook

Vercel, a custom domain, and mainnet. Mainnet keys must not be placed in `.env`.
