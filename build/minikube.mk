# Minikube cluster management

MINIKUBE_PROFILE ?= kubernetes-mcp-server

# Detect container engine — prefer podman on Linux to stay consistent with MINIKUBE_DRIVER
MINIKUBE_DRIVER ?= $(shell command -v podman >/dev/null 2>&1 && [ "$$(uname -s)" = "Linux" ] && echo podman || echo docker)
CONTAINER_ENGINE ?= $(MINIKUBE_DRIVER)

# Enable rootless mode for podman driver on Linux
ifeq ($(MINIKUBE_DRIVER),podman)
ifeq ($(shell uname -s),Linux)
export MINIKUBE_ROOTLESS ?= true
endif
endif

CERT_MANAGER_VERSION ?= v1.16.2

##@ Minikube Cluster

.PHONY: minikube-create-cluster
minikube-create-cluster: minikube ## Create Minikube cluster for development
	@if $(MINIKUBE) status --profile $(MINIKUBE_PROFILE) -f '{{.Host}}' 2>/dev/null | grep -q Running; then \
		echo "Cluster '$(MINIKUBE_PROFILE)' already running, skipping create"; \
	else \
		if $(MINIKUBE) status --profile $(MINIKUBE_PROFILE) -f '{{.Host}}' 2>/dev/null | grep -q .; then \
			echo "Cluster '$(MINIKUBE_PROFILE)' exists in bad state, cleaning up..."; \
			$(MINIKUBE) delete --profile $(MINIKUBE_PROFILE) --purge || true; \
		fi; \
		echo "Creating Minikube cluster '$(MINIKUBE_PROFILE)' (driver=$(MINIKUBE_DRIVER))..."; \
		$(MINIKUBE) start \
			--profile $(MINIKUBE_PROFILE) \
			--driver $(MINIKUBE_DRIVER) \
			--cpus 2 --memory 4096 \
			--apiserver-port=6443 \
			--container-runtime containerd; \
	fi
	@echo "Exporting kubeconfig to _output/kubeconfig..."
	@mkdir -p _output
	@$(MINIKUBE) kubectl --profile $(MINIKUBE_PROFILE) -- config view --flatten > _output/kubeconfig
	@echo "Kubeconfig exported to _output/kubeconfig"

.PHONY: minikube-delete-cluster
minikube-delete-cluster: minikube ## Delete Minikube cluster
	@if $(MINIKUBE) status --profile $(MINIKUBE_PROFILE) -f '{{.Host}}' 2>/dev/null | grep -q .; then \
		echo "Deleting Minikube cluster '$(MINIKUBE_PROFILE)'..."; \
		$(MINIKUBE) delete --profile $(MINIKUBE_PROFILE); \
		rm -f _output/kubeconfig; \
	else \
		echo "Cluster '$(MINIKUBE_PROFILE)' does not exist, nothing to delete"; \
	fi

.PHONY: minikube-load-image
minikube-load-image: minikube ## Load a container image into the Minikube cluster
	@if echo "$(CONTAINER_ENGINE)" | grep -q podman; then \
		set -e; \
		echo "Saving image $(E2E_IMAGE) via podman..."; \
		TMPTAR=$$(mktemp /tmp/e2e-image-XXXXXX.tar); \
		trap 'rm -f $$TMPTAR' EXIT; \
		$(CONTAINER_ENGINE) save -o "$$TMPTAR" $(E2E_IMAGE); \
		echo "Loading image into minikube..."; \
		$(MINIKUBE) image load "$$TMPTAR" --profile $(MINIKUBE_PROFILE); \
		rm -f "$$TMPTAR"; \
	else \
		echo "Loading image $(E2E_IMAGE) into minikube..."; \
		$(MINIKUBE) image load $(E2E_IMAGE) --profile $(MINIKUBE_PROFILE); \
	fi

##@ Cluster Addons

.PHONY: install-cert-manager
install-cert-manager: kubectl ## Install cert-manager and self-signed ClusterIssuer
	@echo "Installing cert-manager $(CERT_MANAGER_VERSION)..."
	@$(KUBECTL) apply -f https://github.com/cert-manager/cert-manager/releases/download/$(CERT_MANAGER_VERSION)/cert-manager.yaml
	@echo "Waiting for cert-manager deployments..."
	@$(KUBECTL) wait --namespace cert-manager --for=condition=available deployment/cert-manager --timeout=120s
	@$(KUBECTL) wait --namespace cert-manager --for=condition=available deployment/cert-manager-cainjector --timeout=120s
	@$(KUBECTL) wait --namespace cert-manager --for=condition=available deployment/cert-manager-webhook --timeout=120s
	@echo "Waiting for webhook readiness..."
	@sleep 5
	@$(KUBECTL) apply -f dev/config/cert-manager/selfsigned-issuer.yaml
	@echo "cert-manager installed"
