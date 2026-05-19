-include .env
export

.PHONY: help run-server run-worker build tidy test lint up down logs ps migrate-up migrate-down

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

run-server: ## Run HTTP server
	go run ./cmd/server

run-worker: ## Run worker
	go run ./cmd/worker

build: ## Build server and worker binaries into bin/
	go build -o bin/server ./cmd/server
	go build -o bin/worker ./cmd/worker

tidy: ## Run go mod tidy
	go mod tidy

test: ## Run tests
	go test ./...

lint: ## Run linter (placeholder until golangci-lint is configured)
	go vet ./...

mocks: ## Regenerate test mocks via mockery
	mockery

up: ## Start docker stack (postgres + minio + rabbitmq)
	docker compose up -d

down: ## Stop docker stack
	docker compose down

logs: ## Tail docker stack logs
	docker compose logs -f

ps: ## Show docker stack status
	docker compose ps

migrate-up: ## Apply DB migrations
	migrate -path migrations -database "$(POSTGRES_DSN)" up

migrate-down: ## Rollback last DB migration
	migrate -path migrations -database "$(POSTGRES_DSN)" down 1
