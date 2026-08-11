# KubeVirt installation and management

# KubeVirt version configuration
KUBEVIRT_VERSION ?= v1.8.2
CDI_VERSION ?= v1.65.0
MULTUS_VERSION ?= v4.2.4

# Detect if we're using a released version or main/latest
KUBEVIRT_RELEASE_URL = https://github.com/kubevirt/kubevirt/releases/download/$(KUBEVIRT_VERSION)
CDI_RELEASE_URL = https://github.com/kubevirt/containerized-data-importer/releases/download/$(CDI_VERSION)
MULTUS_RELEASE_URL = https://raw.githubusercontent.com/k8snetworkplumbingwg/multus-cni/$(MULTUS_VERSION)/deployments

##@ KubeVirt

.PHONY: kubevirt-install
kubevirt-install: kubectl ## Install KubeVirt, CDI, and Multus on the cluster
	@echo "========================================="
	@echo "Installing KubeVirt $(KUBEVIRT_VERSION)"
	@echo "========================================="
	@echo ""
	@echo "Installing KubeVirt operator..."
	@$(KUBECTL) apply -f $(KUBEVIRT_RELEASE_URL)/kubevirt-operator.yaml
	@echo ""
	@echo "Installing KubeVirt CR..."
	@$(KUBECTL) apply -f $(KUBEVIRT_RELEASE_URL)/kubevirt-cr.yaml
	@echo ""
	@echo "Waiting for KubeVirt to become ready (this can take a few minutes)..."
	@$(KUBECTL) -n kubevirt wait kv kubevirt --for condition=Available --timeout=15m
	@echo "✅ KubeVirt is ready"
	@echo ""
	@echo "Enabling Snapshot feature gate and software emulation..."
	@$(KUBECTL) patch kubevirt kubevirt -n kubevirt --type=merge -p '{"spec":{"configuration":{"developerConfiguration":{"featureGates":["Snapshot"],"useEmulation":true}}}}'
	@echo "✅ Snapshot feature gate and software emulation enabled"
	@echo ""
	@echo "Installing CDI (Containerized Data Importer) $(CDI_VERSION)..."
	@$(KUBECTL) apply -f $(CDI_RELEASE_URL)/cdi-operator.yaml
	@$(KUBECTL) apply -f $(CDI_RELEASE_URL)/cdi-cr.yaml
	@echo ""
	@echo "Waiting for CDI to become ready..."
	@$(KUBECTL) wait --for=condition=Available cdi/cdi -n cdi --timeout=5m
	@echo "✅ CDI is ready"
	@echo ""
	@echo "Installing Multus CNI $(MULTUS_VERSION)..."
	@$(KUBECTL) apply -f $(MULTUS_RELEASE_URL)/multus-daemonset-thick.yml
	@echo ""
	@echo "Waiting for Multus daemonset to be ready..."
	@$(KUBECTL) -n kube-system rollout status daemonset/kube-multus-ds --timeout=5m
	@echo "✅ Multus is ready"
	@echo ""
	@echo "========================================="
	@echo "KubeVirt Installation Complete"
	@echo "========================================="
	@echo ""
	@echo "KubeVirt version: $(KUBEVIRT_VERSION)"
	@echo "CDI version: $(CDI_VERSION)"
	@echo "Multus version: $(MULTUS_VERSION)"
	@echo ""
	@echo "Verify installation with:"
	@echo "  kubectl get kubevirt -n kubevirt"
	@echo "  kubectl get cdi -n cdi"
	@echo "  kubectl get pods -n kube-system -l app=multus"
	@echo ""

.PHONY: kubevirt-uninstall
kubevirt-uninstall: kubectl ## Uninstall KubeVirt, CDI, and Multus from the cluster
	@echo "Uninstalling KubeVirt, CDI, and Multus..."
	@$(KUBECTL) delete -f $(KUBEVIRT_RELEASE_URL)/kubevirt-cr.yaml --ignore-not-found
	@$(KUBECTL) delete -f $(KUBEVIRT_RELEASE_URL)/kubevirt-operator.yaml --ignore-not-found
	@$(KUBECTL) delete -f $(CDI_RELEASE_URL)/cdi-cr.yaml --ignore-not-found
	@$(KUBECTL) delete -f $(CDI_RELEASE_URL)/cdi-operator.yaml --ignore-not-found
	@$(KUBECTL) delete -f $(MULTUS_RELEASE_URL)/multus-daemonset-thick.yml --ignore-not-found
	@echo "✅ KubeVirt, CDI, and Multus uninstalled"

.PHONY: kubevirt-status
kubevirt-status: kubectl ## Show KubeVirt, CDI, and Multus status
	@echo "========================================="
	@echo "KubeVirt Status"
	@echo "========================================="
	@echo ""
	@echo "KubeVirt:"
	@$(KUBECTL) get kubevirt -n kubevirt -o wide || { echo "KubeVirt not installed"; exit 1; }
	@echo ""
	@echo "CDI:"
	@$(KUBECTL) get cdi -n cdi -o wide || { echo "CDI not installed"; exit 1; }
	@echo ""
	@echo "Multus Pods:"
	@$(KUBECTL) get pods -n kube-system -l app=multus || echo "Multus not installed"
	@echo ""
	@echo "KubeVirt Pods:"
	@$(KUBECTL) get pods -n kubevirt
	@echo ""
	@echo "CDI Pods:"
	@$(KUBECTL) get pods -n cdi
	@echo ""
	@echo "VirtualMachines (all namespaces):"
	@$(KUBECTL) get virtualmachines --all-namespaces || echo "No VirtualMachines found"
	@echo ""
	@echo "VirtualMachineInstances (all namespaces):"
	@$(KUBECTL) get virtualmachineinstances --all-namespaces || echo "No VirtualMachineInstances found"
	@echo ""
	@echo "Network Attachment Definitions (all namespaces):"
	@$(KUBECTL) get network-attachment-definitions --all-namespaces || echo "No NetworkAttachmentDefinitions found"
	@echo ""

##@ Multus CNI

.PHONY: multus-install
multus-install: kubectl ## Install Multus CNI on the cluster
	@echo "========================================="
	@echo "Installing Multus CNI $(MULTUS_VERSION)"
	@echo "========================================="
	@echo ""
	@echo "Installing Multus thick plugin daemonset..."
	@$(KUBECTL) apply -f $(MULTUS_RELEASE_URL)/multus-daemonset-thick.yml
	@echo ""
	@echo "Waiting for Multus daemonset to be ready..."
	@$(KUBECTL) -n kube-system rollout status daemonset/kube-multus-ds --timeout=5m
	@echo "Multus is ready"
	@echo ""
	@echo "========================================="
	@echo "Multus Installation Complete"
	@echo "========================================="
	@echo ""
	@echo "Multus version: $(MULTUS_VERSION)"
	@echo ""
	@echo "Verify installation with:"
	@echo "  kubectl get pods -n kube-system -l app=multus"
	@echo "  kubectl get network-attachment-definitions --all-namespaces"
	@echo ""

.PHONY: multus-uninstall
multus-uninstall: kubectl ## Uninstall Multus CNI from the cluster
	@echo "Uninstalling Multus CNI..."
	@$(KUBECTL) delete -f $(MULTUS_RELEASE_URL)/multus-daemonset-thick.yml --ignore-not-found
	@echo "Multus CNI uninstalled"

.PHONY: multus-status
multus-status: kubectl ## Show Multus CNI status
	@echo "========================================="
	@echo "Multus CNI Status"
	@echo "========================================="
	@echo ""
	@echo "Multus Pods:"
	@$(KUBECTL) get pods -n kube-system -l app=multus || echo "Multus not installed"
	@echo ""
	@echo "Network Attachment Definitions (all namespaces):"
	@$(KUBECTL) get network-attachment-definitions --all-namespaces || echo "No NetworkAttachmentDefinitions found"
	@echo ""
