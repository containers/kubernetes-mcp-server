##@ Tool Binaries

KUBECTL = $(shell pwd)/_output/tools/bin/kubectl
KUBECTL_VERSION ?= v1.36.2

HELM = $(shell pwd)/_output/tools/bin/helm
HELM_VERSION ?= v3.21.2

MINIKUBE = $(shell pwd)/_output/tools/bin/minikube
MINIKUBE_VERSION ?= v1.38.1

# Download and install kubectl if not already installed
.PHONY: kubectl
kubectl:
	@[ -f $(KUBECTL) ] || { \
		set -e ;\
		echo "Installing kubectl $(KUBECTL_VERSION) to $(KUBECTL)..." ;\
		mkdir -p $$(dirname $(KUBECTL)) ;\
		OS=$$(uname -s | tr '[:upper:]' '[:lower:]') ;\
		ARCH=$$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') ;\
		curl -fL https://dl.k8s.io/release/$(KUBECTL_VERSION)/bin/$${OS}/$${ARCH}/kubectl -o $(KUBECTL) ;\
		chmod +x $(KUBECTL) ;\
	}

# Download and install helm if not already installed
.PHONY: helm
helm:
	@[ -f $(HELM) ] || { \
		set -e ;\
		echo "Installing helm $(HELM_VERSION) to $(HELM)..." ;\
		mkdir -p $$(dirname $(HELM)) ;\
		TMPDIR=$$(mktemp -d) ;\
		OS=$$(uname -s | tr '[:upper:]' '[:lower:]') ;\
		ARCH=$$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') ;\
		curl -fL https://get.helm.sh/helm-$(HELM_VERSION)-$${OS}-$${ARCH}.tar.gz | tar xz -C $$TMPDIR ;\
		mv $$TMPDIR/$${OS}-$${ARCH}/helm $(HELM) ;\
		rm -rf $$TMPDIR ;\
	}

# Download and install minikube if not already installed
.PHONY: minikube
minikube:
	@[ -f $(MINIKUBE) ] || { \
		set -e ;\
		echo "Installing minikube $(MINIKUBE_VERSION) to $(MINIKUBE)..." ;\
		mkdir -p $$(dirname $(MINIKUBE)) ;\
		OS=$$(uname -s | tr '[:upper:]' '[:lower:]') ;\
		ARCH=$$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') ;\
		curl -fL https://storage.googleapis.com/minikube/releases/$(MINIKUBE_VERSION)/minikube-$${OS}-$${ARCH} -o $(MINIKUBE) ;\
		chmod +x $(MINIKUBE) ;\
	}
