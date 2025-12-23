##@ Istio/Kiali

ISTIOCTL = $(shell pwd)/_output/tools/bin/istioctl

# Download and install istioctl if not already installed
.PHONY: istioctl
istioctl:
	@[ -f $(ISTIOCTL) ] || { \
		set -e ;\
		echo "Installing istioctl to $(ISTIOCTL)..." ;\
		mkdir -p $(shell dirname $(ISTIOCTL)) ;\
		TMPDIR=$$(mktemp -d) ;\
		cd $$TMPDIR ;\
		curl -sL https://istio.io/downloadIstio | sh - ;\
		ISTIODIR=$$(ls -d istio-* | head -n1) ;\
		cp $$ISTIODIR/bin/istioctl $(ISTIOCTL) ;\
		cd - >/dev/null ;\
		rm -rf $$TMPDIR ;\
	}

# Install Istio (demo profile) and enable sidecar injection in default namespace
.PHONY: install-istio
install-istio: istioctl
	$(ISTIOCTL) install --set profile=demo -y
	kubectl label namespace default istio-injection=enabled --overwrite

# Install Istio addons
.PHONY: install-istio-addons
install-istio-addons: install-istio
	kubectl apply -f dev/config/istio/prometheus.yaml -n istio-system
	kubectl apply -f dev/config/istio/kiali.yaml -n istio-system
	kubectl wait --namespace istio-system --for=condition=available deployment/kiali --timeout=300s
	kubectl wait --namespace istio-system --for=condition=available deployment/prometheus --timeout=300s

# Install Bookinfo demo
.PHONY: install-bookinfo-demo
install-bookinfo-demo:
	kubectl create ns bookinfo
	kubectl label namespace bookinfo istio-discovery=enabled istio.io/rev=default istio-injection=enabled
	kubectl apply -f dev/config/istio/bookinfo.yaml -n bookinfo
	kubectl wait --for=condition=Ready pod --all -n bookinfo --timeout=300s

.PHONY: setup-kiali
setup-kiali: install-istio-addons install-bookinfo-demo ## Setup Kiali
