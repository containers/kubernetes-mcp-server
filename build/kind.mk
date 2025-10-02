# Kind cluster management

KIND_CLUSTER_NAME ?= kubernetes-mcp-server

# Detect container engine (docker or podman)
CONTAINER_ENGINE ?= $(shell command -v docker 2>/dev/null || command -v podman 2>/dev/null)

.PHONY: kind-create-cluster
kind-create-cluster: kind ## Create the kind cluster for development
	@# Set KIND provider for podman on Linux
	@if [ "$(shell uname -s)" != "Darwin" ] && echo "$(CONTAINER_ENGINE)" | grep -q "podman"; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
	fi; \
	if $(KIND) get clusters 2>/dev/null | grep -q "^$(KIND_CLUSTER_NAME)$$"; then \
		echo "Kind cluster '$(KIND_CLUSTER_NAME)' already exists, skipping creation"; \
	else \
		echo "Creating Kind cluster '$(KIND_CLUSTER_NAME)'..."; \
		$(KIND) create cluster --name $(KIND_CLUSTER_NAME) --config config/kind/cluster.yaml; \
	fi

.PHONY: kind-delete-cluster
kind-delete-cluster: kind ## Delete the kind cluster
	@# Set KIND provider for podman on Linux
	@if [ "$(shell uname -s)" != "Darwin" ] && echo "$(CONTAINER_ENGINE)" | grep -q "podman"; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
	fi; \
	$(KIND) delete cluster --name $(KIND_CLUSTER_NAME)
