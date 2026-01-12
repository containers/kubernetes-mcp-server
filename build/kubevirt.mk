# KubeVirt installation and management

# KubeVirt version configuration
KUBEVIRT_VERSION ?= v1.7.0
CDI_VERSION ?= v1.64.0

# Detect if we're using a released version or main/latest
KUBEVIRT_RELEASE_URL = https://github.com/kubevirt/kubevirt/releases/download/$(KUBEVIRT_VERSION)
CDI_RELEASE_URL = https://github.com/kubevirt/containerized-data-importer/releases/download/$(CDI_VERSION)

##@ KubeVirt

.PHONY: kubevirt-install
kubevirt-install: ## Install KubeVirt and CDI on the cluster
	@echo "========================================="
	@echo "Installing KubeVirt $(KUBEVIRT_VERSION)"
	@echo "========================================="
	@echo ""
	@echo "Installing KubeVirt operator..."
	@kubectl apply -f $(KUBEVIRT_RELEASE_URL)/kubevirt-operator.yaml
	@echo ""
	@echo "Installing KubeVirt CR..."
	@kubectl apply -f $(KUBEVIRT_RELEASE_URL)/kubevirt-cr.yaml
	@echo ""
	@echo "Waiting for KubeVirt to become ready (this can take a few minutes)..."
	@kubectl -n kubevirt wait kv kubevirt --for condition=Available --timeout=15m
	@echo "✅ KubeVirt is ready"
	@echo ""
	@echo "Installing CDI (Containerized Data Importer) $(CDI_VERSION)..."
	@kubectl apply -f $(CDI_RELEASE_URL)/cdi-operator.yaml
	@kubectl apply -f $(CDI_RELEASE_URL)/cdi-cr.yaml
	@echo ""
	@echo "Waiting for CDI to become ready..."
	@kubectl wait --for=condition=Available cdi/cdi -n cdi --timeout=5m
	@echo "✅ CDI is ready"
	@echo ""
	@echo "========================================="
	@echo "KubeVirt Installation Complete"
	@echo "========================================="
	@echo ""
	@echo "KubeVirt version: $(KUBEVIRT_VERSION)"
	@echo "CDI version: $(CDI_VERSION)"
	@echo ""
	@echo "Verify installation with:"
	@echo "  kubectl get kubevirt -n kubevirt"
	@echo "  kubectl get cdi -n cdi"
	@echo ""

.PHONY: kubevirt-uninstall
kubevirt-uninstall: ## Uninstall KubeVirt and CDI from the cluster
	@echo "Uninstalling KubeVirt and CDI..."
	@kubectl delete -f $(KUBEVIRT_RELEASE_URL)/kubevirt-cr.yaml --ignore-not-found
	@kubectl delete -f $(KUBEVIRT_RELEASE_URL)/kubevirt-operator.yaml --ignore-not-found
	@kubectl delete -f $(CDI_RELEASE_URL)/cdi-cr.yaml --ignore-not-found
	@kubectl delete -f $(CDI_RELEASE_URL)/cdi-operator.yaml --ignore-not-found
	@echo "✅ KubeVirt and CDI uninstalled"

.PHONY: kubevirt-status
kubevirt-status: ## Show KubeVirt and CDI status
	@echo "========================================="
	@echo "KubeVirt Status"
	@echo "========================================="
	@echo ""
	@echo "KubeVirt:"
	@kubectl get kubevirt -n kubevirt -o wide || { echo "KubeVirt not installed"; exit 1; }
	@echo ""
	@echo "CDI:"
	@kubectl get cdi -n cdi -o wide || { echo "CDI not installed"; exit 1; }
	@echo ""
	@echo "KubeVirt Pods:"
	@kubectl get pods -n kubevirt
	@echo ""
	@echo "CDI Pods:"
	@kubectl get pods -n cdi
	@echo ""
	@echo "VirtualMachines (all namespaces):"
	@kubectl get virtualmachines --all-namespaces || echo "No VirtualMachines found"
	@echo ""
	@echo "VirtualMachineInstances (all namespaces):"
	@kubectl get virtualmachineinstances --all-namespaces || echo "No VirtualMachineInstances found"
	@echo ""
