# Kind cluster management

KIND_CLUSTER_NAME ?= kubernetes-mcp-server

# Detect container engine (docker or podman)
CONTAINER_ENGINE ?= $(shell command -v docker 2>/dev/null || command -v podman 2>/dev/null)

.PHONY: kind-create-certs
kind-create-certs: ## Generate placeholder CA certificate for KIND bind mount
	@if [ ! -f hack/cert-manager-ca/ca.crt ]; then \
		echo "Creating placeholder CA certificate for bind mount..."; \
		./hack/generate-placeholder-ca.sh; \
	else \
		echo "✅ Placeholder CA already exists"; \
	fi

.PHONY: kind-create-cluster
kind-create-cluster: kind kind-create-certs ## Create the kind cluster for development
	@# Set KIND provider for podman on Linux
	@if [ "$(shell uname -s)" != "Darwin" ] && echo "$(CONTAINER_ENGINE)" | grep -q "podman"; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
	fi; \
	if $(KIND) get clusters 2>/dev/null | grep -q "^$(KIND_CLUSTER_NAME)$$"; then \
		echo "Kind cluster '$(KIND_CLUSTER_NAME)' already exists, skipping creation"; \
	else \
		echo "Creating Kind cluster '$(KIND_CLUSTER_NAME)'..."; \
		$(KIND) create cluster --name $(KIND_CLUSTER_NAME) --config config/kind/cluster.yaml; \
		echo "Adding ingress-ready label to control-plane node..."; \
		kubectl label node $(KIND_CLUSTER_NAME)-control-plane ingress-ready=true --overwrite; \
		echo "Installing nginx ingress controller..."; \
		kubectl apply -f config/ingress/nginx-ingress.yaml; \
		echo "Waiting for ingress controller to be ready..."; \
		kubectl wait --namespace ingress-nginx --for=condition=ready pod --selector=app.kubernetes.io/component=controller --timeout=90s; \
		echo "✅ Ingress controller ready"; \
		echo "Installing cert-manager..."; \
		kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.2/cert-manager.yaml; \
		echo "Waiting for cert-manager to be ready..."; \
		kubectl wait --namespace cert-manager --for=condition=ready pod --selector=app.kubernetes.io/instance=cert-manager --timeout=120s; \
		kubectl wait --namespace cert-manager --for=condition=ready pod --selector=app.kubernetes.io/name=webhook --timeout=120s; \
		echo "✅ cert-manager ready"; \
		echo "Creating cert-manager ClusterIssuer..."; \
		sleep 5; \
		kubectl apply -f config/cert-manager/selfsigned-issuer.yaml; \
		echo "✅ ClusterIssuer created"; \
	fi

.PHONY: kind-delete-cluster
kind-delete-cluster: kind ## Delete the kind cluster
	@# Set KIND provider for podman on Linux
	@if [ "$(shell uname -s)" != "Darwin" ] && echo "$(CONTAINER_ENGINE)" | grep -q "podman"; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
	fi; \
	$(KIND) delete cluster --name $(KIND_CLUSTER_NAME)
