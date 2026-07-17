# ==== Utility-style Makefile ====

# Use bash for all systems consistency
SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

# ==== Variables ====
ARTIFACTS_DIR := $(PWD)/.reports/pipelines
BIN_DIR := .bin
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
CACHE_DIR := $(PWD)/.cache
GIT_COMMIT ?= $(shell git rev-parse HEAD)
GOCACHE_DIR := go-build
GOVULNCHECK_ARTIFACT := govulncheck-report.json
PREFIX ?= /usr/local
# Must match GitHub repository name
PROJECT_NAME := oci-registry-mirror
SDLC_ARTIFACTS_DIR := SDLC
SECURITY_ARTIFACTS_DIR := security
UNIT_TEST_OUT_ARTIFACT := coverage.out
UNIT_TEST_XML_ARTIFACT := coverage.xml
VERSION ?= development

# Export coverage variables to shell
export MIN_COVERAGE=75.0


# ==== Convenience targets ====
##@Convenience


# ==== Clean ====
.PHONY: clean
clean: ## Complete clean (using all clean available targets)
	@echo "[TASK] Complete clean"
	$(MAKE) clean-build
	$(MAKE) clean-debug
	@echo "✅ Clean completed successfully"

.PHONY: clean-build
clean-build: ## Clean build artifacts, caches, and reports
	@echo "[TASK] Clean build artifacts and caches"
	@rm -rf .bin .cache .reports
	@go clean -testcache
	@echo "✅ Build clean completed successfully"

.PHONY: clean-debug
clean-debug: ## Clean debug/runtime artifacts
	@echo "[TASK] Clean debug artifacts"
	@rm -rf tmp/
	@find . -name '__debug_bin*' -exec rm -f {} + 2>/dev/null || true
	@echo "✅ Debug clean completed successfully"


# ==== Security ====
.PHONY: check-vulnerability
check-vulnerability: ## Run vulnerability check on project
	@echo "[TASK] Running vulnerability check JSON scan"
	@mkdir -p "$(ARTIFACTS_DIR)/$(SECURITY_ARTIFACTS_DIR)"
	go run golang.org/x/vuln/cmd/govulncheck@latest -json ./... > "$(ARTIFACTS_DIR)/$(SECURITY_ARTIFACTS_DIR)/$(GOVULNCHECK_ARTIFACT)"
	@if [ -s "$(ARTIFACTS_DIR)/$(SECURITY_ARTIFACTS_DIR)/$(GOVULNCHECK_ARTIFACT)" ] && grep -q '"finding"' "$(ARTIFACTS_DIR)/$(SECURITY_ARTIFACTS_DIR)/$(GOVULNCHECK_ARTIFACT)"; then \
		echo "❌ Vulnerabilities found:"; \
		jq -r 'select(.finding != null) | .finding as $$f | $$f.trace[] | "Package: \(.module) ~> Fixed version: \($$f.fixed_version)"' "$(ARTIFACTS_DIR)/$(SECURITY_ARTIFACTS_DIR)/$(GOVULNCHECK_ARTIFACT)" | sort -u; \
		exit 1; \
	else \
		echo "✅ No known vulnerabilities found"; \
	fi


# === Quality ===
.PHONY: format-check
format-check: ## Check code formatting
	@echo "[TASK] Checking code formatting"
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "❌ Formatting issues found:"; \
		gofmt -l ./$(PROJECT_NAME); \
		exit 1; \
	else \
		echo "✅ Code formatting is correct"; \
	fi

.PHONY: lint
lint: ## Run golangci-lint
	@command -v golangci-lint >/dev/null 2>&1 || { echo "❌ golangci-lint is not installed"; exit 1; }
	@echo "[TASK] Running golangci-lint for component"
	@if golangci-lint run ./...; then \
		echo "✅ Lint passed successfully"; \
	else \
		echo "❌ Lint failed"; \
		exit 1; \
	fi


# ==== Build lifecycle ====
.PHONY: build
build: ## Build a single component binary
	@echo "[TASK] Building"

	@mkdir -p "$(CACHE_DIR)/$(GOCACHE_DIR)" "$(BIN_DIR)"
	@export GOCACHE="$(CACHE_DIR)/$(GOCACHE_DIR)" && \
	\
	go mod download && \
	echo "🔨 Building binary" && \
	go build -ldflags "\
		-X 'github.com/ghdrope/go-version.Version=$(VERSION)' \
		-X 'github.com/ghdrope/go-version.GitCommit=$(GIT_COMMIT)' \
		-X 'github.com/ghdrope/go-version.BuildDate=$(BUILD_DATE)'" \
		-o "$(BIN_DIR)/$(PROJECT_NAME)" "$(PWD)/cmd"; \
	echo "✅ Build completed successfully"; \


# ==== Tests ====
.PHONY: test-unit
test-unit: build ## Run unit tests with coverage enforcement
	@echo "[TASK] Running unit tests"
	@mkdir -p "$(ARTIFACTS_DIR)/$(SDLC_ARTIFACTS_DIR)"

	PACKAGES=$$(go list ./internal/... ./pkg/... | grep -v '/tests' | grep -v 'testhelper' | grep -v '^$(PROJECT_NAME)/$$') && \
	echo "$$PACKAGES" && \
	\
	COVERAGE_MIN=$$MIN_COVERAGE; \
	\
	go test -v -coverprofile="$(ARTIFACTS_DIR)/$(SDLC_ARTIFACTS_DIR)/$(PROJECT_NAME)-$(UNIT_TEST_OUT_ARTIFACT)" $$PACKAGES && \
	\
	COVERAGE_ACTUAL=$$(go tool cover -func="$(ARTIFACTS_DIR)/$(SDLC_ARTIFACTS_DIR)/$(PROJECT_NAME)-$(UNIT_TEST_OUT_ARTIFACT)" | grep total: | awk '{print substr($$3,1,length($$3)-1)}') && \
	if awk "BEGIN {exit !($$COVERAGE_ACTUAL >= $$COVERAGE_MIN)}"; then \
		echo "📊 Total Coverage $$COVERAGE_ACTUAL% >= Minimum Coverage $$COVERAGE_MIN%"; \
	else \
		echo "❌ Total Coverage $$COVERAGE_ACTUAL% < Minimum Coverage $$COVERAGE_MIN%"; \
		exit 1; \
	fi && \
	\
	if command -v gocover-cobertura >/dev/null 2>&1; then \
		gocover-cobertura < "$(ARTIFACTS_DIR)/$(SDLC_ARTIFACTS_DIR)/$(PROJECT_NAME)-$(UNIT_TEST_OUT_ARTIFACT)" > "$(ARTIFACTS_DIR)/$(SDLC_ARTIFACTS_DIR)/$(PROJECT_NAME)-$(UNIT_TEST_XML_ARTIFACT)" && \
		echo "📝 Cobertura report generated: '$(ARTIFACTS_DIR)/$(SDLC_ARTIFACTS_DIR)/$(PROJECT_NAME)-$(UNIT_TEST_XML_ARTIFACT)'"; \
	else \
		echo "⚠️ gocover-cobertura not found, skipping Cobertura report"; \
	fi 

	@echo "✅ Unit tests completed successfully"


# ==== Help ====
.PHONY: help
help: ## help: list available targets
	@echo "Available targets:"
	@gawk '\
		/^##@Convenience/ {in_convenience=1; next} \
		/^##@/ {in_convenience=0} \
		in_convenience && /^[a-zA-Z0-9_-]+:.*?##/ { \
			match($$0, /^([a-zA-Z0-9_-]+):/, arr); \
			target = arr[1]; \
			sub(/^.*## /,"",$$0); \
			printf "  \033[36m%-30s\033[0m %s\n", target, $$0 \
		} \
	' $(MAKEFILE_LIST)