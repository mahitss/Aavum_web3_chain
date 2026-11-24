.PHONY: setup test-contracts deploy-aevum run-worker run-orchestrator run-frontend e2e-test docker-up

setup:
	cd contracts && forge install || true
	cd agent-orchestrator && go mod tidy
	cd frontend && npm install

test-contracts:
	cd contracts && forge test -vvv --gas-report

deploy-aevum:
	bash scripts/deploy-aevum.sh

run-worker:
	cd agent-worker && cargo run --release

run-orchestrator:
	cd agent-orchestrator && go run ./cmd

run-frontend:
	cd frontend && npm run dev

e2e-test:
	bash scripts/verify-e2e.sh

docker-up:
	@test -f .env || cp .env.example .env
	docker compose up --build -d
