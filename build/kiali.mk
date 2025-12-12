# Kind cluster management

KIND_CLUSTER_NAME ?= kubernetes-mcp-server

# Detect container engine (docker or podman)
CONTAINER_ENGINE ?= $(shell command -v docker 2>/dev/null || command -v podman 2>/dev/null)

##@ Istio

ISTIOCTL = _output/bin/istioctl

$(ISTIOCTL):
	@mkdir -p _output/bin
	@echo "Downloading istioctl..."
	@set -e; \
	TMPDIR=$$(mktemp -d); \
	cd $$TMPDIR; \
	curl -sL https://istio.io/downloadIstio | sh -; \
	ISTIODIR=$$(ls -d istio-* | head -n1); \
	cp $$ISTIODIR/bin/istioctl $(PWD)/$(ISTIOCTL); \
	cd - >/dev/null; \
	rm -rf $$TMPDIR; \
	echo "istioctl installed at $(ISTIOCTL)"

.PHONY: istioctl
istioctl: $(ISTIOCTL) ## Ensure istioctl is installed to _output/bin/

.PHONY: install-istio
install-istio: istioctl ## Install Istio (demo profile) and enable sidecar injection in default ns
	./$(ISTIOCTL) install --set profile=demo -y
	kubectl label namespace default istio-injection=enabled --overwrite

.PHONY: install-istio-addons
install-istio-addons: install-istio ## Install Istio addons
	kubectl apply -f dev/config/istio/prometheus.yaml -n istio-system
	kubectl apply -f dev/config/istio/kiali.yaml -n istio-system
	kubectl wait --namespace istio-system --for=condition=available deployment/kiali --timeout=300s
	kubectl wait --namespace istio-system --for=condition=available deployment/prometheus --timeout=300s

.PHONY: install-bookinfo-demo
install-bookinfo-demo:  ## Install Bookinfo demo
	kubectl create ns bookinfo
	kubectl label namespace bookinfo istio-discovery=enabled istio.io/rev=default istio-injection=enabled
	kubectl apply -f dev/config/istio/bookinfo.yaml -n bookinfo
	kubectl wait --for=condition=Ready pod --all -n bookinfo --timeout=300s

.PHONY: setup-kiali
setup-kiali: install-istio-addons install-bookinfo-demo ## Setup Kiali