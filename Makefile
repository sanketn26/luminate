.PHONY: help build test clean docker-build docker-push deploy-k8s lint fmt vet deps run dev

# Variables
APP_NAME := luminate
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)

# Docker variables
DOCKER_REGISTRY ?= docker.io
DOCKER_REPO ?= yourusername
DOCKER_IMAGE := $(DOCKER_REGISTRY)/$(DOCKER_REPO)/$(APP_NAME)
DOCKER_TAG ?= $(VERSION)

# Build directory
BUILD_DIR := build
BIN_DIR := $(BUILD_DIR)/bin

# Go variables
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
GOFMT := gofmt
GOLINT := golangci-lint

help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

deps: ## Download dependencies
	$(GOMOD) download
	$(GOMOD) tidy

build: deps ## Build the application
	@echo "Building $(APP_NAME) version $(VERSION)..."
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME)-linux-amd64 ./cmd/$(APP_NAME)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME)-darwin-amd64 ./cmd/$(APP_NAME)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME)-darwin-arm64 ./cmd/$(APP_NAME)
	@echo "Build complete!"

build-linux: deps ## Build for Linux only
	@echo "Building $(APP_NAME) for Linux..."
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME) ./cmd/$(APP_NAME)

build-local: deps ## Build for current platform
	@echo "Building $(APP_NAME) for current platform..."
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME) ./cmd/$(APP_NAME)

test: ## Run tests
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

test-coverage: test ## Run tests with coverage report
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

benchmark: ## Run benchmarks
	$(GOTEST) -bench=. -benchmem ./...

lint: ## Run linter
	$(GOLINT) run ./...

fmt: ## Format code
	$(GOFMT) -s -w .

vet: ## Run go vet
	$(GOCMD) vet ./...

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "Clean complete!"

run: build-local ## Build and run locally
	$(BIN_DIR)/$(APP_NAME) --config configs/config.yaml

dev: ## Run in development mode with hot reload (requires air)
	air -c .air.toml

# Docker targets

docker-build: ## Build Docker image
	@echo "Building Docker image $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		-t $(DOCKER_IMAGE):latest \
		-f deployments/docker/Dockerfile .
	@echo "Docker build complete!"

docker-push: docker-build ## Push Docker image to registry
	@echo "Pushing Docker image $(DOCKER_IMAGE):$(DOCKER_TAG)..."
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)
	docker push $(DOCKER_IMAGE):latest
	@echo "Docker push complete!"

docker-run: ## Run Docker container locally
	docker run -it --rm \
		-p 8080:8080 \
		-v $(PWD)/configs:/app/configs \
		-e LUMINATE_AUTH_JWT_SECRET=dev-secret \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

# Kubernetes targets

k8s-deploy: ## Deploy to Kubernetes (ClickHouse backend)
	@echo "Deploying to Kubernetes..."
	kubectl apply -f deployments/kubernetes/namespace.yaml
	kubectl apply -f deployments/kubernetes/configmap.yaml
	kubectl apply -f deployments/kubernetes/secret.yaml
	kubectl apply -f deployments/kubernetes/deployment.yaml
	kubectl apply -f deployments/kubernetes/service.yaml
	kubectl apply -f deployments/kubernetes/hpa.yaml
	@echo "Deployment complete!"

k8s-deploy-badger: ## Deploy to Kubernetes (BadgerDB backend - stateful)
	@echo "Deploying to Kubernetes with BadgerDB..."
	kubectl apply -f deployments/kubernetes/namespace.yaml
	kubectl apply -f deployments/kubernetes/configmap-badger.yaml
	kubectl apply -f deployments/kubernetes/secret.yaml
	kubectl apply -f deployments/kubernetes/statefulset.yaml
	kubectl apply -f deployments/kubernetes/service.yaml
	@echo "Deployment complete!"

k8s-delete: ## Delete Kubernetes resources
	kubectl delete -f deployments/kubernetes/ --ignore-not-found=true

k8s-logs: ## View logs from Kubernetes pods
	kubectl logs -f -l app=$(APP_NAME) -n luminate

k8s-status: ## Check Kubernetes deployment status
	kubectl get all -n luminate

# Database setup targets

clickhouse-setup: ## Setup ClickHouse database schema
	@echo "Setting up ClickHouse schema..."
	@bash scripts/clickhouse-setup.sh

clickhouse-local: ## Run ClickHouse locally with Docker
	docker run -d \
		--name clickhouse-local \
		-p 8123:8123 \
		-p 9000:9000 \
		-v clickhouse-data:/var/lib/clickhouse \
		clickhouse/clickhouse-server:latest

# Development tools

install-tools: ## Install development tools
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/cosmtrek/air@latest
	@echo "Tools installed!"

generate: ## Generate code (mocks, etc.)
	$(GOCMD) generate ./...

# Release targets

release: test lint ## Create a release build
	@echo "Creating release $(VERSION)..."
	@mkdir -p $(BUILD_DIR)/release
	$(MAKE) build
	tar -czf $(BUILD_DIR)/release/$(APP_NAME)-$(VERSION)-linux-amd64.tar.gz -C $(BIN_DIR) $(APP_NAME)-linux-amd64
	tar -czf $(BUILD_DIR)/release/$(APP_NAME)-$(VERSION)-darwin-amd64.tar.gz -C $(BIN_DIR) $(APP_NAME)-darwin-amd64
	tar -czf $(BUILD_DIR)/release/$(APP_NAME)-$(VERSION)-darwin-arm64.tar.gz -C $(BIN_DIR) $(APP_NAME)-darwin-arm64
	@echo "Release artifacts created in $(BUILD_DIR)/release/"

# CI/CD targets

ci: deps lint test build ## Run CI pipeline locally

cd: docker-build docker-push k8s-deploy ## Run CD pipeline

.DEFAULT_GOAL := help
