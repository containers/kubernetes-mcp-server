# Kuadrant MCP Gateway installation and management

# Pinned versions — update periodically
MCP_GATEWAY_VERSION ?= 0.7.1
GATEWAY_API_VERSION ?= v1.3.0
KUADRANT_ISTIO_VERSION ?= 1.30.1
GATEWAY_PUBLIC_HOST ?= mcp.gateway.local

##@ Kuadrant MCP Gateway

.PHONY: kuadrant-install-prerequisites
kuadrant-install-prerequisites: helm kubectl ## Install Gateway API CRDs and Istio (prerequisites for Kuadrant MCP Gateway)
	@echo "========================================="
	@echo "Installing Kuadrant MCP Gateway Prerequisites"
	@echo "========================================="
	@echo ""
	@echo "Installing Gateway API CRDs $(GATEWAY_API_VERSION)..."
	@$(KUBECTL) apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/$(GATEWAY_API_VERSION)/standard-install.yaml
	@echo "✅ Gateway API CRDs installed"
	@echo ""
	@echo "Adding Istio Helm repo..."
	@$(HELM) repo add istio https://istio-release.storage.googleapis.com/charts
	@$(HELM) repo update
	@echo ""
	@echo "Installing Istio base..."
	@$(HELM) upgrade --install istio-base istio/base -n istio-system --create-namespace --version $(KUADRANT_ISTIO_VERSION) --wait
	@echo "✅ Istio base installed"
	@echo ""
	@echo "Installing Istiod..."
	@$(HELM) upgrade --install istiod istio/istiod -n istio-system --version $(KUADRANT_ISTIO_VERSION) --wait
	@echo "✅ Istiod installed"
	@echo ""
	@echo "Creating gateway-system namespace..."
	@$(KUBECTL) create namespace gateway-system --dry-run=client -o yaml | $(KUBECTL) apply -f -
	@echo "✅ gateway-system namespace ready"
	@echo ""
	@echo "========================================="
	@echo "Kuadrant Prerequisites Installed"
	@echo "========================================="

.PHONY: kuadrant-install-gateway
kuadrant-install-gateway: helm kubectl ## Install Kuadrant MCP Gateway via Helm
	@echo "========================================="
	@echo "Installing Kuadrant MCP Gateway v$(MCP_GATEWAY_VERSION)"
	@echo "========================================="
	@echo ""
	$(HELM) upgrade --install mcp-gateway oci://ghcr.io/kuadrant/charts/mcp-gateway \
		--create-namespace \
		--namespace mcp-system \
		--version $(MCP_GATEWAY_VERSION) \
		--set broker.create=true \
		--set gateway.create=true \
		--set gateway.name=mcp-gateway \
		--set gateway.namespace=gateway-system \
		--set gateway.publicHost=$(GATEWAY_PUBLIC_HOST) \
		--set envoyFilter.name=mcp-gateway \
		--set mcpGatewayExtension.gatewayRef.name=mcp-gateway \
		--set mcpGatewayExtension.gatewayRef.namespace=gateway-system
	@echo ""
	@echo "Waiting for MCP Gateway deployments to be created..."
	@for deploy in mcp-gateway mcp-gateway-controller; do \
		until $(KUBECTL) get deployment "$$deploy" -n mcp-system 2>/dev/null; do \
			echo "  Waiting for deployment/$$deploy to exist..."; \
			sleep 5; \
		done; \
	done
	@echo "Waiting for MCP Gateway pods to be ready..."
	@$(KUBECTL) wait --for=condition=available --timeout=300s deployment/mcp-gateway -n mcp-system
	@$(KUBECTL) wait --for=condition=available --timeout=300s deployment/mcp-gateway-controller -n mcp-system
	@echo "✅ MCP Gateway deployments ready"
	@echo ""
	@echo "Waiting for Istio gateway pod..."
	@$(KUBECTL) wait --for=condition=ready --timeout=300s pod -l gateway.networking.k8s.io/gateway-name=mcp-gateway -n gateway-system
	@echo "✅ Istio gateway pod ready"
	@echo ""
	@echo "========================================="
	@echo "Kuadrant MCP Gateway Installed"
	@echo "========================================="
	@echo ""
	@echo "MCP Gateway version: $(MCP_GATEWAY_VERSION)"
	@echo ""
	@echo "Verify installation with:"
	@echo "  make kuadrant-status"

.PHONY: kuadrant-setup
kuadrant-setup: kuadrant-install-prerequisites kuadrant-install-gateway ## Install all Kuadrant MCP Gateway components (prerequisites + gateway)

.PHONY: kuadrant-uninstall
kuadrant-uninstall: helm ## Uninstall Kuadrant MCP Gateway
	@echo "Uninstalling Kuadrant MCP Gateway..."
	@$(HELM) uninstall mcp-gateway -n mcp-system --ignore-not-found || true
	@$(HELM) uninstall istiod -n istio-system --ignore-not-found || true
	@$(HELM) uninstall istio-base -n istio-system --ignore-not-found || true
	@echo "✅ Kuadrant MCP Gateway uninstalled"

.PHONY: kuadrant-status
kuadrant-status: kubectl ## Show Kuadrant MCP Gateway status
	@echo "========================================="
	@echo "Kuadrant MCP Gateway Status"
	@echo "========================================="
	@echo ""
	@echo "MCP System Pods:"
	@$(KUBECTL) get pods -n mcp-system 2>/dev/null || echo "  (namespace not found)"
	@echo ""
	@echo "Gateway System Pods:"
	@$(KUBECTL) get pods -n gateway-system 2>/dev/null || echo "  (namespace not found)"
	@echo ""
	@echo "Gateway:"
	@$(KUBECTL) get gateway -n gateway-system 2>/dev/null || echo "  (not found)"
	@echo ""
	@echo "MCPGatewayExtension:"
	@$(KUBECTL) get mcpgatewayextension -n mcp-system 2>/dev/null || echo "  (not found)"
	@echo ""
	@echo "MCPServerRegistrations (all namespaces):"
	@$(KUBECTL) get mcpserverregistration --all-namespaces 2>/dev/null || echo "  (not found)"
	@echo ""
