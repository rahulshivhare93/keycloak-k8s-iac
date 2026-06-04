SHELL := /usr/bin/env bash
PULUMI_DIR := pulumi
HOSTNAME ?= keycloak.localhost

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

.PHONY: preflight
preflight: ## Install required tooling and start the Docker runtime
	@./scripts/preflight.sh

.PHONY: up
up: ## Provision the cluster and deploy Keycloak (one command)
	@./scripts/deploy.sh

.PHONY: creds
creds: ## Print the Keycloak admin credentials
	@./scripts/credentials.sh

.PHONY: url
url: ## Print the Keycloak URL
	@cd $(PULUMI_DIR) && pulumi stack output keycloakURL 2>/dev/null

.PHONY: verify
verify: ## Verify HTTPS access and admin login end-to-end
	@./scripts/verify.sh

.PHONY: status
status: ## Show cluster workload status
	@kubectl get pods,svc,ingress,networkpolicy -n keycloak 2>/dev/null || \
		echo "Cluster not reachable. Run 'make up' first."

.PHONY: hosts
hosts: ## Add a /etc/hosts entry for the Keycloak hostname (requires sudo)
	@if grep -q "$(HOSTNAME)" /etc/hosts; then \
		echo "$(HOSTNAME) already present in /etc/hosts"; \
	else \
		echo "Adding 127.0.0.1 $(HOSTNAME) to /etc/hosts (sudo)"; \
		echo "127.0.0.1 $(HOSTNAME)" | sudo tee -a /etc/hosts >/dev/null; \
	fi

.PHONY: destroy
destroy: ## Tear down Keycloak and the k3d cluster
	@./scripts/destroy.sh

.PHONY: fmt
fmt: ## Format Go code
	@cd $(PULUMI_DIR) && gofmt -w . && go mod tidy

.PHONY: vet
vet: ## Run go vet
	@cd $(PULUMI_DIR) && go vet ./...

.PHONY: lint
lint: ## Run golangci-lint (installs if missing)
	@command -v golangci-lint >/dev/null 2>&1 || \
		{ echo "Installing golangci-lint..."; brew install golangci-lint; }
	@cd $(PULUMI_DIR) && golangci-lint run ./...

.PHONY: build
build: ## Compile the Pulumi program
	@cd $(PULUMI_DIR) && go build ./...

.PHONY: scan
scan: ## Run security scans: gitleaks (secrets) + govulncheck (Go CVEs)
	@./scripts/scan.sh

.PHONY: check
check: fmt vet build ## Format, vet and build (run before committing)
