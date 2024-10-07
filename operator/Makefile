# VERSION defines the project version for the bundle.
VERSION ?= 0.0.1

CLUSTER_NAME ?= paladin
NAMESPACE ?= paladin

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
IMAGE_TAG_BASE ?= paladin.io/operator-go
IMG ?= $(IMAGE_TAG_BASE):$(VERSION)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)

# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
USE_IMAGE_DIGESTS ?= false

# Set the Operator SDK version
OPERATOR_SDK_VERSION ?= v1.37.0

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.29.0

# Tool Binaries and Versions
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_TOOLS_VERSION)
ENVTEST ?= $(LOCALBIN)/setup-envtest-$(ENVTEST_VERSION)
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)
CRD_REF_DOCS ?= $(LOCALBIN)/crd-ref-docs
HELMIFY ?= $(LOCALBIN)/helmify
CONTAINER_TOOL ?= docker
HELM ?= helm
KIND_CLUSTER ?= kind

# PLATFORMS defines the target platforms for cross-platform builds
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le

# Tool Versions
KUSTOMIZE_VERSION ?= v5.4.3
CONTROLLER_TOOLS_VERSION ?= v0.16.3
ENVTEST_VERSION ?= release-0.17
GOLANGCI_LINT_VERSION ?= v1.57.2

# Paths for Helm charts
CHART_PATH_CRD = charts/paladin-operator-crd
CHART_NAME_CRD = paladin-crds
CHART_PATH_OPERATOR = charts/paladin-operator
CHART_NAME_OPERATOR = paladin

# SHELL setup to allow bash commands
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# Helper to install go tools
define go-install-tool
@[ -f $(1) ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv "$$(echo "$(1)" | sed "s/-$(3)$$//")" $(1) ;\
}
endef

##@ General Targets
.PHONY: all
all: build

.PHONY: help
help: ## Display help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development
.PHONY: manifests
manifests: controller-gen ## Generate CRDs and Webhook configs.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate DeepCopy and related methods.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run unit tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: lint
lint: golangci-lint ## Run linters (go and yaml).
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run linters and apply fixes.
	$(GOLANGCI_LINT) run --fix

##@ E2E Testing (Kind)
.PHONY: test-e2e
test-e2e: ## Run e2e tests with Kind.
	go test ./test/e2e/ -v -ginkgo.v

.PHONY: kind-start
kind-start:	## Create a Kind cluster.
	if ! $(KIND_CLUSTER) get clusters | grep -q "^${CLUSTER_NAME}$$"; then \
		$(KIND_CLUSTER) create cluster --name "${CLUSTER_NAME}" --config paladin-kind.yaml; \
	fi
	$(KUBECTL) version

.PHONY: kind-promote
kind-promote: kind-start ## Load Docker images into Kind cluster.
	$(KIND_CLUSTER) load docker-image paladin-operator:latest --name "${CLUSTER_NAME}"
	$(KIND_CLUSTER) load docker-image paladin:test --name "${CLUSTER_NAME}"
	$(KIND_CLUSTER) load docker-image postgres:latest --name "${CLUSTER_NAME}"
	$(KIND_CLUSTER) load docker-image hyperledger/besu:latest --name "${CLUSTER_NAME}"

.PHONY: kind-delete
kind-delete:	## Delete the Kind cluster.
	$(KIND_CLUSTER) delete cluster --name "${CLUSTER_NAME}" || true

##@ Build
.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run 
run: manifests generate fmt vet ## Run controller locally.
	go run ./cmd/main.go

.PHONY: docker-build
docker-build: ## Build Docker image for manager.
	$(CONTAINER_TOOL) build -f Dockerfile -t ${IMG} ..

.PHONY: docker-load
docker-load: docker-build ## Load Docker image into Kind.
	$(KIND_CLUSTER) load docker-image ${IMG} --name ${CLUSTER_NAME}

.PHONY: docker-push
docker-push: ## Push Docker image to registry.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: docker-buildx
docker-buildx: ## Build and push Docker image for cross-platform support.
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name paladin-builder
	$(CONTAINER_TOOL) buildx use paladin-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm paladin-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate ## Build consolidated installer YAML.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Helm Chart - Deployment
.PHONY: build-crds
build-crds: manifests helmify ## Build CRDs using helmify.
	$(KUSTOMIZE) build config/default | $(HELMIFY) ${CHART_PATH_CRD}
	find ${CHART_PATH_CRD}/templates/ -type f ! \( -name '*-crd.yaml' -o -name '_helpers.tpl' \) -exec rm -f {} +
	rm -f ${CHART_PATH_CRD}/values.yaml
	touch ${CHART_PATH_CRD}/values.yaml

.PHONY: install-crds
install-crds: build-crds ## Install CRDs using Helm.
	$(HELM) upgrade --install ${CHART_NAME_CRD} ${CHART_PATH_CRD}

.PHONY: uninstall-crds
uninstall-crds: ## Uninstall CRDs.
	$(HELM) uninstall ${CHART_NAME_CRD} || true

.PHONY: helm-template
helm-template: ## Install operator using Helm.
	$(HELM) template ${CHART_NAME_OPERATOR} ${CHART_PATH_OPERATOR} --set operator.namespace=${NAMESPACE} 

.PHONY: helm-install
helm-install: ## Install operator using Helm.
	$(HELM) upgrade --install ${CHART_NAME_OPERATOR} ${CHART_PATH_OPERATOR} -n ${NAMESPACE} --create-namespace --set operator.namespace=${NAMESPACE}

.PHONY: helm-uninstall
helm-uninstall: ## Uninstall operator using Helm.
	$(HELM) uninstall ${CHART_NAME_OPERATOR} -n ${NAMESPACE} || true
	$(KUBECTL) delete namespace ${NAMESPACE} || true

.PHONY: helm-pause
helm-pause: ## Pause the operator (set replica count to 0).
	$(HELM) upgrade --install ${CHART_NAME_OPERATOR} ${CHART_PATH_OPERATOR} -n ${NAMESPACE} --create-namespace --set operator.namespace=${NAMESPACE} --set replicaCount=0

##@ Kustomize - Deployment
.PHONY: kustomize-install
kustomize-install: manifests ## Install CRDs using Kustomize.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: kustomize-uninstall
kustomize-uninstall: manifests ## Uninstall CRDs using Kustomize.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=true -f -

.PHONY: kustomize-deploy
kustomize-deploy: manifests ## Deploy controller using Kustomize.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: kustomize-undeploy
kustomize-undeploy: ## Undeploy controller using Kustomize.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=true -f -

##@ Dependencies
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Install controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Install envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Install golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,${GOLANGCI_LINT_VERSION})

.PHONY: helmify
helmify: $(HELMIFY) ## Install helmify locally if necessary.
$(HELMIFY): $(LOCALBIN)
	test -s $(LOCALBIN)/helmify || GOBIN=$(LOCALBIN) go install github.com/arttor/helmify/cmd/helmify@latest

.PHONY: crd-ref-docs
crd-ref-docs: $(CRD_REF_DOCS) ## Install crd-ref-docs locally if necessary.
$(CRD_REF_DOCS): $(LOCALBIN)
	test -s $(LOCALBIN)/crd-ref-docs || GOBIN=$(LOCALBIN) go install github.com/elastic/crd-ref-docs@v0.0.9

##@ Dev Helpers
.PHONY: update-crds
update-crds: manifests generate ## Update CRDs in the cluster.
	$(KUBECTL) apply -k config/crd

.PHONY: generate-reference-docs
generate-reference-docs: crd-ref-docs ## Generate API reference documentation.
	${CRD_REF_DOCS} --source-path "${PWD}/api/" --config "${PWD}/docs/config.yaml" --renderer=markdown --output-path="${PWD}/docs/api.md"

##@ Paladin Operator

##@ Paladin Operator Instances

.PHONY: create-node-besu
create-node-besu: ## Create a Besu Node instance.
	$(KUBECTL) apply -f config/samples/core_v1alpha1_besu.yaml -n ${NAMESPACE}

.PHONY: create-genesis
create-genesis: ## Create a Besu Genesis instance.
	$(KUBECTL) apply -f config/samples/core_v1alpha1_besugenesis.yaml -n ${NAMESPACE}

.PHONY: create-node-paladin
create-node-paladin: ## Create a Paladin Node instance.
	$(KUBECTL) apply -f config/samples/core_v1alpha1_paladin.yaml -n ${NAMESPACE}

.PHONY: create-node
create-node: create-node-besu create-genesis create-node-paladin ## Create all instances.
	@echo "All instances have been created."

.PHONY: delete-node
delete-node-paladin: ## Delete the Paladin Node instance.
	$(KUBECTL) delete -f config/samples/core_v1alpha1_paladin.yaml -n ${NAMESPACE} || true

.PHONY: delete-genesis
delete-genesis: ## Delete the Besu Genesis instance.
	$(KUBECTL) delete -f config/samples/core_v1alpha1_besugenesis.yaml -n ${NAMESPACE} || true

.PHONY: delete-node-besu
delete-node-besu: ## Delete the Besu Node instance.
	$(KUBECTL) delete -f config/samples/core_v1alpha1_besu.yaml -n ${NAMESPACE} || true

.PHONY: delete-node
delete-node: delete-node-paladin delete-genesis delete-node-besu  ## Delete all instances.
	@echo "All instances have been deleted."

##@ Cleanup
.PHONY: clean
clean: delete-node helm-uninstall uninstall-crds ## Clean up all resources.
