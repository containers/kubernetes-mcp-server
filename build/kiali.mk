##@ Istio/Kiali

ISTIOCTL = $(shell pwd)/_output/tools/bin/istioctl
ISTIO_ADDONS_DIR = $(shell pwd)/_output/istio-addons
ISTIO_VERSION = 1.30.1
KIALI_VERSION = v2.27
# Release version without patch (e.g. 1.28.0 -> 1.28)

# Download and install istioctl (also copies samples/addons for install-istio)
.PHONY: istioctl
istioctl:
	@{ \
		set -e ;\
		echo "Installing istioctl to $(ISTIOCTL)..." ;\
		mkdir -p $(shell dirname $(ISTIOCTL)) ;\
		TMPDIR=$$(mktemp -d) ;\
		cd $$TMPDIR ;\
		curl -L https://istio.io/downloadIstio | ISTIO_VERSION=$(ISTIO_VERSION) sh - ; \
		ISTIODIR=$$(ls -d istio-* | head -n1) ;\
		cp $$ISTIODIR/bin/istioctl $(ISTIOCTL) ;\
		mkdir -p $(ISTIO_ADDONS_DIR) ;\
		cp $$ISTIODIR/samples/addons/jaeger.yaml $$ISTIODIR/samples/addons/prometheus.yaml $$ISTIODIR/samples/addons/kiali.yaml $(ISTIO_ADDONS_DIR)/ ;\
		sed -i '/ tracing:/,/ identity:/ { s/ enabled: false/ enabled: true\n        in_cluster_url: "http:\/\/tracing.istio-system:16685\/jaeger"\n        use_grpc: true/ }' $(ISTIO_ADDONS_DIR)/kiali.yaml ;\
		cd - >/dev/null ;\
		rm -rf $$TMPDIR ;\
	}

# Install Gateway API CRDs required for Gateway API eval tasks and Kiali AI tools
.PHONY: install-gateway-api-crds
install-gateway-api-crds:
	@evals/tasks/kiali/scripts/ensure_gateway_api_crds.sh

# Install Istio (demo profile) and enable sidecar injection in default namespace
.PHONY: install-istio
install-istio: istioctl kubectl
	$(ISTIOCTL) install --set profile=demo \
		--set meshConfig.defaultConfig.tracing.zipkin.address=zipkin.istio-system:9411 \
		-y
	$(KUBECTL) apply -f $(ISTIO_ADDONS_DIR)/prometheus.yaml -n istio-system
	$(KUBECTL) apply -f $(ISTIO_ADDONS_DIR)/kiali.yaml -n istio-system
	$(KUBECTL) apply -f $(ISTIO_ADDONS_DIR)/jaeger.yaml -n istio-system
	$(KUBECTL) wait --namespace istio-system --for=condition=available deployment/kiali --timeout=300s
	$(KUBECTL) wait --namespace istio-system --for=condition=available deployment/prometheus --timeout=300s
	$(KUBECTL) wait --for=condition=Ready pod --all -n istio-system --timeout=300s
	$(KUBECTL) rollout status deployment/kiali -n istio-system
	$(KUBECTL) label namespace default istio-injection=enabled --overwrite
	$(KUBECTL) wait --for=condition=Ready pod --all -n istio-system --timeout=300s
	
# Install Bookinfo demo
.PHONY: install-bookinfo-demo
install-bookinfo-demo: kubectl
	$(KUBECTL) create ns bookinfo
	$(KUBECTL) label namespace bookinfo istio-discovery=enabled istio.io/rev=default istio-injection=enabled
	$(KUBECTL) apply -f https://raw.githubusercontent.com/openshift-service-mesh/istio/refs/heads/master/samples/bookinfo/platform/kube/bookinfo.yaml -n bookinfo
	$(KUBECTL) apply -n bookinfo -f https://raw.githubusercontent.com/istio-ecosystem/sail-operator/main/chart/samples/ingress-gateway.yaml
	$(KUBECTL) apply -f https://raw.githubusercontent.com/openshift-service-mesh/istio/refs/heads/master/samples/bookinfo/networking/bookinfo-gateway.yaml -n bookinfo
	$(KUBECTL) wait --for=condition=Ready pod --all -n bookinfo --timeout=300s

# Update Kiali version
.PHONY: update-kiali-version
update-kiali-version: kubectl
	@echo "Updating Kiali version to $(KIALI_VERSION)..."
	@$(KUBECTL) patch deployment kiali -n istio-system -p '{"spec":{"template":{"spec":{"containers":[{"name":"kiali","image":"quay.io/kiali/kiali_mcp:$(KIALI_VERSION)"}]}}}}'
	@$(KUBECTL) delete pod -l app=kiali -n istio-system
	@$(KUBECTL) wait --for=condition=available deployment/kiali -n istio-system --timeout=300s

# Expose Bookinfo demo
.PHONY: expose-bookinfo-demo
expose-bookinfo-demo: kubectl
	@echo "Exposing Bookinfo demo..."
	@$(KUBECTL) port-forward svc/istio-ingressgateway 20002:80 -n bookinfo >/dev/null 2>&1 & \
	while true; do curl -s -o /dev/null http://localhost:20002/productpage; sleep 1; done & \
	echo "Bookinfo demo is being exposed on http://localhost:20002/productpage and generator is running"

# Expose Kiali service
.PHONY: expose-kiali
expose-kiali: kubectl
	@echo "Exposing Kiali service..."
	$(KUBECTL) -n istio-system port-forward svc/kiali 20001:20001 & \
	timeout 30s bash -c 'until curl -s localhost:20001; do sleep 1; done' && \
	echo "Kiali is being exposed on http://localhost:20001"

.PHONY: setup-kiali
setup-kiali: install-istio install-gateway-api-crds update-kiali-version install-bookinfo-demo expose-kiali expose-bookinfo-demo ## Setup Kiali
