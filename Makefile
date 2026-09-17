.PHONY: help build test test-integration vet fmt fmt-check tidy update-vendor-hash

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
	@echo "  update-vendor-hash  Build the backend vendor hash"

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

update-vendor-hash:
	@cp flake.nix flake.nix.bak
	@sed -i.tmp -E 's/vendorHash = "[^"]*";/vendorHash = "";/' flake.nix && rm flake.nix.tmp
	@HASH=$$(nix build .#default.goModules --no-link 2>&1 | grep -oE 'got:[[:space:]]+sha256-\S+' | awk '{print $$2}'); \
	if [ -z "$$HASH" ]; then \
			echo "Failed to extract vendorHash — build may have succeeded (deps unchanged) or failed for another reason."; \
			mv flake.nix.bak flake.nix; \
			exit 1; \
	fi; \
	sed -i.tmp -E "s/vendorHash = \"\";/vendorHash = \"$$HASH\";/" flake.nix && rm flake.nix.tmp; \
	rm flake.nix.bak; \
	echo "vendorHash updated to $$HASH"
