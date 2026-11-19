##@ Config Multicluster

KIND ?= kind
KIND_CLUSTERS ?= east west
KIND_IMAGE ?= kindest/node:v1.31.0

.PHONY: setup-config-multicluster
setup-config-multicluster: kubectl ## Setup multicluster Kind for config evals
	@set -e; \
	if command -v podman >/dev/null 2>&1 && ! command -v docker >/dev/null 2>&1; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
		echo "Using Podman as container runtime..."; \
	fi; \
	echo "Creating Kind clusters: $(KIND_CLUSTERS)..."; \
	TMPDIR=$$(mktemp -d); \
	trap 'rm -rf $$TMPDIR' EXIT; \
	for cluster in $(KIND_CLUSTERS); do \
		echo "  Creating kind-$$cluster..."; \
		$(KIND) create cluster --name $$cluster \
			--image $(KIND_IMAGE) \
			--kubeconfig $$TMPDIR/kubeconfig-$$cluster \
			--wait 120s; \
	done; \
	echo "Merging kubeconfigs..."; \
	mkdir -p _output; \
	KUBECONFIG=$$(find $$TMPDIR -name 'kubeconfig-*' | paste -sd: -) \
	$(KUBECTL) config view --flatten > _output/kubeconfig-multicluster; \
	echo "✓ Kubeconfig merged to _output/kubeconfig-multicluster"; \
	echo ""; \
	echo "Available contexts:"; \
	KUBECONFIG=_output/kubeconfig-multicluster $(KUBECTL) config get-contexts; \
	echo ""; \
	echo "Verifying cluster connectivity..."; \
	for ctx in kind-east kind-west; do \
		echo "  Testing $$ctx..."; \
		KUBECONFIG=_output/kubeconfig-multicluster $(KUBECTL) --context $$ctx cluster-info | head -1; \
	done; \
	echo "✓ Multicluster setup complete"

.PHONY: config-multicluster-cleanup
config-multicluster-cleanup: ## Delete config multicluster Kind clusters
	@if command -v podman >/dev/null 2>&1 && ! command -v docker >/dev/null 2>&1; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
	fi; \
	echo "Deleting Kind clusters..."; \
	for cluster in $(KIND_CLUSTERS); do \
		echo "  Deleting kind-$$cluster..."; \
		$(KIND) delete cluster --name $$cluster 2>/dev/null || true; \
	done; \
	rm -f _output/kubeconfig-multicluster; \
	echo "✓ Cleanup complete"
