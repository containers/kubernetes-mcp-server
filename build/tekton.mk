# Tekton Pipelines installation and management

TEKTON_VERSION ?= v1.10.0

TEKTON_RELEASE_URL = https://infra.tekton.dev/tekton-releases/pipeline/previous/$(TEKTON_VERSION)/release.yaml

##@ Tekton Pipelines

.PHONY: tekton-install
tekton-install: ## Install Tekton Pipelines on the cluster
	@echo "========================================="
	@echo "Installing Tekton Pipelines $(TEKTON_VERSION)"
	@echo "========================================="
	@echo ""
	@echo "Installing Tekton Pipelines..."
	@kubectl apply -f $(TEKTON_RELEASE_URL)
	@echo ""
	@echo "Waiting for Tekton Pipelines controller to be ready..."
	@kubectl wait --for=condition=available deployment/tekton-pipelines-controller \
		-n tekton-pipelines --timeout=5m
	@echo "Waiting for Tekton Pipelines webhook to be ready..."
	@kubectl wait --for=condition=available deployment/tekton-pipelines-webhook \
		-n tekton-pipelines --timeout=5m
	@echo "✅ Tekton Pipelines is ready"
	@echo ""
	@echo "========================================="
	@echo "Tekton Pipelines Installation Complete"
	@echo "========================================="
	@echo ""
	@echo "Tekton Pipelines version: $(TEKTON_VERSION)"
	@echo ""
	@echo "Verify installation with:"
	@echo "  make tekton-status"
	@echo ""

.PHONY: tekton-uninstall
tekton-uninstall: ## Uninstall Tekton Pipelines from the cluster
	@echo "Uninstalling Tekton Pipelines $(TEKTON_VERSION)..."
	@kubectl delete -f $(TEKTON_RELEASE_URL) --ignore-not-found
	@echo "✅ Tekton Pipelines uninstalled"

.PHONY: tekton-status
tekton-status: ## Show Tekton Pipelines status
	@echo "========================================="
	@echo "Tekton Pipelines Status"
	@echo "========================================="
	@echo ""
	@echo "Tekton Pods:"
	@kubectl get pods -n tekton-pipelines 2>/dev/null || echo "Tekton Pipelines not installed"
	@echo ""
	@if kubectl get crd pipelines.tekton.dev > /dev/null 2>&1; then \
		echo "Pipelines (all namespaces):"; \
		kubectl get pipelines --all-namespaces 2>/dev/null || echo "No Pipelines found"; \
		echo ""; \
		echo "PipelineRuns (all namespaces):"; \
		kubectl get pipelineruns --all-namespaces 2>/dev/null || echo "No PipelineRuns found"; \
		echo ""; \
		echo "Tasks (all namespaces):"; \
		kubectl get tasks --all-namespaces 2>/dev/null || echo "No Tasks found"; \
		echo ""; \
		echo "TaskRuns (all namespaces):"; \
		kubectl get taskruns --all-namespaces 2>/dev/null || echo "No TaskRuns found"; \
		echo ""; \
	else \
		echo "Tekton CRDs not installed — run: make tekton-install"; \
	fi
