# Kind cluster management

KIND = $(shell pwd)/_output/tools/bin/kind
KIND_VERSION ?= v0.30.0

# Download and install kind if not already installed
.PHONY: kind
kind:
	@[ -f $(KIND) ] || { \
		set -e ;\
		echo "Installing kind to $(KIND)..." ;\
		mkdir -p $(shell dirname $(KIND)) ;\
		GOBIN=$(shell dirname $(KIND)) go install sigs.k8s.io/kind@$(KIND_VERSION) ;\
	}

KIND_CLUSTER_NAME ?= kubernetes-mcp-server

# Detect container engine (docker or podman)
CONTAINER_ENGINE ?= $(shell command -v docker 2>/dev/null || command -v podman 2>/dev/null)

.PHONY: kind-create-certs
kind-create-certs:
	@if [ ! -f _output/cert-manager-ca/ca.crt ]; then \
		echo "Creating placeholder CA certificate for bind mount..."; \
		./hack/generate-placeholder-ca.sh; \
	else \
		echo "✅ Placeholder CA already exists"; \
	fi

.PHONY: kind-create-cluster
kind-create-cluster: kind kind-create-certs
	@# Set KIND provider for podman on Linux
	@if [ "$(shell uname -s)" != "Darwin" ] && echo "$(CONTAINER_ENGINE)" | grep -q "podman"; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
	fi; \
	if $(KIND) get clusters 2>/dev/null | grep -q "^$(KIND_CLUSTER_NAME)$$"; then \
		echo "Kind cluster '$(KIND_CLUSTER_NAME)' already exists, skipping creation"; \
	else \
		echo "Creating Kind cluster '$(KIND_CLUSTER_NAME)'..."; \
		$(KIND) create cluster --name $(KIND_CLUSTER_NAME) --config dev/config/kind/cluster.yaml; \
		echo "Adding ingress-ready label to control-plane node..."; \
		kubectl label node $(KIND_CLUSTER_NAME)-control-plane ingress-ready=true --overwrite; \
		echo "Installing nginx ingress controller..."; \
		kubectl apply -f dev/config/ingress/nginx-ingress.yaml; \
		echo "Waiting for ingress controller to be ready..."; \
		kubectl wait --namespace ingress-nginx --for=condition=ready pod --selector=app.kubernetes.io/component=controller --timeout=90s; \
		echo "✅ Ingress controller ready"; \
		echo "Installing cert-manager..."; \
		kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.2/cert-manager.yaml; \
		echo "Waiting for cert-manager to be ready..."; \
		kubectl wait --namespace cert-manager --for=condition=available deployment/cert-manager --timeout=120s; \
		kubectl wait --namespace cert-manager --for=condition=available deployment/cert-manager-cainjector --timeout=120s; \
		kubectl wait --namespace cert-manager --for=condition=available deployment/cert-manager-webhook --timeout=120s; \
		echo "✅ cert-manager ready"; \
		echo "Creating cert-manager ClusterIssuer..."; \
		sleep 5; \
		kubectl apply -f dev/config/cert-manager/selfsigned-issuer.yaml; \
		echo "✅ ClusterIssuer created"; \
		echo "Adding /etc/hosts entry for Keycloak in control plane..."; \
		if command -v docker >/dev/null 2>&1 && docker ps --filter "name=$(KIND_CLUSTER_NAME)-control-plane" --format "{{.Names}}" | grep -q "$(KIND_CLUSTER_NAME)-control-plane"; then \
			docker exec $(KIND_CLUSTER_NAME)-control-plane bash -c 'grep -q "keycloak.127-0-0-1.sslip.io" /etc/hosts || echo "127.0.0.1 keycloak.127-0-0-1.sslip.io" >> /etc/hosts'; \
		elif command -v podman >/dev/null 2>&1 && podman ps --filter "name=$(KIND_CLUSTER_NAME)-control-plane" --format "{{.Names}}" | grep -q "$(KIND_CLUSTER_NAME)-control-plane"; then \
			podman exec $(KIND_CLUSTER_NAME)-control-plane bash -c 'grep -q "keycloak.127-0-0-1.sslip.io" /etc/hosts || echo "127.0.0.1 keycloak.127-0-0-1.sslip.io" >> /etc/hosts'; \
		fi; \
		echo "✅ /etc/hosts entry added"; \
	fi
	@echo "Exporting kubeconfig to _output/kubeconfig..."; \
	mkdir -p _output; \
	$(KIND) export kubeconfig --name $(KIND_CLUSTER_NAME) --kubeconfig _output/kubeconfig; \
	echo "✅ Kubeconfig exported to _output/kubeconfig"

.PHONY: kind-delete-cluster
kind-delete-cluster: kind
	@# Set KIND provider for podman on Linux
	@if [ "$(shell uname -s)" != "Darwin" ] && echo "$(CONTAINER_ENGINE)" | grep -q "podman"; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
	fi; \
	$(KIND) delete cluster --name $(KIND_CLUSTER_NAME)
