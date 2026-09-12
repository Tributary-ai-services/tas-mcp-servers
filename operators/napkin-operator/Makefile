.PHONY: all build test clean docker-build install uninstall help

# Build variables
BINARY_NAME=napkin-operator
VERSION?=1.0.0
IMG?=tas-mcp/napkin-operator:$(VERSION)
GO=go

# Go build flags
LDFLAGS=-ldflags "-X main.version=$(VERSION)"

all: build

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the operator binary
	$(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/operator/

test: ## Run unit tests
	$(GO) test ./... -v -count=1

test-coverage: ## Run tests with coverage
	$(GO) test ./... -v -coverprofile=coverage.out
	$(GO) tool cover -html=coverage.out -o coverage.html

lint: ## Run linter
	golangci-lint run ./...

fmt: ## Format code
	$(GO) fmt ./...

vet: ## Run go vet
	$(GO) vet ./...

clean: ## Clean build artifacts
	rm -rf bin/ coverage.out coverage.html

docker-build: ## Build Docker image
	docker build -t $(IMG) .

docker-push: docker-build ## Push Docker image
	docker push $(IMG)

install: ## Install CRDs into cluster
	kubectl apply -f deployments/kubernetes/crds/napkinvisual_crd.yaml

uninstall: ## Uninstall CRDs from cluster
	kubectl delete -f deployments/kubernetes/crds/napkinvisual_crd.yaml

deploy: install ## Deploy operator to cluster
	kubectl apply -k ../../k8s/napkin-operator/

undeploy: ## Remove operator from cluster
	kubectl delete -k ../../k8s/napkin-operator/

run: ## Run operator locally
	$(GO) run ./cmd/operator/ --metrics-bind-address=:8088 --health-probe-bind-address=:8089

ci: fmt vet test build ## Full CI pipeline
