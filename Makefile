# Image URL to use all building/pushing image targets
REGISTRY ?= docker.io/your-username  # Change this to your registry
IMAGE_NAME ?= kubeflare-light
TAG ?= latest
IMG ?= $(REGISTRY)/$(IMAGE_NAME):$(TAG)
NAMESPACE ?= kubeflare-light-system
CLOUDFLARE_API_TOKEN ?= your-cloudflare-api-token  # Replace with your token or set via environment

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: docker-build docker-push deploy ## Build and deploy the operator

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'.
.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	GO111MODULE=on go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	GO111MODULE=on go vet ./...

.PHONY: test
test: fmt vet ## Run tests.
	GO111MODULE=on go test ./... -coverprofile cover.out

##@ Build

.PHONY: tidy
tidy: ## Run go mod tidy to clean up dependencies
	GO111MODULE=on go mod tidy

.PHONY: build
build: tidy ## Build manager binary.
	GO111MODULE=on go build -o bin/manager cmd/kubeflare/main.go

.PHONY: run
run: ## Run a controller from your host.
	GO111MODULE=on go run cmd/kubeflare/main.go

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

##@ Deployment

.PHONY: install-crds
install-crds: ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	kubectl apply -f deploy/crds/

.PHONY: uninstall-crds
uninstall-crds: ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	kubectl delete -f deploy/crds/

.PHONY: apply-rbac
apply-rbac: ## Apply RBAC resources to the cluster
	kubectl apply -f deploy/rbac.yaml

.PHONY: create-namespace
create-namespace: ## Create namespace if it doesn't exist
	kubectl create namespace $(NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -

.PHONY: create-secret
create-secret: ## Create Cloudflare API token secret
	@echo "Creating Cloudflare API token secret..."
	@echo "Using token: $(if $(CLOUDFLARE_API_TOKEN),$(if $(shell echo $(CLOUDFLARE_API_TOKEN) | grep -E '^your-'),Please replace default token in Makefile or set via environment,OK),Token not set!)"
	kubectl create secret generic cloudflare-api-token \
		--namespace $(NAMESPACE) \
		--from-literal=token=$(CLOUDFLARE_API_TOKEN) \
		--dry-run=client -o yaml | kubectl apply -f -

.PHONY: apply-deployment
apply-deployment: ## Apply deployment with the correct image
	@echo "Applying deployment with image $(IMG)..."
	@sed "s|image: kubeflare-light:latest|image: $(IMG)|g" deploy/deployment.yaml | kubectl apply -f -

.PHONY: deploy
deploy: create-namespace install-crds apply-rbac create-secret apply-deployment ## Deploy controller to the K8s cluster
	@echo "Deployment complete!"
	@echo "You can check the deployment status with:"
	@echo "  kubectl get pods -n $(NAMESPACE)"

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster.
	kubectl delete -f deploy/deployment.yaml --ignore-not-found
	kubectl delete -f deploy/rbac.yaml --ignore-not-found
	kubectl delete -f deploy/crds/ --ignore-not-found
	@echo "You may want to remove the namespace with:"
	@echo "  kubectl delete namespace $(NAMESPACE)"

.PHONY: generate-examples
generate-examples: ## Generate example resources from the examples directory
	kubectl apply -f examples/

##@ Code Generation

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=deploy/crds

##@ Tools

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen

## Tool Versions
CONTROLLER_TOOLS_VERSION ?= v0.13.0

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)
