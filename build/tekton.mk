# Tekton Pipelines installation and management

TEKTON_VERSION ?= v1.10.0

TEKTON_RELEASE_URL = https://infra.tekton.dev/tekton-releases/pipeline/previous/$(TEKTON_VERSION)/release.yaml

##@ Tekton Pipelines

.PHONY: tekton-install
tekton-install: kubectl ## Install Tekton Pipelines on the cluster
	@echo "========================================="
	@echo "Installing Tekton Pipelines $(TEKTON_VERSION)"
	@echo "========================================="
	@echo ""
	@echo "Installing Tekton Pipelines..."
	@$(KUBECTL) apply -f $(TEKTON_RELEASE_URL)
	@echo ""
	@echo "Waiting for Tekton Pipelines controller to be ready..."
	@$(KUBECTL) wait --for=condition=available deployment/tekton-pipelines-controller \
		-n tekton-pipelines --timeout=5m
	@echo "Waiting for Tekton Pipelines webhook to be ready..."
	@$(KUBECTL) wait --for=condition=available deployment/tekton-pipelines-webhook \
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
tekton-uninstall: kubectl ## Uninstall Tekton Pipelines from the cluster
	@echo "Uninstalling Tekton Pipelines $(TEKTON_VERSION)..."
	@$(KUBECTL) delete -f $(TEKTON_RELEASE_URL) --ignore-not-found
	@echo "✅ Tekton Pipelines uninstalled"

.PHONY: tekton-status
tekton-status: kubectl ## Show Tekton Pipelines status
	@echo "========================================="
	@echo "Tekton Pipelines Status"
	@echo "========================================="
	@echo ""
	@echo "Tekton Pods:"
	@$(KUBECTL) get pods -n tekton-pipelines 2>/dev/null || echo "Tekton Pipelines not installed"
	@echo ""
	@if $(KUBECTL) get crd pipelines.tekton.dev > /dev/null 2>&1; then \
		echo "Pipelines (all namespaces):"; \
		$(KUBECTL) get pipelines --all-namespaces 2>/dev/null || echo "No Pipelines found"; \
		echo ""; \
		echo "PipelineRuns (all namespaces):"; \
		$(KUBECTL) get pipelineruns --all-namespaces 2>/dev/null || echo "No PipelineRuns found"; \
		echo ""; \
		echo "Tasks (all namespaces):"; \
		$(KUBECTL) get tasks --all-namespaces 2>/dev/null || echo "No Tasks found"; \
		echo ""; \
		echo "TaskRuns (all namespaces):"; \
		$(KUBECTL) get taskruns --all-namespaces 2>/dev/null || echo "No TaskRuns found"; \
		echo ""; \
	else \
		echo "Tekton CRDs not installed — run: make tekton-install"; \
	fi
