IMG ?= registry-api.tas.scharber.com/federated-mcp-operator:0.1.0
CONTROLLER_GEN ?= $(shell go env GOPATH)/bin/controller-gen

.PHONY: all
all: generate manifests fmt vet build

.PHONY: generate
generate: ## Generate zz_generated.deepcopy.go
	$(CONTROLLER_GEN) object paths="./api/v1/..."

.PHONY: manifests
manifests: ## Generate CRD + RBAC manifests
	$(CONTROLLER_GEN) crd paths="./api/v1/..." output:crd:dir=./config/crd
	$(CONTROLLER_GEN) rbac:roleName=federated-mcp-operator-role paths="./pkg/controllers/..." output:rbac:dir=./config/rbac

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: test
test:
	go test ./... -count=1

.PHONY: build
build:
	go build -o bin/operator ./cmd/operator

.PHONY: docker-build
docker-build:
	docker build -t $(IMG) .

.PHONY: docker-push
docker-push:
	docker push $(IMG)

.PHONY: install
install: ## Apply the CRD
	kubectl apply -f config/crd/

.PHONY: deploy
deploy: install ## Apply RBAC + operator Deployment
	kubectl apply -f config/rbac/
	kubectl apply -f config/manager/deployment.yaml

.PHONY: samples
samples:
	kubectl apply -f config/samples/
