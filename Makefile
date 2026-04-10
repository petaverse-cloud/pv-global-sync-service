.PHONY: build run test test-coverage lint clean docker-build docker-run help

APP_NAME := global-sync
BUILD_DIR := bin
LDFLAGS := -ldflags="-s -w -X main.version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)"

## help: Show available commands
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## .*:' Makefile | sed 's/## \(.*\): \(.*\)/  \1 - \2/'

## build: Build the binary
build:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME) ./cmd/server

## run: Run the service (requires environment variables)
run: build
	./$(BUILD_DIR)/$(APP_NAME)

## test: Run unit tests
test:
	go test -v -race -count=1 ./...

## test-coverage: Run tests with coverage report
test-coverage:
	go test -v -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## fmt: Format code
fmt:
	go fmt ./...

## tidy: Clean up go.mod
tidy:
	go mod tidy

## vet: Run go vet
vet:
	go vet ./...

## clean: Remove build artifacts
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

## docker-build: Build Docker image
docker-build:
	docker build -t global-sync-service:latest .

## docker-run: Run Docker container
docker-run:
	docker run --rm -p 8080:8080 \
		-e REGION=na \
		-e REGIONAL_DB_HOST=localhost \
		-e GLOBAL_INDEX_DB_HOST=localhost \
		-e REDIS_HOST=localhost \
		-e ROCKETMQ_NAME_SERVER=localhost:9876 \
		global-sync-service:latest

## ci: Run all CI checks (lint + vet + test)
ci: lint vet test-coverage
	@echo "All CI checks passed"

## init: Initialize development environment
init:
	@echo "Installing dependencies..."
	go mod download
	@echo "Installing golangci-lint..."
	@command -v golangci-lint >/dev/null 2>&1 || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Setup complete"
