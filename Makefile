.DEFAULT_GOAL := help
GO ?= go
PYTHON ?= .venv/bin/python
EXPORT_PYTHON ?= $(abspath .venv/bin/python)
export EXPORT_PYTHON
# Resolve before recipes cd into backend, so API and worker share one store.
FILE_STORAGE_DIR ?= $(abspath var/private-files)
export FILE_STORAGE_DIR

.PHONY: help setup-tools check check-contract test-contract check-go test-integration test-migrations-local migrate-up migrate-status run-api run-worker
help:
	@printf '%s\n' 'setup-tools            Create Python validation environment' 'check                  Validate contract, tests, Go formatting/vet/build' 'test-migrations-local  Test using a disposable local PostgreSQL cluster' 'test-integration       Test against TEST_DATABASE_URL (name ends in _test)' 'migrate-up             Apply embedded migrations to DATABASE_URL' 'migrate-status         Show migration status' 'run-api                Run HTTP API (configured environment required)' 'run-worker             Run mock generation and document export Worker'

setup-tools:
	python3 -m venv .venv
	$(PYTHON) -m pip install -r scripts/requirements.lock -r scripts/export-requirements.lock

check: check-contract test-contract check-go

check-contract:
	$(PYTHON) scripts/check_contract.py

test-contract:
	$(PYTHON) -m unittest discover -s tests/contract -v

check-go:
	@test -z "$$(find backend -name '*.go' -exec gofmt -l {} +)" || (echo 'Run gofmt on Go files'; exit 1)
	cd backend && $(GO) vet ./...
	cd backend && $(GO) test ./cmd/... ./migrations/... ./internal/...
	@mkdir -p bin
	cd backend && $(GO) build -o ../bin/migrate ./cmd/migrate
	cd backend && $(GO) build -o ../bin/api ./cmd/api
	cd backend && $(GO) build -o ../bin/worker ./cmd/worker

test-integration:
	@test -n "$$TEST_DATABASE_URL" || (echo 'Set TEST_DATABASE_URL (database name must end in _test)'; exit 1)
	cd backend && $(GO) test -count=1 -v ./tests/integration

test-migrations-local:
	$(PYTHON) scripts/test_migrations_local.py

migrate-up:
	cd backend && $(GO) run ./cmd/migrate up

migrate-status:
	cd backend && $(GO) run ./cmd/migrate status

run-api:
	cd backend && $(GO) run ./cmd/api

run-worker:
	cd backend && $(GO) run ./cmd/worker

.PHONY: run-frontend check-frontend generate-frontend-types
run-frontend:
	cd frontend && npm run dev

check-frontend:
	cd frontend && npm run format:check && npm test && npm run build

generate-frontend-types:
	$(PYTHON) scripts/generate_frontend_types.py
