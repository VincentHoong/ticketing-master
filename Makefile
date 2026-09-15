.PHONY: help build test test-integration vet fmt fmt-check tidy

BACKEND := backend

.DEFAULT_GOAL := help

help:
	@echo "Available targets:"
	@echo "  build             Build the backend binaries"
	@echo "  test              Run fast unit tests (no Docker required)"
	@echo "  test-integration  Run integration + race tests against real Postgres/Redis via testcontainers (requires Docker)"
	@echo "  vet               Run go vet"
	@echo "  fmt               Format all Go files"
	@echo "  fmt-check         Check formatting without modifying files"
	@echo "  tidy              Run go mod tidy, including integration-tagged files"

build:
	cd $(BACKEND) && go build ./...

test:
	cd $(BACKEND) && go test ./...

test-integration:
	cd $(BACKEND) && go test -tags=integration -race ./integrationtest/...

vet:
	cd $(BACKEND) && go vet ./...

fmt:
	cd $(BACKEND) && gofmt -w .

fmt-check:
	cd $(BACKEND) && test -z "$$(gofmt -l .)"

tidy:
	cd $(BACKEND) && GOFLAGS=-tags=integration go mod tidy
