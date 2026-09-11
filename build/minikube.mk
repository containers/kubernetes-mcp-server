# Minikube cluster management

MINIKUBE_PROFILE ?= kubernetes-mcp-server
MINIKUBE_SPOKE_PROFILE ?= kubernetes-mcp-server-spoke

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

# _minikube_ensure_cluster creates or reuses a minikube cluster.
# $(1) = profile name, $(2) = label (for log messages), $(3) = extra minikube start flags
define _minikube_ensure_cluster
if $(MINIKUBE) status --profile $(1) -f '{{.Host}}' 2>/dev/null | grep -q Running; then \
	echo "$(2) cluster '$(1)' already running, skipping create"; \
else \
	if $(MINIKUBE) status --profile $(1) -f '{{.Host}}' 2>/dev/null | grep -q .; then \
		echo "$(2) cluster '$(1)' exists in bad state, cleaning up..."; \
		$(MINIKUBE) delete --profile $(1) || true; \
	fi; \
	echo "Creating $(2) cluster '$(1)' (driver=$(MINIKUBE_DRIVER))..."; \
	$(MINIKUBE) start \
		--profile $(1) \
		--driver $(MINIKUBE_DRIVER) \
		$(3) \
		--cpus 2 --memory 4096 \
		--container-runtime containerd; \
fi
endef

# _minikube_load_image loads a container image into a minikube cluster.
# $(1) = profile name
define _minikube_load_image
if echo "$(CONTAINER_ENGINE)" | grep -q podman; then \
	set -e; \
	echo "Saving image $(E2E_IMAGE) via podman..."; \
	TMPTAR=$$(mktemp /tmp/e2e-image-XXXXXX.tar); \
	trap 'rm -f $$TMPTAR' EXIT; \
	$(CONTAINER_ENGINE) save -o "$$TMPTAR" $(E2E_IMAGE); \
	echo "Loading image into minikube (profile $(1))..."; \
	$(MINIKUBE) image load "$$TMPTAR" --profile $(1); \
	rm -f "$$TMPTAR"; \
else \
	echo "Loading image $(E2E_IMAGE) into minikube (profile $(1))..."; \
	$(MINIKUBE) image load $(E2E_IMAGE) --profile $(1); \
fi
endef

##@ Minikube Cluster

.PHONY: minikube-create-cluster
minikube-create-cluster: minikube ## Create Minikube cluster for development
	@$(call _minikube_ensure_cluster,$(MINIKUBE_PROFILE),Hub,--apiserver-port=6443)
	@echo "Exporting kubeconfig to _output/kubeconfig..."
	@mkdir -p _output
	@$(MINIKUBE) kubectl --profile $(MINIKUBE_PROFILE) -- config view --flatten --minify > _output/kubeconfig
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
	@$(call _minikube_load_image,$(MINIKUBE_PROFILE))

##@ Spoke Cluster (multicluster)

# The spoke cluster is created on the hub's container network (--network) so that
# pods on the hub can reach the spoke API server by container IP, and the spoke
# node can reach the hub's Keycloak NodePort.
.PHONY: minikube-create-spoke-cluster
minikube-create-spoke-cluster: minikube ## Create spoke Minikube cluster for multicluster testing
	@$(call _minikube_ensure_cluster,$(MINIKUBE_SPOKE_PROFILE),Spoke,--network $(MINIKUBE_PROFILE) --apiserver-port=6444)

.PHONY: minikube-delete-spoke-cluster
minikube-delete-spoke-cluster: minikube ## Delete spoke Minikube cluster
	@if $(MINIKUBE) status --profile $(MINIKUBE_SPOKE_PROFILE) -f '{{.Host}}' 2>/dev/null | grep -q Running; then \
		echo "Cleaning up spoke RBAC before deletion..."; \
		$(MINIKUBE) kubectl --profile $(MINIKUBE_SPOKE_PROFILE) -- delete -f dev/config/keycloak/rbac-spoke.yaml --ignore-not-found 2>/dev/null || true; \
	fi
	@if $(MINIKUBE) status --profile $(MINIKUBE_SPOKE_PROFILE) -f '{{.Host}}' 2>/dev/null | grep -q .; then \
		echo "Deleting spoke cluster '$(MINIKUBE_SPOKE_PROFILE)'..."; \
		$(MINIKUBE) delete --profile $(MINIKUBE_SPOKE_PROFILE); \
		rm -f _output/kubeconfig-spoke; \
	else \
		echo "Spoke cluster '$(MINIKUBE_SPOKE_PROFILE)' does not exist, nothing to delete"; \
	fi

.PHONY: minikube-load-image-spoke
minikube-load-image-spoke: minikube ## Load a container image into the spoke Minikube cluster
	@$(call _minikube_load_image,$(MINIKUBE_SPOKE_PROFILE))

.PHONY: minikube-merge-kubeconfigs
minikube-merge-kubeconfigs: kubectl ## Merge hub and spoke kubeconfigs into _output/kubeconfig
	@echo "Merging kubeconfigs for hub and spoke..."
	@test -f _output/kubeconfig || { echo "ERROR: _output/kubeconfig (hub) not found — run keycloak-install first"; exit 1; }
	@test -f _output/kubeconfig-spoke || { echo "ERROR: _output/kubeconfig-spoke not found — run keycloak-setup-spoke first"; exit 1; }
	@MERGED=$$(mktemp); \
		KUBECONFIG=$(shell pwd)/_output/kubeconfig:$(shell pwd)/_output/kubeconfig-spoke \
		$(KUBECTL) config view --flatten > "$$MERGED" && \
		mv "$$MERGED" _output/kubeconfig
	@echo "Merged kubeconfig written to _output/kubeconfig"

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
