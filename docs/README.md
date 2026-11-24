# Nexus402 engineering index

This index is the charter review for the product that already exists in this repository. Status words mean implementation state. They do not mean a third-party certification or a live Aevum deployment.

| Domain | Scope | Status | Where it lives |
|---|---|---|---|
| 01 Product | In scope | Partial | [01-product/PRODUCT_REQUIREMENTS.md](01-product/PRODUCT_REQUIREMENTS.md) |
| 02 Requirements | In scope | Implemented in code | [02-requirements/FUNCTIONAL_SPECIFICATION.md](02-requirements/FUNCTIONAL_SPECIFICATION.md) |
| 03 UI/UX | In scope | Implemented | [04-design/UI_SPECIFICATION.md](04-design/UI_SPECIFICATION.md) |
| 04 Frontend | In scope | Implemented | `frontend/` |
| 05 Mobile and desktop | Out of scope | Not started | No native client |
| 06 Backend | In scope | Implemented | `agent-worker/`, `agent-orchestrator/` |
| 07 API | In scope | Implemented | [09-api/API_SPECIFICATION.md](09-api/API_SPECIFICATION.md) |
| 08 Database | Explicitly unused | Not started | [08-database/DATABASE_DESIGN.md](08-database/DATABASE_DESIGN.md) |
| 09 Analytics warehouse | Out of scope | Not started | Counters only, `GET /api/telemetry` |
| 10 AI and ML | Out of scope | Not started | Worker hashes a string. No model call |
| 11 Distributed systems | In scope, single host | Implemented | [05-architecture/SYSTEM_ARCHITECTURE.md](05-architecture/SYSTEM_ARCHITECTURE.md) |
| 12 Security | In scope | Reviewed in-repo | [SECURITY_AUDIT.md](SECURITY_AUDIT.md), [10-security/THREAT_MODEL.md](10-security/THREAT_MODEL.md) |
| 13 Cloud accounts | Out of scope | Not started | No AWS, GCP, or Azure resources |
| 14 DevOps | In scope | Implemented | [14-devops/DEPLOYMENT_RUNBOOK.md](14-devops/DEPLOYMENT_RUNBOOK.md), `.github/workflows/ci.yml` |
| 15 Reliability | In scope | Partial | [18-reliability/DISASTER_RECOVERY.md](18-reliability/DISASTER_RECOVERY.md) |
| 16 Performance | In scope | Not measured on Aevum | `scripts/mesh-bench.sh` is **UNVERIFIED** live |
| 17 Quality | In scope | Contract tests verified | [15-testing/TEST_STRATEGY.md](15-testing/TEST_STRATEGY.md) |
| 18 Observability | In scope | Partial | [17-observability/OBSERVABILITY_PLAN.md](17-observability/OBSERVABILITY_PLAN.md) |
| 19 Compliance certification | Out of scope | Not claimed | Testnet demo. No SOC 2, ISO, or GDPR program |
| 20 Trust controls | In scope | Implemented in the contract | Hourly cap, per-task cap, revoke |
| 21 Card billing | Out of scope | Not started | Settlement is tUSD8 escrow |
| 22 Enterprise SSO | Out of scope | Not started | Wallet plus EIP-712 session only |
| 23 Release | In scope | Partial | [22-progress/RELEASE_NOTES.md](22-progress/RELEASE_NOTES.md) |
| 24 Documentation | In scope | This set | Protocol detail stays in [ARCHITECTURE.md](ARCHITECTURE.md) |
| 25 Governance | In scope | This review | [21-governance/RISK_REGISTER.md](21-governance/RISK_REGISTER.md) |

Public source: https://github.com/mahitss/Aavum_web3_chain
