# Local development only.

MIGRATIONS := $(abspath ../../db/postgres/migrations)
FLYWAY_IMAGE := flyway/flyway:11.20.0
PSQL := docker compose exec -T postgres psql -U system1 -v ON_ERROR_STOP=1 -q

FLYWAY = docker run --rm \
	--network "$$(docker inspect -f '{{range $$k, $$v := .NetworkSettings.Networks}}{{$$k}}{{end}}' $$(docker compose ps -q postgres))" \
	-v "$(MIGRATIONS):/flyway/sql:ro" $(FLYWAY_IMAGE) \
	-url=jdbc:postgresql://postgres:5432/system1 -user=system1 -password=system1 \
	-locations=filesystem:/flyway/sql

.DEFAULT_GOAL := help
.PHONY: help gen run run-dialogs run-gateway check vet lint test test-integration db-up db-down migrate info validate seed db-reset psql

GO_VERSION := $(shell sed -n 's/^go //p' go.mod)
GOLANGCI_LINT := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0
ENV = set -a && . ./local.env && if [ -f .env ]; then . ./.env; fi && set +a &&

help: ## Show targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  %-17s %s\n", $$1, $$2}'

gen: ## Generate API and sqlc code
	cd ../.. && python3 api/build.py --langs go
	go tool sqlc generate

run: ## Migrate and run all services
	$(MAKE) -j2 run-dialogs run-gateway

run-dialogs: migrate ## Migrate and run s1-dialogs on port 8080
	$(ENV) PORT=8080 go run ./s1-dialogs

run-gateway: migrate ## Migrate and run s1-channels-gateway on port 8081
	$(ENV) PORT=8081 go run ./s1-channels-gateway

check: vet lint test-integration ## Run all checks before commit

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint
	GOTOOLCHAIN=go$(GO_VERSION) go run $(GOLANGCI_LINT) run ./...

test: ## Run unit tests
	go test ./...

test-integration: ## Run unit and integration tests, needs Docker
	go test -tags integration ./...

db-up: ## Start local Postgres
	docker compose up -d --wait

db-down: ## Stop local Postgres, keep data
	docker compose stop

migrate: db-up ## Apply pending migrations
	$(FLYWAY) migrate

info: db-up ## Show migration status
	$(FLYWAY) info

validate: db-up ## Check applied migrations were not edited
	$(FLYWAY) validate

seed: migrate ## Insert local test data
	$(PSQL) -d system1 < seed.sql

db-reset: db-up ## Recreate the database, migrate and seed
	$(PSQL) -d postgres -c "DROP DATABASE IF EXISTS system1 WITH (FORCE)" -c "CREATE DATABASE system1"
	$(MAKE) seed

psql: db-up ## Open psql on the local database
	docker compose exec postgres psql -U system1 -d system1
