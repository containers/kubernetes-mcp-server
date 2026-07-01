# HyperConverged Cluster Operator (HCO) installation and management
#
# Mirrors the official deploy flow from:
#   https://github.com/kubevirt/hyperconverged-cluster-operator/blob/main/hack/deploy.sh
#
# HCO deploys and manages KubeVirt, CDI, Cluster Network Addons, and other
# components as a single operator. This is an ALTERNATIVE to the standalone
# KubeVirt install (kubevirt-install) — do not use both together.

# HCO version configuration
HCO_VERSION ?= v1.17.1
HCO_NAMESPACE = kubevirt-hyperconverged
HCO_RAW_URL = https://raw.githubusercontent.com/kubevirt/hyperconverged-cluster-operator/$(HCO_VERSION)/deploy
HCO_API_URL = https://api.github.com/repos/kubevirt/hyperconverged-cluster-operator/contents/deploy/crds?ref=$(HCO_VERSION)
HCO_CRDS = $(shell curl -sL $(HCO_API_URL) | grep -o '"name": *"[^"]*\.crd\.yaml"' | sed 's/"name": *"//;s/"//')
HCO_CRD_NAME = hyperconvergeds.hco.kubevirt.io

# cert-manager configuration
CERT_MANAGER_VERSION ?= v1.18.2
CERT_MANAGER_TIMEOUT ?= 120s

##@ HCO (HyperConverged Cluster Operator)

.PHONY: cert-manager-install
cert-manager-install: ## Install cert-manager (required by HCO webhooks)
	@echo "========================================="
	@echo "Installing cert-manager $(CERT_MANAGER_VERSION)"
	@echo "========================================="
	@echo ""
	@kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/$(CERT_MANAGER_VERSION)/cert-manager.yaml
	@echo ""
	@echo "Waiting for cert-manager deployments to become ready..."
	@kubectl wait --for=condition=Available --namespace=cert-manager deployment/cert-manager --timeout=$(CERT_MANAGER_TIMEOUT)
	@kubectl wait --for=condition=Available --namespace=cert-manager deployment/cert-manager-cainjector --timeout=$(CERT_MANAGER_TIMEOUT)
	@kubectl wait --for=condition=Available --namespace=cert-manager deployment/cert-manager-webhook --timeout=$(CERT_MANAGER_TIMEOUT)
	@echo ""
	@echo "Waiting for cert-manager webhook CA bundle to become valid..."
	@for i in $$(seq 1 60); do \
		if kubectl get validatingwebhookconfigurations cert-manager-webhook -o jsonpath='{.webhooks[0].clientConfig.caBundle}' 2>/dev/null | base64 -d | openssl x509 -noout 2>/dev/null; then \
			echo "cert-manager webhook CA bundle is valid"; \
			break; \
		fi; \
		if [ $$i -eq 60 ]; then \
			echo "Warning: cert-manager webhook CA bundle still not valid after 60 attempts"; \
		fi; \
		sleep 1; \
	done
	@echo "✅ cert-manager $(CERT_MANAGER_VERSION) is ready"

.PHONY: cert-manager-uninstall
cert-manager-uninstall: ## Uninstall cert-manager
	@echo "Uninstalling cert-manager..."
	@kubectl delete -f https://github.com/cert-manager/cert-manager/releases/download/$(CERT_MANAGER_VERSION)/cert-manager.yaml --ignore-not-found 2>/dev/null || true
	@echo "✅ cert-manager uninstalled"

.PHONY: hco-install
hco-install: cert-manager-install ## Install HCO on the cluster (manages KubeVirt, CDI, network addons)
	@echo ""
	@echo "========================================="
	@echo "Installing HCO $(HCO_VERSION)"
	@echo "========================================="
	@echo ""
	@echo "Creating namespace..."
	@kubectl create namespace $(HCO_NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	@echo ""
	@echo "Installing HCO CRDs..."
	@for crd in $(HCO_CRDS); do \
		kubectl apply --server-side -f $(HCO_RAW_URL)/crds/$${crd}; \
	done
	@echo ""
	@echo "Checking CRD status..."
	@sleep 20
	@if [ "$$(kubectl get crd $(HCO_CRD_NAME) -o=jsonpath='{.status.conditions[?(@.type=="NonStructuralSchema")].status}' 2>/dev/null)" = "True" ]; then \
		echo "Warning: HCO CRD reports NonStructuralSchema condition"; \
		kubectl get crd $(HCO_CRD_NAME) -o go-template='{{ range .status.conditions }}{{ .type }}{{ "\t" }}{{ .status }}{{ "\t" }}{{ .message }}{{ "\n" }}{{ end }}'; \
	fi
	@echo ""
	@echo "Installing HCO RBAC..."
	@kubectl apply -f $(HCO_RAW_URL)/cluster_role.yaml
	@kubectl apply -n $(HCO_NAMESPACE) -f $(HCO_RAW_URL)/service_account.yaml
	@kubectl apply -f $(HCO_RAW_URL)/cluster_role_binding.yaml
	@echo ""
	@echo "Installing HCO webhooks..."
	@kubectl apply -f $(HCO_RAW_URL)/webhooks.yaml
	@echo ""
	@echo "Installing HCO operator (excluding OpenShift-only components)..."
	@kubectl apply -n $(HCO_NAMESPACE) -l 'name!=ssp-operator,name!=hyperconverged-cluster-cli-download' -f $(HCO_RAW_URL)/operator.yaml
	@echo ""
	@echo "Enabling software emulation for Kind (no hardware virtualization)..."
	@kubectl set env deployment/hyperconverged-cluster-operator -n $(HCO_NAMESPACE) KVM_EMULATION=true
	@echo ""
	@echo "Waiting for HCO operator to become ready..."
	@kubectl wait deployment/hyperconverged-cluster-operator -n $(HCO_NAMESPACE) --for=condition=Available --timeout=10m
	@echo "✅ HCO operator is ready"
	@echo ""
	@echo "Waiting for HCO webhook to become ready..."
	@kubectl wait deployment/hyperconverged-cluster-webhook -n $(HCO_NAMESPACE) --for=condition=Available --timeout=5m
	@echo "✅ HCO webhook is ready"
	@echo ""
	@echo "Waiting for sub-operators to become ready..."
	@kubectl wait deployment/cdi-operator -n $(HCO_NAMESPACE) --for=condition=Available --timeout=9m
	@kubectl wait deployment/cluster-network-addons-operator -n $(HCO_NAMESPACE) --for=condition=Available --timeout=9m
	@kubectl wait deployment/kubevirt-migration-operator -n $(HCO_NAMESPACE) --for=condition=Available --timeout=9m 2>/dev/null || true
	@echo "✅ Sub-operators are ready"
	@echo ""
	@echo "Creating HyperConverged CR..."
	@kubectl apply -n $(HCO_NAMESPACE) -f $(HCO_RAW_URL)/hco.cr.yaml
	@echo ""
	@echo "Waiting for HCO to become available (this can take several minutes)..."
	@kubectl wait hyperconverged kubevirt-hyperconverged -n $(HCO_NAMESPACE) --for condition=Available --timeout=30m
	@echo "✅ HCO is ready"
	@echo ""
	@echo "Waiting for component deployments..."
	@kubectl wait deployment/cdi-apiserver -n $(HCO_NAMESPACE) --for=condition=Available --timeout=6m
	@kubectl wait deployment/cdi-deployment -n $(HCO_NAMESPACE) --for=condition=Available --timeout=6m
	@kubectl wait deployment/cdi-uploadproxy -n $(HCO_NAMESPACE) --for=condition=Available --timeout=6m
	@kubectl wait deployment/virt-api -n $(HCO_NAMESPACE) --for=condition=Available --timeout=6m
	@kubectl wait deployment/virt-controller -n $(HCO_NAMESPACE) --for=condition=Available --timeout=6m
	@echo "✅ All component deployments are ready"
	@echo ""
	@echo "========================================="
	@echo "HCO Installation Complete"
	@echo "========================================="
	@echo ""
	@echo "HCO version: $(HCO_VERSION)"
	@echo "Namespace: $(HCO_NAMESPACE)"
	@echo ""
	@echo "Managed components:"
	@echo "  - KubeVirt"
	@echo "  - CDI (Containerized Data Importer)"
	@echo "  - Cluster Network Addons"
	@echo ""
	@echo "Verify installation with:"
	@echo "  make hco-status"
	@echo ""

.PHONY: hco-uninstall
hco-uninstall: ## Uninstall HCO and all managed components from the cluster
	@echo "Uninstalling HCO..."
	@kubectl delete hyperconverged kubevirt-hyperconverged -n $(HCO_NAMESPACE) --ignore-not-found --timeout=5m
	@kubectl delete -n $(HCO_NAMESPACE) -l 'name!=ssp-operator,name!=hyperconverged-cluster-cli-download' -f $(HCO_RAW_URL)/operator.yaml --ignore-not-found 2>/dev/null || true
	@kubectl delete -f $(HCO_RAW_URL)/webhooks.yaml --ignore-not-found 2>/dev/null || true
	@kubectl delete -f $(HCO_RAW_URL)/cluster_role_binding.yaml --ignore-not-found 2>/dev/null || true
	@kubectl delete -n $(HCO_NAMESPACE) -f $(HCO_RAW_URL)/service_account.yaml --ignore-not-found 2>/dev/null || true
	@kubectl delete -f $(HCO_RAW_URL)/cluster_role.yaml --ignore-not-found 2>/dev/null || true
	@for crd in $(HCO_CRDS); do \
		kubectl delete -f $(HCO_RAW_URL)/crds/$${crd} --ignore-not-found 2>/dev/null || true; \
	done
	@kubectl delete namespace $(HCO_NAMESPACE) --ignore-not-found || true
	@echo "✅ HCO uninstalled"

.PHONY: hco-status
hco-status: ## Show HCO and managed component status
	@echo "========================================="
	@echo "HCO Status"
	@echo "========================================="
	@echo ""
	@echo "HyperConverged CR:"
	@kubectl get hyperconverged -n $(HCO_NAMESPACE) -o wide || echo "HCO not installed"
	@echo ""
	@echo "HyperConverged Conditions:"
	@kubectl get hyperconverged kubevirt-hyperconverged -n $(HCO_NAMESPACE) -o jsonpath='{range .status.conditions[*]}{.type}{"\t"}{.status}{"\t"}{.message}{"\n"}{end}' 2>/dev/null || echo "HCO not installed"
	@echo ""
	@echo "KubeVirt (managed by HCO):"
	@kubectl get kubevirt -A -o wide 2>/dev/null || echo "KubeVirt not found"
	@echo ""
	@echo "CDI (managed by HCO):"
	@kubectl get cdi -o wide 2>/dev/null || echo "CDI not found"
	@echo ""
	@echo "Network Addons (managed by HCO):"
	@kubectl get networkaddonsconfig -o wide 2>/dev/null || echo "Network Addons not found"
	@echo ""
	@echo "HCO Operator Pods:"
	@kubectl get pods -n $(HCO_NAMESPACE) -l name=hyperconverged-cluster-operator
	@echo ""
	@echo "KubeVirt Pods:"
	@kubectl get pods -n $(HCO_NAMESPACE) -l kubevirt.io 2>/dev/null || echo "No KubeVirt pods found"
	@echo ""
	@echo "CDI Pods:"
	@kubectl get pods -n $(HCO_NAMESPACE) -l cdi.kubevirt.io 2>/dev/null || echo "No CDI pods found"
	@echo ""
	@echo "Component Versions:"
	@kubectl get hyperconverged kubevirt-hyperconverged -n $(HCO_NAMESPACE) -o jsonpath='{range .status.versions[*]}  {.name}: {.version}{"\n"}{end}' 2>/dev/null || echo "  N/A"
	@echo ""
