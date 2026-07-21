# Kind cluster management

KIND = $(shell pwd)/_output/tools/bin/kind
KIND_VERSION ?= v0.30.0

# Download and install kind if not already installed
.PHONY: kind
kind:
	@[ -f $(KIND) ] || { \
		set -e ;\
		echo "Installing kind to $(KIND)..." ;\
		mkdir -p $(shell dirname $(KIND)) ;\
		GOBIN=$(shell dirname $(KIND)) go install sigs.k8s.io/kind@$(KIND_VERSION) ;\
	}

KIND_CLUSTER_NAME ?= kubernetes-mcp-server

# Detect container engine (docker or podman)
CONTAINER_ENGINE ?= $(shell command -v docker 2>/dev/null || command -v podman 2>/dev/null)

.PHONY: kind-create-certs
kind-create-certs:
	@if [ ! -f _output/cert-manager-ca/ca.crt ]; then \
		echo "Creating placeholder CA certificate for bind mount..."; \
		./hack/generate-placeholder-ca.sh; \
	else \
		echo "✅ Placeholder CA already exists"; \
	fi

.PHONY: kind-create-cluster
kind-create-cluster: kind kind-create-certs
	@# Set KIND provider for podman on Linux
	@if [ "$(shell uname -s)" != "Darwin" ] && echo "$(CONTAINER_ENGINE)" | grep -q "podman"; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
	fi; \
	if $(KIND) get clusters 2>/dev/null | grep -q "^$(KIND_CLUSTER_NAME)$$"; then \
		echo "Kind cluster '$(KIND_CLUSTER_NAME)' already exists, skipping creation"; \
	else \
		echo "Creating Kind cluster '$(KIND_CLUSTER_NAME)'..."; \
		$(KIND) create cluster --name $(KIND_CLUSTER_NAME) --config dev/config/kind/cluster.yaml; \
		if echo "$(CONTAINER_ENGINE)" | grep -q "podman"; then \
			echo "Increasing podman PID limit for Kind node..."; \
			$(CONTAINER_ENGINE) update --pids-limit 4096 $(KIND_CLUSTER_NAME)-control-plane 2>/dev/null || true; \
		fi; \
		echo "Adding ingress-ready label to control-plane node..."; \
		kubectl label node $(KIND_CLUSTER_NAME)-control-plane ingress-ready=true --overwrite; \
		echo "Wrapping CNI portmap plugin to fix nftables rules on podman..."; \
		$(CONTAINER_ENGINE) exec $(KIND_CLUSTER_NAME)-control-plane test -f /opt/cni/bin/portmap.real \
			|| $(CONTAINER_ENGINE) exec $(KIND_CLUSTER_NAME)-control-plane mv /opt/cni/bin/portmap /opt/cni/bin/portmap.real; \
		$(CONTAINER_ENGINE) cp dev/config/kind/portmap-wrapper.sh $(KIND_CLUSTER_NAME)-control-plane:/opt/cni/bin/portmap; \
		$(CONTAINER_ENGINE) exec $(KIND_CLUSTER_NAME)-control-plane chmod +x /opt/cni/bin/portmap; \
		echo "✅ CNI portmap wrapper installed"; \
		echo "Installing nginx ingress controller..."; \
		kubectl apply -f dev/config/ingress/nginx-ingress.yaml; \
		echo "Waiting for ingress controller to be ready..."; \
		kubectl wait --namespace ingress-nginx --for=condition=ready pod --selector=app.kubernetes.io/component=controller --timeout=90s; \
		echo "✅ Ingress controller ready"; \
		echo "Installing cert-manager..."; \
		kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.2/cert-manager.yaml; \
		echo "Waiting for cert-manager to be ready..."; \
		kubectl wait --namespace cert-manager --for=condition=available --timeout=300s \
			deployment/cert-manager \
			deployment/cert-manager-cainjector \
			deployment/cert-manager-webhook; \
		echo "✅ cert-manager ready"; \
		echo "Creating cert-manager ClusterIssuer (waiting for webhook)..."; \
		for i in $$(seq 1 30); do \
			if kubectl apply -f dev/config/cert-manager/selfsigned-issuer.yaml 2>/dev/null; then \
				break; \
			fi; \
			if [ $$i -eq 30 ]; then \
				echo "ERROR: cert-manager webhook not ready after 30 attempts"; \
				exit 1; \
			fi; \
			sleep 2; \
		done; \
		echo "✅ ClusterIssuer created"; \
		echo "Adding /etc/hosts entry for Keycloak in control plane..."; \
		if command -v docker >/dev/null 2>&1 && docker ps --filter "name=$(KIND_CLUSTER_NAME)-control-plane" --format "{{.Names}}" | grep -q "$(KIND_CLUSTER_NAME)-control-plane"; then \
			docker exec $(KIND_CLUSTER_NAME)-control-plane bash -c 'grep -q "keycloak.127-0-0-1.sslip.io" /etc/hosts || echo "127.0.0.1 keycloak.127-0-0-1.sslip.io" >> /etc/hosts'; \
		elif command -v podman >/dev/null 2>&1 && podman ps --filter "name=$(KIND_CLUSTER_NAME)-control-plane" --format "{{.Names}}" | grep -q "$(KIND_CLUSTER_NAME)-control-plane"; then \
			podman exec $(KIND_CLUSTER_NAME)-control-plane bash -c 'grep -q "keycloak.127-0-0-1.sslip.io" /etc/hosts || echo "127.0.0.1 keycloak.127-0-0-1.sslip.io" >> /etc/hosts'; \
		fi; \
		echo "✅ /etc/hosts entry added"; \
	fi
	@echo "Exporting kubeconfig to _output/kubeconfig..."; \
	mkdir -p _output; \
	$(KIND) export kubeconfig --name $(KIND_CLUSTER_NAME) --kubeconfig _output/kubeconfig; \
	echo "✅ Kubeconfig exported to _output/kubeconfig"

SECONDARY_CLUSTER_NAME = $(KIND_CLUSTER_NAME)-2

.PHONY: kind-create-secondary-cluster
kind-create-secondary-cluster: kind kind-create-certs ## Create a secondary Kind cluster for multi-cluster testing
	@if [ "$(shell uname -s)" != "Darwin" ] && echo "$(CONTAINER_ENGINE)" | grep -q "podman"; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
	fi; \
	if $(KIND) get clusters 2>/dev/null | grep -q "^$(SECONDARY_CLUSTER_NAME)$$"; then \
		echo "Kind cluster '$(SECONDARY_CLUSTER_NAME)' already exists, skipping creation"; \
	else \
		echo "Creating secondary Kind cluster '$(SECONDARY_CLUSTER_NAME)'..."; \
		$(KIND) create cluster --name $(SECONDARY_CLUSTER_NAME) --config dev/config/kind/cluster-secondary.yaml; \
		echo "Routing Keycloak traffic from secondary cluster to primary..."; \
		PRIMARY_IP=$$($(CONTAINER_ENGINE) inspect $(KIND_CLUSTER_NAME)-control-plane -f '{{.NetworkSettings.Networks.kind.IPAddress}}' 2>/dev/null || echo ""); \
		if [ -n "$${PRIMARY_IP}" ]; then \
			$(CONTAINER_ENGINE) exec $(SECONDARY_CLUSTER_NAME)-control-plane bash -c "grep -q 'keycloak.127-0-0-1.sslip.io' /etc/hosts || echo '$${PRIMARY_IP} keycloak.127-0-0-1.sslip.io' >> /etc/hosts"; \
			$(CONTAINER_ENGINE) exec $(KIND_CLUSTER_NAME)-control-plane iptables -t nat -C PREROUTING -p tcp --dport 8443 -j REDIRECT --to-port 443 2>/dev/null || \
				$(CONTAINER_ENGINE) exec $(KIND_CLUSTER_NAME)-control-plane iptables -t nat -A PREROUTING -p tcp --dport 8443 -j REDIRECT --to-port 443; \
			echo "✅ Keycloak routing configured (primary=$${PRIMARY_IP})"; \
		else \
			echo "⚠️  Could not determine primary cluster IP — Keycloak OIDC may not work on secondary cluster"; \
		fi; \
	fi
	@echo "Exporting secondary kubeconfig..."; \
	mkdir -p _output; \
	$(KIND) export kubeconfig --name $(SECONDARY_CLUSTER_NAME) --kubeconfig _output/kubeconfig-secondary; \
	echo "✅ Secondary kubeconfig exported to _output/kubeconfig-secondary"

.PHONY: kind-multicluster-kubeconfig
kind-multicluster-kubeconfig: ## Generate a merged kubeconfig with internal Docker IPs for multi-cluster testing
	@echo "Generating multi-cluster kubeconfig with internal IPs..."
	@PRIMARY_IP=$$($(CONTAINER_ENGINE) inspect $(KIND_CLUSTER_NAME)-control-plane -f '{{.NetworkSettings.Networks.kind.IPAddress}}' 2>/dev/null || echo ""); \
	SECONDARY_IP=$$($(CONTAINER_ENGINE) inspect $(SECONDARY_CLUSTER_NAME)-control-plane -f '{{.NetworkSettings.Networks.kind.IPAddress}}' 2>/dev/null || echo ""); \
	if [ -z "$${PRIMARY_IP}" ] || [ -z "$${SECONDARY_IP}" ]; then \
		echo "ERROR: could not determine cluster IPs (primary=$${PRIMARY_IP}, secondary=$${SECONDARY_IP})"; \
		exit 1; \
	fi; \
	KUBECONFIG=_output/kubeconfig:_output/kubeconfig-secondary $(KUBECTL) config view --flatten > _output/multicluster-kubeconfig-raw; \
	PRIMARY_PORT=$$(sed -n 's|.*server: https://127\.0\.0\.1:\([0-9]*\).*|\1|p' _output/kubeconfig | head -1); \
	SECONDARY_PORT=$$(sed -n 's|.*server: https://127\.0\.0\.1:\([0-9]*\).*|\1|p' _output/kubeconfig-secondary | head -1); \
	if [ -z "$${PRIMARY_PORT}" ] || [ -z "$${SECONDARY_PORT}" ]; then \
		echo "ERROR: could not extract API server ports (primary=$${PRIMARY_PORT}, secondary=$${SECONDARY_PORT})"; \
		rm -f _output/multicluster-kubeconfig-raw; \
		exit 1; \
	fi; \
	if [ "$${PRIMARY_PORT}" = "$${SECONDARY_PORT}" ]; then \
		echo "ERROR: both clusters share the same host port ($${PRIMARY_PORT}); cannot disambiguate in sed rewrite"; \
		rm -f _output/multicluster-kubeconfig-raw; \
		exit 1; \
	fi; \
	sed \
		-e "s|https://127\.0\.0\.1:$${PRIMARY_PORT}|https://$${PRIMARY_IP}:6443|g" \
		-e "s|https://127\.0\.0\.1:$${SECONDARY_PORT}|https://$${SECONDARY_IP}:6443|g" \
		_output/multicluster-kubeconfig-raw > _output/multicluster-kubeconfig; \
	rm -f _output/multicluster-kubeconfig-raw; \
	echo "✅ Multi-cluster kubeconfig written to _output/multicluster-kubeconfig"; \
	echo "   Primary:   $${PRIMARY_IP}:6443 (context: kind-$(KIND_CLUSTER_NAME))"; \
	echo "   Secondary: $${SECONDARY_IP}:6443 (context: kind-$(SECONDARY_CLUSTER_NAME))"

.PHONY: kind-delete-cluster
kind-delete-cluster: kind
	@# Set KIND provider for podman on Linux
	@if [ "$(shell uname -s)" != "Darwin" ] && echo "$(CONTAINER_ENGINE)" | grep -q "podman"; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
	fi; \
	$(KIND) delete cluster --name $(KIND_CLUSTER_NAME)

.PHONY: kind-delete-secondary-cluster
kind-delete-secondary-cluster: kind ## Delete the secondary Kind cluster
	@if [ "$(shell uname -s)" != "Darwin" ] && echo "$(CONTAINER_ENGINE)" | grep -q "podman"; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
	fi; \
	$(KIND) delete cluster --name $(SECONDARY_CLUSTER_NAME)
