SHELL := /usr/bin/env bash

GO ?= go
PKGS := ./...
BIN_DIR := .bin

export GOTOOLCHAIN=auto

.PHONY: help
help:
	@echo "Targets:"
	@echo "  fmt              - format Go code"
	@echo "  test             - run unit/integration tests"
	@echo "  test-race        - run race tests"
	@echo "  vet              - run go vet"
	@echo "  lint             - run golangci-lint"
	@echo "  staticcheck      - run staticcheck"
	@echo "  vuln             - run govulncheck"
	@echo "  fuzz-smoke       - short fuzz smoke run"
	@echo "  build            - build all packages"
	@echo "  verify           - full local quality gate"
	@echo "  tidy             - tidy modules"
	@echo "  check-generated  - verify git tree clean after codegen"
	@echo "  ci               - CI entrypoint"

.PHONY: fmt
fmt:
	$(GO) fmt $(PKGS)

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: test
test:
	$(GO) test -count=1 $(PKGS)

.PHONY: test-race
test-race:
	$(GO) test -race -count=1 $(PKGS)

.PHONY: vet
vet:
	$(GO) vet $(PKGS)

.PHONY: lint
lint:
	golangci-lint run --timeout=10m

.PHONY: staticcheck
staticcheck:
	staticcheck $(PKGS)

.PHONY: vuln
vuln:
	govulncheck $(PKGS)

.PHONY: fuzz-smoke
fuzz-smoke:
	@set -euo pipefail; \
	packages="$$( $(GO) list ./... )"; \
	found=0; \
	for pkg in $$packages; do \
		tests="$$( $(GO) test -list='^Fuzz' $$pkg 2>/dev/null || true )"; \
		if echo "$$tests" | grep -q '^Fuzz'; then \
			found=1; \
			while IFS= read -r fuzzname; do \
				[ -z "$$fuzzname" ] && continue; \
				[ "$$fuzzname" = "ok" ] && continue; \
				echo "Running fuzz smoke: $$pkg $$fuzzname"; \
				$(GO) test $$pkg -run=^$$ -fuzz="^$${fuzzname}$$" -fuzztime=5s; \
			done < <(echo "$$tests" | grep '^Fuzz'); \
		fi; \
	done; \
	if [ "$$found" -eq 0 ]; then \
		echo "No fuzz tests found, skipping"; \
	fi

.PHONY: build
build:
	$(GO) build $(PKGS)

.PHONY: check-generated
check-generated:
	@git diff --exit-code -- . ':(exclude)go.sum' || (echo "Generated files or tracked files changed"; exit 1)

.PHONY: verify
verify: fmt tidy test test-race vet lint staticcheck vuln build

.PHONY: ci
ci: tidy test test-race vet lint staticcheck vuln build
