VERSION ?= 1.0.0
IMG ?= registry-api.tas.scharber.com/datasource-operator:$(VERSION)

.PHONY: build test docker-build docker-push install uninstall run ci help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build binary
	CGO_ENABLED=0 go build -o bin/datasource-operator ./cmd/operator/

test: ## Run tests
	go test ./... -v

test-coverage: ## Run tests with coverage
	go test ./... -coverprofile=cover.out
	go tool cover -func=cover.out

lint: ## Run linter
	golangci-lint run

fmt: ## Format code
	go fmt ./...

vet: ## Run go vet
	go vet ./...

generate: ## Generate deepcopy functions
	controller-gen object paths="./api/v1/"

docker-build: ## Build Docker image
	docker build -t $(IMG) .

docker-push: ## Push Docker image
	docker push $(IMG)

install: ## Install CRDs
	kubectl apply -f deployments/kubernetes/crds/datasourceconnection_crd.yaml

uninstall: ## Uninstall CRDs
	kubectl delete -f deployments/kubernetes/crds/datasourceconnection_crd.yaml

deploy: install ## Deploy operator
	kubectl apply -f deployments/kubernetes/crds/rbac.yaml
	kubectl apply -f deployments/kubernetes/

run: ## Run operator locally
	go run ./cmd/operator/ --metrics-bind-address=:8088 --health-probe-bind-address=:8089

ci: fmt vet test build ## Full CI pipeline
