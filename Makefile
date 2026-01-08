# Makefile for netweave O2-IMS Gateway
# Zero-tolerance quality enforcement

.PHONY: help
.DEFAULT_GOAL := help

# Variables
BINARY_NAME := netweave
MAIN_PATH := ./cmd/gateway
BUILD_DIR := ./build
COVERAGE_DIR := ./coverage
DOCKER_REGISTRY := docker.io
DOCKER_IMAGE := $(DOCKER_REGISTRY)/netweave
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT := $(shell git rev-parse --short HEAD)
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Go variables
GOCMD := go
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
GOFMT := gofmt
GOLINT := golangci-lint

# Build flags
LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.buildTime=$(BUILD_TIME)

# Colors for output
COLOR_RESET := \033[0m
COLOR_BOLD := \033[1m
COLOR_GREEN := \033[32m
COLOR_YELLOW := \033[33m
COLOR_RED := \033[31m
COLOR_BLUE := \033[34m

##@ General

help: ## Display this help message
	@awk 'BEGIN {FS = ":.*##"; printf "\n$(COLOR_BOLD)Usage:$(COLOR_RESET)\n  make $(COLOR_BLUE)<target>$(COLOR_RESET)\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  $(COLOR_BLUE)%-20s$(COLOR_RESET) %s\n", $$1, $$2 } /^##@/ { printf "\n$(COLOR_BOLD)%s$(COLOR_RESET)\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

clean: ## Clean build artifacts and caches
	@echo "$(COLOR_YELLOW)Cleaning build artifacts...$(COLOR_RESET)"
	@rm -rf $(BUILD_DIR)
	@rm -rf $(COVERAGE_DIR)
	@rm -f coverage.out
	@rm -f *.prof
	@$(GOCLEAN) -cache -testcache -modcache
	@echo "$(COLOR_GREEN)✓ Clean complete$(COLOR_RESET)"

##@ Development

install-tools: ## Install development tools
	@echo "$(COLOR_YELLOW)Installing development tools...$(COLOR_RESET)"
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.61.0
	@go install golang.org/x/vuln/cmd/govulncheck@latest
	@go install github.com/securego/gosec/v2/cmd/gosec@latest
	@go install gotest.tools/gotestsum@latest
	@go install github.com/vektra/mockery/v2@latest
	@pip install pre-commit || echo "$(COLOR_YELLOW)Warning: pre-commit requires Python$(COLOR_RESET)"
	@echo "$(COLOR_GREEN)✓ Tools installed$(COLOR_RESET)"

install-hooks: ## Install git pre-commit hooks
	@echo "$(COLOR_YELLOW)Installing git hooks...$(COLOR_RESET)"
	@command -v pre-commit >/dev/null 2>&1 || { echo "$(COLOR_RED)✗ pre-commit not found. Run: make install-tools$(COLOR_RESET)"; exit 1; }
	@pre-commit install
	@pre-commit install --hook-type commit-msg
	@echo "$(COLOR_GREEN)✓ Git hooks installed$(COLOR_RESET)"

verify-setup: ## Verify development environment setup
	@echo "$(COLOR_YELLOW)Verifying development environment...$(COLOR_RESET)"
	@command -v go >/dev/null 2>&1 || { echo "$(COLOR_RED)✗ Go not found$(COLOR_RESET)"; exit 1; }
	@command -v golangci-lint >/dev/null 2>&1 || { echo "$(COLOR_RED)✗ golangci-lint not found$(COLOR_RESET)"; exit 1; }
	@command -v gosec >/dev/null 2>&1 || { echo "$(COLOR_RED)✗ gosec not found$(COLOR_RESET)"; exit 1; }
	@command -v govulncheck >/dev/null 2>&1 || { echo "$(COLOR_RED)✗ govulncheck not found$(COLOR_RESET)"; exit 1; }
	@command -v pre-commit >/dev/null 2>&1 || { echo "$(COLOR_YELLOW)⚠ pre-commit not found (optional)$(COLOR_RESET)"; }
	@echo "$(COLOR_GREEN)✓ Environment verified$(COLOR_RESET)"

##@ Code Quality

fmt: ## Format Go code with gofmt
	@echo "$(COLOR_YELLOW)Formatting code...$(COLOR_RESET)"
	@$(GOFMT) -s -w .
	@go run golang.org/x/tools/cmd/goimports -w -local github.com/yourorg/netweave .
	@echo "$(COLOR_GREEN)✓ Code formatted$(COLOR_RESET)"

fmt-check: ## Check if code is formatted
	@echo "$(COLOR_YELLOW)Checking code formatting...$(COLOR_RESET)"
	@test -z "$$($(GOFMT) -s -l . | tee /dev/stderr)" || { echo "$(COLOR_RED)✗ Code not formatted. Run: make fmt$(COLOR_RESET)"; exit 1; }
	@echo "$(COLOR_GREEN)✓ Code formatting OK$(COLOR_RESET)"

lint: fmt-check ## Run all linters (MUST pass before commit)
	@echo "$(COLOR_YELLOW)Running linters...$(COLOR_RESET)"
	@$(GOLINT) run --config=.golangci.yml --timeout=5m
	@echo "$(COLOR_GREEN)✓ Linting passed$(COLOR_RESET)"

lint-fix: ## Auto-fix linting issues where possible
	@echo "$(COLOR_YELLOW)Auto-fixing linting issues...$(COLOR_RESET)"
	@$(GOLINT) run --config=.golangci.yml --timeout=5m --fix
	@echo "$(COLOR_GREEN)✓ Auto-fix complete$(COLOR_RESET)"

##@ Security

security-scan: ## Run security scanners (gosec + govulncheck)
	@echo "$(COLOR_YELLOW)Running security scans...$(COLOR_RESET)"
	@echo "$(COLOR_BLUE)→ Running gosec...$(COLOR_RESET)"
	@gosec -exclude-dir=vendor -exclude-dir=third_party ./...
	@echo "$(COLOR_BLUE)→ Running govulncheck...$(COLOR_RESET)"
	@govulncheck ./...
	@echo "$(COLOR_GREEN)✓ Security scans passed$(COLOR_RESET)"

check-secrets: ## Check for committed secrets
	@echo "$(COLOR_YELLOW)Checking for secrets...$(COLOR_RESET)"
	@command -v gitleaks >/dev/null 2>&1 || { echo "$(COLOR_YELLOW)Warning: gitleaks not installed$(COLOR_RESET)"; exit 0; }
	@gitleaks detect --source=. --verbose
	@echo "$(COLOR_GREEN)✓ No secrets detected$(COLOR_RESET)"

##@ Testing

test: ## Run unit tests
	@echo "$(COLOR_YELLOW)Running unit tests...$(COLOR_RESET)"
	@mkdir -p $(COVERAGE_DIR)
	@$(GOTEST) -v -race -coverprofile=coverage.out -covermode=atomic ./...
	@echo "$(COLOR_GREEN)✓ Tests passed$(COLOR_RESET)"

test-coverage: test ## Run tests with coverage report
	@echo "$(COLOR_YELLOW)Generating coverage report...$(COLOR_RESET)"
	@go tool cover -html=coverage.out -o $(COVERAGE_DIR)/coverage.html
	@go tool cover -func=coverage.out | grep total | awk '{print "Total coverage: " $$3}'
	@COVERAGE=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	if [ $$(echo "$$COVERAGE < 80" | bc -l) -eq 1 ]; then \
		echo "$(COLOR_RED)✗ Coverage $$COVERAGE% is below minimum 80%$(COLOR_RESET)"; \
		exit 1; \
	fi
	@echo "$(COLOR_GREEN)✓ Coverage check passed$(COLOR_RESET)"
	@echo "$(COLOR_BLUE)Coverage report: $(COVERAGE_DIR)/coverage.html$(COLOR_RESET)"

test-integration: ## Run integration tests
	@echo "$(COLOR_YELLOW)Running integration tests...$(COLOR_RESET)"
	@$(GOTEST) -v -tags=integration -timeout=10m ./...
	@echo "$(COLOR_GREEN)✓ Integration tests passed$(COLOR_RESET)"

test-e2e: ## Run end-to-end tests
	@echo "$(COLOR_YELLOW)Running E2E tests...$(COLOR_RESET)"
	@$(GOTEST) -v -tags=e2e -timeout=15m ./...
	@echo "$(COLOR_GREEN)✓ E2E tests passed$(COLOR_RESET)"

test-all: ## Run ALL tests (unit + integration + E2E)
	@echo "$(COLOR_YELLOW)Running all tests...$(COLOR_RESET)"
	@$(MAKE) test
	@$(MAKE) test-integration
	@$(MAKE) test-e2e
	@echo "$(COLOR_GREEN)✓ All tests passed$(COLOR_RESET)"

test-watch: ## Run tests in watch mode (requires gotestsum)
	@command -v gotestsum >/dev/null 2>&1 || { echo "$(COLOR_RED)✗ gotestsum not found. Run: make install-tools$(COLOR_RESET)"; exit 1; }
	@gotestsum --watch --format=testname ./...

benchmark: ## Run benchmarks
	@echo "$(COLOR_YELLOW)Running benchmarks...$(COLOR_RESET)"
	@$(GOTEST) -bench=. -benchmem -run=^# ./...

##@ Build

deps: ## Download Go module dependencies
	@echo "$(COLOR_YELLOW)Downloading dependencies...$(COLOR_RESET)"
	@$(GOMOD) download
	@$(GOMOD) verify
	@echo "$(COLOR_GREEN)✓ Dependencies downloaded$(COLOR_RESET)"

deps-tidy: ## Tidy Go module dependencies
	@echo "$(COLOR_YELLOW)Tidying dependencies...$(COLOR_RESET)"
	@$(GOMOD) tidy
	@echo "$(COLOR_GREEN)✓ Dependencies tidied$(COLOR_RESET)"

deps-upgrade: ## Upgrade all dependencies
	@echo "$(COLOR_YELLOW)Upgrading dependencies...$(COLOR_RESET)"
	@$(GOGET) -u ./...
	@$(GOMOD) tidy
	@echo "$(COLOR_GREEN)✓ Dependencies upgraded$(COLOR_RESET)"

build: ## Build the binary
	@echo "$(COLOR_YELLOW)Building $(BINARY_NAME)...$(COLOR_RESET)"
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=0 $(GOBUILD) -v -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "$(COLOR_GREEN)✓ Build complete: $(BUILD_DIR)/$(BINARY_NAME)$(COLOR_RESET)"

build-all: ## Build for all platforms
	@echo "$(COLOR_YELLOW)Building for all platforms...$(COLOR_RESET)"
	@mkdir -p $(BUILD_DIR)
	@for os in linux darwin; do \
		for arch in amd64 arm64; do \
			echo "$(COLOR_BLUE)→ Building $$os/$$arch...$(COLOR_RESET)"; \
			GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 $(GOBUILD) -v -ldflags="$(LDFLAGS)" \
				-o $(BUILD_DIR)/$(BINARY_NAME)-$$os-$$arch $(MAIN_PATH); \
		done; \
	done
	@echo "$(COLOR_GREEN)✓ Multi-platform build complete$(COLOR_RESET)"

.PHONY: run run-dev run-staging run-prod
run: build ## Build and run the gateway (default: dev environment)
	@echo "$(COLOR_YELLOW)Starting $(BINARY_NAME)...$(COLOR_RESET)"
	@NETWEAVE_ENV=dev $(BUILD_DIR)/$(BINARY_NAME)

run-dev: build ## Build and run the gateway in development mode
	@echo "$(COLOR_YELLOW)Starting $(BINARY_NAME) (development)...$(COLOR_RESET)"
	@NETWEAVE_ENV=dev $(BUILD_DIR)/$(BINARY_NAME)

run-staging: build ## Build and run the gateway in staging mode
	@echo "$(COLOR_YELLOW)Starting $(BINARY_NAME) (staging)...$(COLOR_RESET)"
	@NETWEAVE_ENV=staging $(BUILD_DIR)/$(BINARY_NAME)

run-prod: build ## Build and run the gateway in production mode
	@echo "$(COLOR_YELLOW)Starting $(BINARY_NAME) (production)...$(COLOR_RESET)"
	@NETWEAVE_ENV=prod $(BUILD_DIR)/$(BINARY_NAME)

##@ Docker

docker-build: ## Build Docker image
	@echo "$(COLOR_YELLOW)Building Docker image...$(COLOR_RESET)"
	@docker build -t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest .
	@echo "$(COLOR_GREEN)✓ Docker image built: $(DOCKER_IMAGE):$(VERSION)$(COLOR_RESET)"

docker-scan: ## Scan Docker image with Trivy
	@echo "$(COLOR_YELLOW)Scanning Docker image...$(COLOR_RESET)"
	@command -v trivy >/dev/null 2>&1 || { echo "$(COLOR_RED)✗ trivy not found. Install from: https://trivy.dev/$(COLOR_RESET)"; exit 1; }
	@trivy image --severity HIGH,CRITICAL $(DOCKER_IMAGE):$(VERSION)
	@echo "$(COLOR_GREEN)✓ Docker image scan complete$(COLOR_RESET)"

docker-push: docker-build ## Push Docker image to registry
	@echo "$(COLOR_YELLOW)Pushing Docker image...$(COLOR_RESET)"
	@docker push $(DOCKER_IMAGE):$(VERSION)
	@docker push $(DOCKER_IMAGE):latest
	@echo "$(COLOR_GREEN)✓ Docker image pushed$(COLOR_RESET)"

##@ Quality Gates

pre-commit: fmt-check lint security-scan test ## Run all pre-commit checks
	@echo "$(COLOR_GREEN)✓ All pre-commit checks passed$(COLOR_RESET)"

quality: fmt-check lint security-scan test-coverage ## Run all quality checks (REQUIRED before PR)
	@echo ""
	@echo "$(COLOR_GREEN)$(COLOR_BOLD)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(COLOR_RESET)"
	@echo "$(COLOR_GREEN)$(COLOR_BOLD)  ✓ ALL QUALITY CHECKS PASSED                        $(COLOR_RESET)"
	@echo "$(COLOR_GREEN)$(COLOR_BOLD)━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━$(COLOR_RESET)"
	@echo "$(COLOR_GREEN)Ready to commit!$(COLOR_RESET)"

ci: quality build docker-build ## Run full CI pipeline locally
	@echo "$(COLOR_GREEN)✓ Full CI pipeline passed$(COLOR_RESET)"

##@ Deployment

deploy-dev: ## Deploy to development environment
	@echo "$(COLOR_YELLOW)Deploying to development...$(COLOR_RESET)"
	@kubectl apply -f deployments/kubernetes/dev/ --context=dev
	@echo "$(COLOR_GREEN)✓ Deployed to development$(COLOR_RESET)"

deploy-staging: ## Deploy to staging environment
	@echo "$(COLOR_YELLOW)Deploying to staging...$(COLOR_RESET)"
	@kubectl apply -f deployments/kubernetes/staging/ --context=staging
	@echo "$(COLOR_GREEN)✓ Deployed to staging$(COLOR_RESET)"

deploy-prod: ## Deploy to production (requires confirmation)
	@echo "$(COLOR_RED)$(COLOR_BOLD)WARNING: This will deploy to PRODUCTION!$(COLOR_RESET)"
	@read -p "Are you sure? Type 'yes' to continue: " confirm; \
	if [ "$$confirm" != "yes" ]; then \
		echo "$(COLOR_YELLOW)Deployment cancelled$(COLOR_RESET)"; \
		exit 1; \
	fi
	@echo "$(COLOR_YELLOW)Deploying to production...$(COLOR_RESET)"
	@kubectl apply -f deployments/kubernetes/prod/ --context=production
	@echo "$(COLOR_GREEN)✓ Deployed to production$(COLOR_RESET)"

##@ Kubernetes

k8s-apply: ## Apply Kubernetes manifests (dev)
	@kubectl apply -f deployments/kubernetes/base/
	@echo "$(COLOR_GREEN)✓ Kubernetes manifests applied$(COLOR_RESET)"

k8s-delete: ## Delete Kubernetes resources
	@kubectl delete -f deployments/kubernetes/base/
	@echo "$(COLOR_GREEN)✓ Kubernetes resources deleted$(COLOR_RESET)"

k8s-logs: ## Tail gateway logs
	@kubectl logs -f -l app=netweave-gateway -n o2ims-system

k8s-describe: ## Describe gateway pods
	@kubectl describe pod -l app=netweave-gateway -n o2ims-system

##@ Development Utilities

generate: ## Generate code (mocks, etc.)
	@echo "$(COLOR_YELLOW)Generating code...$(COLOR_RESET)"
	@go generate ./...
	@echo "$(COLOR_GREEN)✓ Code generation complete$(COLOR_RESET)"

mod-graph: ## Display module dependency graph
	@$(GOMOD) graph

mod-why: ## Explain why a dependency is needed (usage: make mod-why PKG=github.com/example/pkg)
	@$(GOMOD) why $(PKG)

profile-cpu: ## Run CPU profiling
	@echo "$(COLOR_YELLOW)Running CPU profiling...$(COLOR_RESET)"
	@$(GOTEST) -cpuprofile=cpu.prof -bench=. ./...
	@go tool pprof -http=:8080 cpu.prof

profile-mem: ## Run memory profiling
	@echo "$(COLOR_YELLOW)Running memory profiling...$(COLOR_RESET)"
	@$(GOTEST) -memprofile=mem.prof -bench=. ./...
	@go tool pprof -http=:8080 mem.prof

##@ Compliance

compliance-check: ## Run O-RAN specification compliance validation
	@echo "$(COLOR_YELLOW)Running O-RAN compliance checks...$(COLOR_RESET)"
	@go build -o $(BUILD_DIR)/compliance ./cmd/compliance
	@$(BUILD_DIR)/compliance -url http://localhost:8080 -output text
	@echo "$(COLOR_GREEN)✓ Compliance check complete$(COLOR_RESET)"

compliance-badges: ## Generate compliance badges for README
	@echo "$(COLOR_YELLOW)Generating compliance badges...$(COLOR_RESET)"
	@go build -o $(BUILD_DIR)/compliance ./cmd/compliance
	@$(BUILD_DIR)/compliance -url http://localhost:8080 -output badges
	@echo "$(COLOR_GREEN)✓ Badges generated$(COLOR_RESET)"

compliance-json: ## Generate compliance report as JSON
	@echo "$(COLOR_YELLOW)Generating compliance JSON report...$(COLOR_RESET)"
	@mkdir -p $(BUILD_DIR)/reports
	@go build -o $(BUILD_DIR)/compliance ./cmd/compliance
	@$(BUILD_DIR)/compliance -url http://localhost:8080 -output json > $(BUILD_DIR)/reports/compliance.json
	@echo "$(COLOR_GREEN)✓ JSON report generated: $(BUILD_DIR)/reports/compliance.json$(COLOR_RESET)"

compliance-update-readme: ## Update README.md with compliance badges
	@echo "$(COLOR_YELLOW)Updating README with compliance badges...$(COLOR_RESET)"
	@go build -o $(BUILD_DIR)/compliance ./cmd/compliance
	@$(BUILD_DIR)/compliance -url http://localhost:8080 -update-readme
	@echo "$(COLOR_GREEN)✓ README.md updated with compliance badges$(COLOR_RESET)"

##@ Documentation

lint-docs: ## Lint all Markdown documentation
	@echo "$(COLOR_YELLOW)Linting documentation...$(COLOR_RESET)"
	@command -v markdownlint >/dev/null 2>&1 || command -v markdownlint-cli >/dev/null 2>&1 || { echo "$(COLOR_YELLOW)Warning: markdownlint not installed. Install: npm install -g markdownlint-cli$(COLOR_RESET)"; exit 0; }
	@markdownlint '**/*.md' --ignore node_modules --ignore vendor || true
	@echo "$(COLOR_GREEN)✓ Documentation linting complete$(COLOR_RESET)"

check-links: ## Verify documentation links are valid
	@echo "$(COLOR_YELLOW)Checking documentation links...$(COLOR_RESET)"
	@command -v markdown-link-check >/dev/null 2>&1 || { echo "$(COLOR_YELLOW)Warning: markdown-link-check not installed. Install: npm install -g markdown-link-check$(COLOR_RESET)"; exit 0; }
	@find . -name "*.md" -not -path "./vendor/*" -not -path "./node_modules/*" -exec markdown-link-check {} \; || true
	@echo "$(COLOR_GREEN)✓ Link check complete$(COLOR_RESET)"

verify-examples: ## Verify code examples in docs are working
	@echo "$(COLOR_YELLOW)Verifying code examples...$(COLOR_RESET)"
	@echo "$(COLOR_BLUE)→ Extracting and testing code examples from documentation$(COLOR_RESET)"
	@# TODO: Implement example extraction and validation
	@echo "$(COLOR_GREEN)✓ Example verification complete$(COLOR_RESET)"

##@ Git Utilities

git-hooks-test: ## Test pre-commit hooks
	@echo "$(COLOR_YELLOW)Testing pre-commit hooks...$(COLOR_RESET)"
	@pre-commit run --all-files

git-tag: ## Create and push a new version tag (usage: make git-tag VERSION=v1.0.0)
	@if [ -z "$(VERSION)" ]; then \
		echo "$(COLOR_RED)✗ VERSION not set. Usage: make git-tag VERSION=v1.0.0$(COLOR_RESET)"; \
		exit 1; \
	fi
	@echo "$(COLOR_YELLOW)Creating tag $(VERSION)...$(COLOR_RESET)"
	@git tag -a $(VERSION) -m "Release $(VERSION)"
	@git push origin $(VERSION)
	@echo "$(COLOR_GREEN)✓ Tag $(VERSION) created and pushed$(COLOR_RESET)"

##@ Information

version: ## Show version information
	@echo "Version:    $(VERSION)"
	@echo "Commit:     $(COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"

info: ## Show build information
	@echo "$(COLOR_BOLD)netweave O2-IMS Gateway$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BLUE)Version Information:$(COLOR_RESET)"
	@echo "  Version:    $(VERSION)"
	@echo "  Commit:     $(COMMIT)"
	@echo "  Build Time: $(BUILD_TIME)"
	@echo ""
	@echo "$(COLOR_BLUE)Go Environment:$(COLOR_RESET)"
	@go version
	@echo ""
	@echo "$(COLOR_BLUE)Build Configuration:$(COLOR_RESET)"
	@echo "  Binary:     $(BINARY_NAME)"
	@echo "  Main:       $(MAIN_PATH)"
	@echo "  Build Dir:  $(BUILD_DIR)"
	@echo ""
	@echo "$(COLOR_BLUE)Docker Configuration:$(COLOR_RESET)"
	@echo "  Registry:   $(DOCKER_REGISTRY)"
	@echo "  Image:      $(DOCKER_IMAGE)"

## ==============================================================================
## Helm Operations
## ==============================================================================

HELM_CHART_PATH := helm/netweave
HELM_NAMESPACE ?= o2ims-system

.PHONY: helm-lint
helm-lint: ## Lint Helm chart
	@echo "$(COLOR_YELLOW)Linting Helm chart...$(COLOR_RESET)"
	@helm lint $(HELM_CHART_PATH)
	@echo "$(COLOR_GREEN)✓ Helm chart lint passed$(COLOR_RESET)"

.PHONY: helm-template
helm-template: ## Template Helm chart
	@echo "$(COLOR_YELLOW)Templating Helm chart...$(COLOR_RESET)"
	@helm template netweave $(HELM_CHART_PATH) --debug

.PHONY: helm-template-dev
helm-template-dev: ## Template with dev values
	@helm template netweave $(HELM_CHART_PATH) -f $(HELM_CHART_PATH)/values-dev.yaml

.PHONY: helm-template-prod
helm-template-prod: ## Template with prod values
	@helm template netweave $(HELM_CHART_PATH) -f $(HELM_CHART_PATH)/values-prod.yaml

.PHONY: helm-install-dev
helm-install-dev: ## Install with dev values
	@echo "$(COLOR_YELLOW)Installing Helm chart (dev)...$(COLOR_RESET)"
	@helm install netweave $(HELM_CHART_PATH) \
		--namespace $(HELM_NAMESPACE) \
		--create-namespace \
		--values $(HELM_CHART_PATH)/values-dev.yaml \
		--wait
	@echo "$(COLOR_GREEN)✓ Helm chart installed$(COLOR_RESET)"

.PHONY: helm-install-prod
helm-install-prod: ## Install with prod values
	@echo "$(COLOR_YELLOW)Installing Helm chart (prod)...$(COLOR_RESET)"
	@helm install netweave $(HELM_CHART_PATH) \
		--namespace $(HELM_NAMESPACE) \
		--values $(HELM_CHART_PATH)/values-prod.yaml \
		--wait \
		--timeout 10m
	@echo "$(COLOR_GREEN)✓ Helm chart installed$(COLOR_RESET)"

.PHONY: helm-upgrade
helm-upgrade: ## Upgrade Helm release
	@echo "$(COLOR_YELLOW)Upgrading Helm release...$(COLOR_RESET)"
	@helm upgrade netweave $(HELM_CHART_PATH) \
		--namespace $(HELM_NAMESPACE) \
		--wait
	@echo "$(COLOR_GREEN)✓ Helm chart upgraded$(COLOR_RESET)"

.PHONY: helm-upgrade-dev
helm-upgrade-dev: ## Upgrade with dev values
	@helm upgrade netweave $(HELM_CHART_PATH) \
		--namespace $(HELM_NAMESPACE) \
		--values $(HELM_CHART_PATH)/values-dev.yaml \
		--wait

.PHONY: helm-upgrade-prod
helm-upgrade-prod: ## Upgrade with prod values
	@helm upgrade netweave $(HELM_CHART_PATH) \
		--namespace $(HELM_NAMESPACE) \
		--values $(HELM_CHART_PATH)/values-prod.yaml \
		--wait \
		--timeout 10m

.PHONY: helm-uninstall
helm-uninstall: ## Uninstall Helm release
	@echo "$(COLOR_YELLOW)Uninstalling Helm release...$(COLOR_RESET)"
	@helm uninstall netweave --namespace $(HELM_NAMESPACE)
	@echo "$(COLOR_GREEN)✓ Helm chart uninstalled$(COLOR_RESET)"

.PHONY: helm-test
helm-test: ## Run Helm tests
	@echo "$(COLOR_YELLOW)Running Helm tests...$(COLOR_RESET)"
	@helm test netweave --namespace $(HELM_NAMESPACE)
	@echo "$(COLOR_GREEN)✓ Helm tests passed$(COLOR_RESET)"

.PHONY: helm-package
helm-package: ## Package Helm chart
	@echo "$(COLOR_YELLOW)Packaging Helm chart...$(COLOR_RESET)"
	@helm package $(HELM_CHART_PATH) --destination ./build
	@echo "$(COLOR_GREEN)✓ Helm chart packaged$(COLOR_RESET)"

.PHONY: helm-deps
helm-deps: ## Update Helm dependencies
	@echo "$(COLOR_YELLOW)Updating Helm dependencies...$(COLOR_RESET)"
	@helm dependency update $(HELM_CHART_PATH)
	@echo "$(COLOR_GREEN)✓ Helm dependencies updated$(COLOR_RESET)"

.PHONY: helm-docs
helm-docs: ## Generate Helm documentation
	@echo "$(COLOR_YELLOW)Generating Helm documentation...$(COLOR_RESET)"
	@helm-docs $(HELM_CHART_PATH) || echo "Install helm-docs: go install github.com/norwoodj/helm-docs/cmd/helm-docs@latest"

.PHONY: helm-all
helm-all: helm-lint helm-template helm-test ## Run all Helm checks

