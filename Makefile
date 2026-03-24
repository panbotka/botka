.PHONY: fmt vet lint test check build run clean migrate-up migrate-down migrate-create frontend-install frontend-dev dev-backend frontend-build prod-build deploy ensure-dist

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

# Run tests with race detector
test: ensure-dist
	CGO_ENABLED=1 go test -race -coverprofile=coverage.out ./cmd/... ./internal/...

# Full CI gate: format + vet + lint + test
check: fmt vet lint test

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
	sudo systemctl stop botka
	sudo cp bin/$(BINARY_NAME) /usr/bin/$(BINARY_NAME)
	sudo systemctl start botka

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
