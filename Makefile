.PHONY: help fmt vet lint test test-coverage check build run clean migrate-up migrate-down migrate-create frontend-install frontend-dev frontend-test dev-backend frontend-build prod-build deploy install-service docker-build docker-up docker-down ensure-dist test-db

## help: Show available targets
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Development:"
	@echo "  run              Run Go backend on :5110"
	@echo "  frontend-dev     Run Vite dev server on :5173 (proxies /api to :5110)"
	@echo "  dev-backend      Alias for run"
	@echo ""
	@echo "Quality:"
	@echo "  fmt              Format code with goimports + gofmt"
	@echo "  vet              Run go vet"
	@echo "  lint             Run golangci-lint"
	@echo "  test             Run tests with race detector"
	@echo "  test-coverage    Run tests with coverage report and threshold check"
	@echo "  check            Full CI gate: fmt + vet + lint + test"
	@echo ""
	@echo "Build:"
	@echo "  build            Build Go binary to build/botka"
	@echo "  prod-build       Build frontend + Go binary to bin/botka"
	@echo "  frontend-install Install frontend npm dependencies"
	@echo "  frontend-build   Build frontend only"
	@echo "  clean            Remove build artifacts"
	@echo ""
	@echo "Deploy:"
	@echo "  deploy           Build and deploy to systemd service"
	@echo "  install-service  Install systemd unit file and enable"
	@echo "  docker-build     Build Docker image"
	@echo "  docker-up        Start with docker compose"
	@echo "  docker-down      Stop docker compose"
	@echo ""
	@echo "Database:"
	@echo "  migrate-up       Apply all pending migrations"
	@echo "  migrate-down     Rollback last migration"
	@echo "  migrate-create   Create migration (NAME=migration_name)"

BINARY_NAME=botka
BUILD_DIR=build
GOBIN=$(shell go env GOPATH)/bin

# Format code with goimports and gofmt
fmt:
	$(GOBIN)/goimports -w cmd/ internal/
	gofmt -w cmd/ internal/

# Ensure frontend/dist exists for go:embed (creates placeholder if not built)
ensure-dist:
	@mkdir -p frontend/dist
	@if [ -z "$$(ls -A frontend/dist 2>/dev/null)" ]; then touch frontend/dist/.gitkeep; fi

# Run go vet
vet: ensure-dist
	go vet ./cmd/... ./internal/...

# Run golangci-lint
lint: ensure-dist
	golangci-lint run ./cmd/... ./internal/...

# Create the test database (run once)
test-db:
	docker exec shared-postgres psql -U postgres -c "CREATE DATABASE botka_test OWNER botka;" 2>/dev/null || true

# Run tests with race detector
DATABASE_TEST_URL ?= postgres://botka:botka@localhost:5432/botka_test?sslmode=disable
test: ensure-dist
	CGO_ENABLED=1 DATABASE_TEST_URL="$(DATABASE_TEST_URL)" go test -race -coverprofile=coverage.out ./cmd/... ./internal/...

# Run tests with coverage and print summary; fail if below threshold
test-coverage: ensure-dist
	CGO_ENABLED=1 DATABASE_TEST_URL="$(DATABASE_TEST_URL)" go test -race -coverprofile=coverage.out ./cmd/... ./internal/...
	@echo ""
	@echo "=== Coverage by package ==="
	@go tool cover -func=coverage.out | grep -E '(^total:|internal/)'
	@echo ""
	@go tool cover -html=coverage.out -o coverage.html
	@echo "HTML report: coverage.html"
	@total=$$(go tool cover -func=coverage.out | grep '^total:' | awk '{print $$NF}' | tr -d '%'); \
	threshold=45; \
	echo "Total coverage: $${total}% (threshold: $${threshold}%)"; \
	if [ $$(echo "$${total} < $${threshold}" | bc) -eq 1 ]; then \
		echo "FAIL: coverage $${total}% is below $${threshold}%"; \
		exit 1; \
	fi

# Type-check frontend (no bundle)
frontend-check:
	cd frontend && npx tsc -b

# Run frontend unit tests
frontend-test:
	cd frontend && npx vitest run

# Full CI gate: format + vet + lint + test + frontend type-check + frontend tests
check: fmt vet lint test frontend-check frontend-test

# Build the server binary
build: ensure-dist
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/server

# Run the server (development)
run: ensure-dist
	go run ./cmd/server

# Run Go backend with hot reload via air (if installed)
dev-backend: ensure-dist
	go run ./cmd/server

# Run Vite dev server
frontend-dev:
	cd frontend && npm run dev

# Frontend targets
frontend-install:
	cd frontend && npm ci

frontend-build:
	cd frontend && npm run build

# Build production binary with embedded frontend
prod-build: frontend-build
	CGO_ENABLED=0 go build -o bin/$(BINARY_NAME) ./cmd/server

# Deploy: build, stop service, copy binary, start service
deploy: prod-build
	sudo systemctl stop botka || true
	sudo cp bin/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	sudo systemctl start botka

# Install systemd service: copy unit file and enable
install-service:
	sudo cp packaging/botka.service /etc/systemd/system/botka.service
	sudo systemctl daemon-reload
	sudo systemctl enable botka

# Docker targets
docker-build:
	docker compose build

docker-up:
	docker compose up -d

docker-down:
	docker compose down

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR) bin/
	rm -f coverage.out coverage.html

# Database migrations
DATABASE_URL ?= postgres://botka:botka@localhost:5432/botka?sslmode=disable

migrate-up:
	migrate -database "$(DATABASE_URL)" -path migrations up

migrate-down:
	migrate -database "$(DATABASE_URL)" -path migrations down 1

migrate-create:
	@if [ -z "$(NAME)" ]; then echo "Usage: make migrate-create NAME=migration_name"; exit 1; fi
	migrate create -ext sql -dir migrations -seq $(NAME)
