##@ E2E Tests

UV = $(shell pwd)/_output/tools/bin/uv
UV_VERSION = 0.7.15
E2E_IMAGE ?= localhost/kubernetes-mcp-server:e2e
E2E_DIR = test/e2e

KUBECTL ?= $(shell pwd)/_output/tools/bin/kubectl
KUBECTL_VERSION ?= v1.36.2

HELM = $(shell pwd)/_output/tools/bin/helm
HELM_VERSION ?= v3.21.2

KEYCLOAK_CA_CERT ?= $(shell pwd)/_output/cert-manager-ca/ca.crt
SERVER_BINARY ?= $(shell pwd)/kubernetes-mcp-server
MULTICLUSTER_KUBECONFIG ?= $(shell pwd)/_output/multicluster-kubeconfig

.PHONY: uv
uv:
	@[ -f $(UV) ] || { \
		set -e ;\
		echo "Installing uv $(UV_VERSION) to $$(dirname $(UV))..." ;\
		mkdir -p $$(dirname $(UV)) ;\
		curl -LsSf https://astral.sh/uv/$(UV_VERSION)/install.sh | env UV_INSTALL_DIR=$$(dirname $(UV)) INSTALLER_NO_MODIFY_PATH=1 sh ;\
	}

# Download and install kubectl if not already installed
.PHONY: kubectl
kubectl:
	@[ -f $(KUBECTL) ] || { \
		set -e ;\
		echo "Installing kubectl $(KUBECTL_VERSION) to $(KUBECTL)..." ;\
		mkdir -p $$(dirname $(KUBECTL)) ;\
		OS=$$(uname -s | tr '[:upper:]' '[:lower:]') ;\
		ARCH=$$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') ;\
		curl -L https://dl.k8s.io/release/$(KUBECTL_VERSION)/bin/$${OS}/$${ARCH}/kubectl -o $(KUBECTL) ;\
		chmod +x $(KUBECTL) ;\
	}

# Download and install helm if not already installed
.PHONY: helm
helm:
	@[ -f $(HELM) ] || { \
		set -e ;\
		echo "Installing helm $(HELM_VERSION) to $(HELM)..." ;\
		mkdir -p $$(dirname $(HELM)) ;\
		TMPDIR=$$(mktemp -d) ;\
		OS=$$(uname -s | tr '[:upper:]' '[:lower:]') ;\
		ARCH=$$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') ;\
		curl -L https://get.helm.sh/helm-$(HELM_VERSION)-$${OS}-$${ARCH}.tar.gz | tar xz -C $$TMPDIR ;\
		mv $$TMPDIR/$${OS}-$${ARCH}/helm $(HELM) ;\
		rm -rf $$TMPDIR ;\
	}

ENTRA_MOCK_IMAGE ?= localhost/entra-mock:e2e
ENTRA_MOCK_DIR = test/e2e/entra-mock

.PHONY: entra-mock-image
entra-mock-image: kind ## Build the entra-mock container image and load it into the Kind cluster
	$(CONTAINER_ENGINE) build -t $(ENTRA_MOCK_IMAGE) $(ENTRA_MOCK_DIR)
	@if [ "$(shell uname -s)" != "Darwin" ] && echo "$(CONTAINER_ENGINE)" | grep -q "podman"; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
	fi; \
	$(CONTAINER_ENGINE) save $(ENTRA_MOCK_IMAGE) -o _output/entra-mock-image.tar
	@if [ "$(shell uname -s)" != "Darwin" ] && echo "$(CONTAINER_ENGINE)" | grep -q "podman"; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
	fi; \
	$(KIND) load image-archive _output/entra-mock-image.tar --name $(KIND_CLUSTER_NAME)
	@rm -f _output/entra-mock-image.tar

.PHONY: e2e-image
e2e-image: entra-mock-image ## Build the e2e container images and load them into the Kind cluster
	$(CONTAINER_ENGINE) build -t $(E2E_IMAGE) .
	@if [ "$(shell uname -s)" != "Darwin" ] && echo "$(CONTAINER_ENGINE)" | grep -q "podman"; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
	fi; \
	$(CONTAINER_ENGINE) save $(E2E_IMAGE) -o _output/e2e-image.tar
	@if [ "$(shell uname -s)" != "Darwin" ] && echo "$(CONTAINER_ENGINE)" | grep -q "podman"; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
	fi; \
	$(KIND) load image-archive _output/e2e-image.tar --name $(KIND_CLUSTER_NAME)
	@rm -f _output/e2e-image.tar

.PHONY: e2e-full-setup
e2e-full-setup: build kind-create-cluster ## Build binary, create Kind clusters, install all infra, and build e2e images
	-$(MAKE) kind-create-secondary-cluster kind-multicluster-kubeconfig
	@# Restore kubectl context to primary cluster (secondary creation switches it)
	-kubectl config use-context kind-$(KIND_CLUSTER_NAME) >/dev/null 2>&1
	-$(MAKE) keycloak-install
	-$(MAKE) kuadrant-setup
	$(MAKE) e2e-image

.PHONY: e2e-full-cleanup
e2e-full-cleanup: ## Delete both Kind clusters and clean up e2e artifacts
	@echo "Cleaning up e2e environment..."
	-$(MAKE) kind-delete-cluster
	-$(MAKE) kind-delete-secondary-cluster
	@rm -f $(MULTICLUSTER_KUBECONFIG)
	@rm -rf _output/cert-manager-ca
	@echo "E2E environment cleaned up"

.PHONY: e2e-test
e2e-test: uv kubectl helm ## Run all e2e tests (auto-skips tests whose infrastructure is missing)
	KUBECTL_BIN=$(KUBECTL) HELM_BIN=$(HELM) MCP_SERVER_IMAGE=$(E2E_IMAGE) \
	KEYCLOAK_CA_CERT=$(KEYCLOAK_CA_CERT) SERVER_BINARY=$(SERVER_BINARY) \
	MULTICLUSTER_KUBECONFIG=$(MULTICLUSTER_KUBECONFIG) \
	$(UV) run --directory $(E2E_DIR) --locked pytest -v $(PYTEST_ARGS)
