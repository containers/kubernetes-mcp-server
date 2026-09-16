# ArgoCD CRDs installation for eval tests
# Installs only the CRDs (no operator/controller) so that eval tasks can
# create ArgoCD resources without requiring a full ArgoCD deployment.

ARGOCD_VERSION ?= v2.14.12
ARGOCD_OPERATOR_VERSION ?= v0.15.0

ARGOCD_CRDS_URL = https://raw.githubusercontent.com/argoproj/argo-cd/$(ARGOCD_VERSION)/manifests/crds
ARGOCD_OPERATOR_CRDS_URL = https://raw.githubusercontent.com/argoproj-labs/argocd-operator/$(ARGOCD_OPERATOR_VERSION)/config/crd/bases

##@ ArgoCD

.PHONY: argocd-install
argocd-install: ## Install ArgoCD CRDs on the cluster (CRDs only, no operator)
	@echo "========================================="
	@echo "Installing ArgoCD CRDs"
	@echo "========================================="
	@echo ""
	@echo "Installing ArgoCD core CRDs ($(ARGOCD_VERSION))..."
	@kubectl apply -f $(ARGOCD_CRDS_URL)/application-crd.yaml
	@kubectl apply -f $(ARGOCD_CRDS_URL)/appproject-crd.yaml
	@kubectl apply -f $(ARGOCD_CRDS_URL)/applicationset-crd.yaml
	@echo ""
	@echo "Installing ArgoCD Operator CRD ($(ARGOCD_OPERATOR_VERSION))..."
	@kubectl apply -f $(ARGOCD_OPERATOR_CRDS_URL)/argoproj.io_argocds.yaml
	@echo ""
	@echo "Waiting for CRDs to be established..."
	@kubectl wait --for=condition=Established crd/applications.argoproj.io --timeout=60s
	@kubectl wait --for=condition=Established crd/appprojects.argoproj.io --timeout=60s
	@kubectl wait --for=condition=Established crd/applicationsets.argoproj.io --timeout=60s
	@kubectl wait --for=condition=Established crd/argocds.argoproj.io --timeout=60s
	@echo ""
	@echo "========================================="
	@echo "ArgoCD CRDs Installation Complete"
	@echo "========================================="
	@echo ""
	@echo "ArgoCD version: $(ARGOCD_VERSION)"
	@echo "ArgoCD Operator version: $(ARGOCD_OPERATOR_VERSION)"
	@echo ""
	@echo "Verify installation with:"
	@echo "  make argocd-status"
	@echo ""

.PHONY: argocd-uninstall
argocd-uninstall: ## Uninstall ArgoCD CRDs from the cluster
	@echo "Uninstalling ArgoCD CRDs..."
	@kubectl delete -f $(ARGOCD_OPERATOR_CRDS_URL)/argoproj.io_argocds.yaml --ignore-not-found
	@kubectl delete -f $(ARGOCD_CRDS_URL)/applicationset-crd.yaml --ignore-not-found
	@kubectl delete -f $(ARGOCD_CRDS_URL)/appproject-crd.yaml --ignore-not-found
	@kubectl delete -f $(ARGOCD_CRDS_URL)/application-crd.yaml --ignore-not-found
	@echo "ArgoCD CRDs uninstalled"

.PHONY: argocd-status
argocd-status: ## Show ArgoCD CRDs status
	@echo "========================================="
	@echo "ArgoCD CRDs Status"
	@echo "========================================="
	@echo ""
	@echo "Installed CRDs:"
	@kubectl get crd applications.argoproj.io appprojects.argoproj.io applicationsets.argoproj.io argocds.argoproj.io 2>/dev/null || echo "ArgoCD CRDs not installed — run: make argocd-install"
	@echo ""
	@if kubectl get crd applications.argoproj.io > /dev/null 2>&1; then \
		echo "Applications (all namespaces):"; \
		kubectl get applications.argoproj.io --all-namespaces 2>/dev/null || echo "No Applications found"; \
		echo ""; \
		echo "AppProjects (all namespaces):"; \
		kubectl get appprojects.argoproj.io --all-namespaces 2>/dev/null || echo "No AppProjects found"; \
		echo ""; \
		echo "ApplicationSets (all namespaces):"; \
		kubectl get applicationsets.argoproj.io --all-namespaces 2>/dev/null || echo "No ApplicationSets found"; \
		echo ""; \
		echo "ArgoCD instances (all namespaces):"; \
		kubectl get argocds.argoproj.io --all-namespaces 2>/dev/null || echo "No ArgoCD instances found"; \
		echo ""; \
	fi
