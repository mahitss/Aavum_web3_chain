# Release notes

## 24 Nov 2026 — submission package and charter set

- README, architecture, demo script, and DoraHacks form describe the code and leave the contract address empty.
- Foundry: 8 passed, 0 failed.
- Docker images for all three services built. Compose did not stay healthy with an empty worker key.
- Command center can submit `refundExpiredTask` for the newest unsettled task id.
- Engineering index under `docs/` covers the charter domains that apply. Domains that do not apply are marked out of scope instead of given fake designs.
- CI workflow added. GitHub execution of that workflow is **UNVERIFIED** until the next push is observed.

## Not in this release

A mined lock, a mined claim, a Vercel deployment, and a video.
