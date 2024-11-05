# VERSION defines the project version for the bundle.
VERSION ?= 0.0.1

CLUSTER_NAME ?= paladin
NAMESPACE ?= default

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
IMAGE_TAG_BASE ?= paladin.io/paladin-operator
OPERATOR_IMG ?= $(IMAGE_TAG_BASE):$(VERSION)
BUNDLE_OPERATOR_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)
PALADIN_IMG ?= paladin:test 

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
	fi;

# $(CONTAINER_TOOL) images -q postgres:latest || $(CONTAINER_TOOL) pull postgres:latest
# $(CONTAINER_TOOL) images -q hyperledger/besu:latest || $(CONTAINER_TOOL) pull hyperledger/besu:latest
.PHONY: kind-promote
kind-promote: kind-start ## Load Docker images into Kind cluster.
	echo "Loading images into Kind cluster..."
	$(KIND_CLUSTER) load docker-image ${OPERATOR_IMG} --name "${CLUSTER_NAME}"
	$(KIND_CLUSTER) load docker-image ${PALADIN_IMG} --name "${CLUSTER_NAME}"

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
	$(CONTAINER_TOOL) build -f Dockerfile -t ${OPERATOR_IMG} ..

.PHONY: docker-load
docker-load: docker-build ## Load Docker image into Kind.
	$(KIND_CLUSTER) load docker-image ${OPERATOR_IMG} --name ${CLUSTER_NAME}

.PHONY: docker-push
docker-push: ## Push Docker image to registry.
	$(CONTAINER_TOOL) push ${OPERATOR_IMG}

.PHONY: docker-buildx
docker-buildx: ## Build and push Docker image for cross-platform support.
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name paladin-builder
	$(CONTAINER_TOOL) buildx use paladin-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${OPERATOR_IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm paladin-builder
	rm Dockerfile.cross

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary. If wrong version is installed, it will be removed before downloading.
$(KUSTOMIZE): $(LOCALBIN)
	@if test -x $(LOCALBIN)/kustomize && ! $(LOCALBIN)/kustomize version | grep -q $(KUSTOMIZE_VERSION); then \
		echo "$(LOCALBIN)/kustomize version is not expected $(KUSTOMIZE_VERSION). Removing it before installing."; \
		rm -rf $(LOCALBIN)/kustomize; \
	fi
	test -s $(LOCALBIN)/kustomize || { curl -Ss $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN); }

.PHONY: build-installer
build-installer: manifests generate kustomize ## Build consolidated installer YAML.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${OPERATOR_IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Helm Chart - Deployment
.PHONY: prepare-crd-chart
prepare-crd-chart: manifests kustomize helmify ## Build CRDs using helmify.
	$(KUSTOMIZE) build config/default | $(HELMIFY) ${CHART_PATH_CRD}
	find ${CHART_PATH_CRD}/templates/ -type f ! \( -name '*-crd.yaml' -o -name '_helpers.tpl' \) -exec rm -f {} +
	rm -f ${CHART_PATH_CRD}/values.yaml
	touch ${CHART_PATH_CRD}/values.yaml
	ls -la charts/paladin-operator-crd/templates

.PHONY: install-crds
install-crds: prepare-crd-chart ## Install CRDs using Helm.
	$(HELM) upgrade --install ${CHART_NAME_CRD} ${CHART_PATH_CRD}


.PHONY: uninstall-crds
uninstall-crds: ## Uninstall CRDs.
	$(HELM) uninstall ${CHART_NAME_CRD} || true

.PHONY: helm-template
helm-template: ## Install operator using Helm.
	$(HELM) template ${CHART_NAME_OPERATOR} ${CHART_PATH_OPERATOR} --set operator.namespace=${NAMESPACE} 

.PHONY: helm-install
helm-install: ## Install operator using Helm.
	$(HELM) dependency build ${CHART_PATH_OPERATOR} || true
	$(HELM) upgrade --install ${CHART_NAME_OPERATOR} ${CHART_PATH_OPERATOR} \
		-n ${NAMESPACE} --create-namespace \
		--set operator.namespace=${NAMESPACE} \
		--set operator.image.repository=${IMAGE_TAG_BASE} \
		--set operator.image.tag=${VERSION} \
		--set certManager.enabled=true \
		--set certManager.namespace=cert-manager \
		--set prometheus.enabled=false
	kubectl kustomize ./config/samples | kubectl apply -f -

.PHONY: helm-uninstall
helm-uninstall: ## Uninstall operator using Helm.
	$(HELM) uninstall ${CHART_NAME_OPERATOR} -n ${NAMESPACE} || true
	$(KUBECTL) delete namespace ${NAMESPACE} || true
	$(KUBECTL) delete namespace cert-manager || true
	$(KUBECTL) delete namespace prometheus || true

.PHONY: helm-pause
helm-pause: ## Pause the operator (set replica count to 0).
	$(HELM) upgrade --install ${CHART_NAME_OPERATOR} ${CHART_PATH_OPERATOR} -n ${NAMESPACE} --create-namespace --set operator.namespace=${NAMESPACE} --set replicaCount=0

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
	test -s $(LOCALBIN)/crd-ref-docs || GOBIN=$(LOCALBIN) go install github.com/elastic/crd-ref-docs@latest

##@ Dev Helpers

.PHONY: generate-reference-docs
generate-reference-docs: crd-ref-docs ## Generate API reference documentation.
	${CRD_REF_DOCS} --source-path "${PWD}/api/" --config "${PWD}/docs/config.yaml" --renderer=markdown --output-path="${PWD}/docs/api.md"

##@ Paladin Operator

##@ Paladin Operator Instances

.PHONY: create-node
create-node: # TODO: more complete startup check
	kubectl apply -n ${NAMESPACE} -k config/samples; \
	kubectl wait paladins.core.paladin.io/node1 --for jsonpath='{.status.phase}=Completed' --namespace ${NAMESPACE} --timeout 5m

.PHONY: delete-node
delete-node:
	kubectl delete -n ${NAMESPACE} -k config/samples || true

##@ Cleanup
DELETE_CLUSTER ?= false
.PHONY: clean
clean: helm-uninstall uninstall-crds ## Clean up all resources.
	@if [ "$(DELETE_CLUSTER)" = "true" ]; then \
		$(MAKE) kind-delete; \
	else \
		echo "Skipping Kind cluster deletion. Use DELETE_CLUSTER=true to delete it."; \
	fi



# kubectl wait deployment.apps/cert-manager-webhook --for condition=Available --namespace cert-manager --timeout 5m
