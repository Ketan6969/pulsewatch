DB_URL ?= postgres://pulsewatch:pulsewatch@localhost:5432/pulsewatch?sslmode=disable
TEST_DB_URL ?= postgres://pulsewatch:pulsewatch@localhost:5432/pulsewatch_test?sslmode=disable
DEV_TOKEN ?= dev-agent-token-0123456789

.PHONY: build test test-go test-web lint run agent web-dev web-build up down dev-db

build: web-build
	go build -o bin/ ./cmd/...

test: test-go test-web

## Go unit + integration tests (integration tests need Postgres; see dev-db).
test-go:
	PULSEWATCH_TEST_DATABASE_URL="$(TEST_DB_URL)" go test -race -p 1 ./...

test-web:
	cd web && npm test && npm run typecheck

lint:
	go vet ./...
	test -z "$$(gofmt -l .)"

run:
	PULSEWATCH_DATABASE_URL="$(DB_URL)" PULSEWATCH_AGENT_TOKEN=$(DEV_TOKEN) PULSEWATCH_LOG_FORMAT=text \
	PULSEWATCH_CORS_ORIGIN=http://localhost:5173 go run ./cmd/pulsewatch

agent:
	PULSEWATCH_AGENT_TOKEN=$(DEV_TOKEN) PULSEWATCH_LOG_FORMAT=text go run ./cmd/pulsewatch-agent

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm ci && npm run build

dev-db:
	docker compose -f docker-compose.dev.yml up -d

up:
	docker compose up -d --build

down:
	docker compose down
