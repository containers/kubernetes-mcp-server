##@ Istio/Kiali

SHELL := /bin/bash

ISTIOCTL = $(shell pwd)/_output/tools/bin/istioctl
ISTIO_ADDONS_DIR = $(shell pwd)/_output/istio-addons
ISTIO_VERSION = 1.30.1
KIALI_VERSION = v2.27
# Multicluster evals call Kiali MCP tools (e.g. list_clusters) that exist only on Kiali master today.
KIALI_MC_VERSION = dev
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
install-istio: istioctl
	$(ISTIOCTL) install --set profile=demo \
		--set meshConfig.defaultConfig.tracing.zipkin.address=zipkin.istio-system:9411 \
		-y
	kubectl apply -f $(ISTIO_ADDONS_DIR)/prometheus.yaml -n istio-system
	kubectl apply -f $(ISTIO_ADDONS_DIR)/kiali.yaml -n istio-system
	kubectl apply -f $(ISTIO_ADDONS_DIR)/jaeger.yaml -n istio-system
	kubectl wait --namespace istio-system --for=condition=available deployment/kiali --timeout=300s
	kubectl wait --namespace istio-system --for=condition=available deployment/prometheus --timeout=300s
	kubectl wait --for=condition=Ready pod --all -n istio-system --timeout=300s
	kubectl rollout status deployment/kiali -n istio-system
	kubectl label namespace default istio-injection=enabled --overwrite
	kubectl wait --for=condition=Ready pod --all -n istio-system --timeout=300s
	
# Install Bookinfo demo
.PHONY: install-bookinfo-demo
install-bookinfo-demo:
	kubectl create ns bookinfo
	kubectl label namespace bookinfo istio-discovery=enabled istio.io/rev=default istio-injection=enabled
	kubectl apply -f https://raw.githubusercontent.com/openshift-service-mesh/istio/refs/heads/master/samples/bookinfo/platform/kube/bookinfo.yaml -n bookinfo
	kubectl apply -n bookinfo -f https://raw.githubusercontent.com/istio-ecosystem/sail-operator/main/chart/samples/ingress-gateway.yaml
	kubectl apply -f https://raw.githubusercontent.com/openshift-service-mesh/istio/refs/heads/master/samples/bookinfo/networking/bookinfo-gateway.yaml -n bookinfo
	kubectl wait --for=condition=Ready pod --all -n bookinfo --timeout=300s

# Update Kiali version
.PHONY: update-kiali-version
update-kiali-version:
	@echo "Updating Kiali version to $(KIALI_VERSION)..."
	@kubectl patch deployment kiali -n istio-system -p '{"spec":{"template":{"spec":{"containers":[{"name":"kiali","image":"quay.io/kiali/kiali_mcp:$(KIALI_VERSION)"}]}}}}'
	@kubectl delete pod -l app=kiali -n istio-system
	@kubectl wait --for=condition=available deployment/kiali -n istio-system --timeout=300s

# Expose Bookinfo demo
.PHONY: expose-bookinfo-demo
expose-bookinfo-demo:
	@echo "Exposing Bookinfo demo..."
	@kubectl port-forward svc/istio-ingressgateway 20002:80 -n bookinfo >/dev/null 2>&1 & \
	while true; do curl -s -o /dev/null http://localhost:20002/productpage; sleep 1; done & \
	echo "Bookinfo demo is being exposed on http://localhost:20002/productpage and generator is running"

# Expose Kiali service
.PHONY: expose-kiali
expose-kiali:
	@echo "Exposing Kiali service..."
	kubectl -n istio-system port-forward svc/kiali 20001:20001 & \
	timeout 30s bash -c 'until curl -s localhost:20001; do sleep 1; done' && \
	echo "Kiali is being exposed on http://localhost:20001"

.PHONY: setup-kiali
setup-kiali: install-istio install-gateway-api-crds update-kiali-version install-bookinfo-demo expose-kiali expose-bookinfo-demo ## Setup Kiali

# Optional local Kiali checkout for multicluster hack scripts (override: KIALI_SRC=/path/to/kiali).
KIALI_REF ?= master
KIALI_SRC ?=
KIALI_HACK_DIR = $(if $(KIALI_SRC),$(KIALI_SRC),$(shell pwd)/_output/kiali-hack)
KIALI_MC_CONFIG = dev/config/mcp-configs-multicluster/kiali.toml

.PHONY: write-kiali-multicluster-mcp-config
write-kiali-multicluster-mcp-config: ## Write MCP kiali.toml from Kiali LoadBalancer on kind-east
	@set -euo pipefail; \
	kubectl wait --for=jsonpath='{.status.loadBalancer.ingress[0]}' -n istio-system service/kiali \
		--context kind-east --timeout=300s; \
	KIALI_URL=$$(kubectl get svc kiali -n istio-system --context kind-east \
		-o=jsonpath='http://{.status.loadBalancer.ingress[0].ip}/kiali/'); \
	if [ -z "$${KIALI_URL}" ] || [ "$${KIALI_URL}" = "http:///kiali/" ]; then \
		echo "ERROR: failed to resolve Kiali LoadBalancer URL on kind-east" >&2; \
		exit 1; \
	fi; \
	mkdir -p dev/config/mcp-configs-multicluster; \
	printf '%s\n' "[toolset_configs.kiali]" "url = \"$${KIALI_URL}\"" "insecure = true" > "$(KIALI_MC_CONFIG)"; \
	echo "Wrote $(KIALI_MC_CONFIG) (url=$${KIALI_URL})"

.PHONY: setup-kiali-multicluster
setup-kiali-multicluster: ## Setup primary-remote multicluster Kind + Istio/Kiali for MCP evals
	@set -euo pipefail; \
	ROOTDIR="$$(pwd)"; \
	KIALI_HACK_DIR="$(KIALI_HACK_DIR)"; \
	for cmd in git curl kubectl docker helm yq envsubst jq; do \
		command -v "$$cmd" >/dev/null 2>&1 || { echo "ERROR: required command not found: $$cmd" >&2; exit 1; }; \
	done; \
	if [ ! -f "$${KIALI_HACK_DIR}/hack/setup-kind-in-ci.sh" ]; then \
		echo "Cloning kiali/kiali ($(KIALI_REF)) into $${KIALI_HACK_DIR}..."; \
		git clone --depth 1 --branch "$(KIALI_REF)" https://github.com/kiali/kiali.git "$${KIALI_HACK_DIR}"; \
	fi; \
	if ! grep -q 'kiali-version' "$${KIALI_HACK_DIR}/hack/setup-kind-in-ci.sh" 2>/dev/null; then \
		echo "ERROR: $${KIALI_HACK_DIR} is too old for multicluster setup (missing --kiali-version support)." >&2; \
		echo "Set KIALI_SRC to a newer checkout or remove stale clone: rm -rf $${KIALI_HACK_DIR}" >&2; \
		exit 1; \
	fi; \
	echo "Using Kiali hack scripts from: $${KIALI_HACK_DIR}"; \
	echo "Setting up primary-remote multicluster Kind clusters (east/west)..."; \
	( \
		cd "$${KIALI_HACK_DIR}"; \
		./hack/setup-kind-in-ci.sh \
			--multicluster primary-remote \
			--tempo false \
			--auth-strategy anonymous \
			-kv "$(KIALI_MC_VERSION)" \
			--istio-version "$(ISTIO_VERSION)" \
			--deploy-kiali true; \
	); \
	echo "Waiting for Kiali on kind-east..."; \
	kubectl rollout status deployment/kiali -n istio-system --context kind-east --timeout=300s; \
	$(MAKE) write-kiali-multicluster-mcp-config; \
	KIALI_URL=$$(kubectl get svc kiali -n istio-system --context kind-east \
		-o=jsonpath='http://{.status.loadBalancer.ingress[0].ip}/kiali/'); \
	echo "Waiting for Kiali health at $${KIALI_URL}..."; \
	start=$$(date +%s); \
	while ! curl -sf "$${KIALI_URL}healthz" >/dev/null 2>&1; do \
		elapsed=$$(( $$(date +%s) - start )); \
		if [ "$$elapsed" -ge 120 ]; then \
			echo "ERROR: timed out waiting for Kiali health at $${KIALI_URL}healthz" >&2; \
			exit 1; \
		fi; \
		sleep 2; \
	done; \
	echo "Kiali is healthy"; \
	echo "Verifying Kiali MCP list_clusters..."; \
	list_clusters=$$(curl -sk -X POST "$${KIALI_URL}api/chat/mcp/list_clusters" \
		-H 'Content-Type: application/json' -d '{"mcp_mode":true}'); \
	if ! echo "$$list_clusters" | jq -e 'type == "array"' >/dev/null 2>&1; then \
		echo "ERROR: Kiali MCP list_clusters unavailable: $$list_clusters" >&2; \
		echo "Multicluster evals require Kiali master (KIALI_MC_VERSION=dev). Try:" >&2; \
		echo "  KIALI_SRC=~/dev/kiali_sources/kiali make redeploy-kiali-multicluster-dev" >&2; \
		exit 1; \
	fi; \
	echo "list_clusters OK ($$(echo "$$list_clusters" | jq -c 'map(.name)'))"; \
	echo "Waiting for reviews traffic on west mesh cluster..."; \
	start=$$(date +%s); \
	while true; do \
		elapsed=$$(( $$(date +%s) - start )); \
		if [ "$$elapsed" -ge 300 ]; then \
			echo "ERROR: timed out waiting for healthy reviews app on west cluster" >&2; \
			exit 1; \
		fi; \
		response=$$(curl -sk "$${KIALI_URL}api/clusters/apps?namespaces=bookinfo&clusterName=west&health=true&istioResources=true&rateInterval=60s"); \
		if [ "$$(echo "$$response" | jq '[.applications[]? | select(.name=="reviews" and .cluster=="west" and .health.requests.inbound.http."200" > 0)] | length > 0')" = "true" ]; then \
			echo "reviews app on west cluster is healthy"; \
			break; \
		fi; \
		sleep 10; \
	done; \
	echo "Waiting for traces on west mesh cluster..."; \
	traces_date=$$(( ($$(date +%s) - 300) * 1000 )); \
	trace_url="$${KIALI_URL}api/namespaces/bookinfo/workloads/reviews-v2/traces?startMicros=$${traces_date}&tags=&limit=100&clusterName=west"; \
	start=$$(date +%s); \
	while true; do \
		elapsed=$$(( $$(date +%s) - start )); \
		if [ "$$elapsed" -ge 120 ]; then \
			echo "WARNING: timed out waiting for traces on west; continuing anyway" >&2; \
			break; \
		fi; \
		result=$$(curl -sk "$$trace_url" | jq -r '.data // []'); \
		if [ -n "$$result" ] && [ "$$result" != "[]" ]; then \
			echo "Kiali has traces for reviews-v2 on west"; \
			break; \
		fi; \
		sleep 10; \
	done; \
	echo "Multicluster setup complete."; \
	echo "  Kiali URL: $${KIALI_URL}"; \
	echo "  MCP config: $(KIALI_MC_CONFIG)"; \
	echo "  Contexts: kind-east (primary), kind-west (remote)"; \
	echo "  Mesh clusters: east, west"

.PHONY: redeploy-kiali-multicluster-dev
redeploy-kiali-multicluster-dev: ## Rebuild Kiali dev image from master and redeploy on kind-east
	@set -euo pipefail; \
	KIALI_REPO="$(KIALI_HACK_DIR)"; \
	if [ ! -f "$${KIALI_REPO}/Makefile" ]; then \
		echo "ERROR: Kiali repo not found at $${KIALI_REPO}. Set KIALI_SRC to a master checkout." >&2; \
		exit 1; \
	fi; \
	if ! git -C "$${KIALI_REPO}" cat-file -e HEAD:ai/mcp/list_clusters/list_clusters.go 2>/dev/null; then \
		echo "ERROR: Kiali checkout at $${KIALI_REPO} lacks list_clusters (need master)." >&2; \
		exit 1; \
	fi; \
	echo "Building Kiali dev image from $${KIALI_REPO}..."; \
	$(MAKE) -e -C "$${KIALI_REPO}" build-ui build; \
	$(MAKE) -e -C "$${KIALI_REPO}" DORP=docker CLUSTER_TYPE=kind KIND_NAME=east cluster-push-kiali; \
	kubectl rollout restart deployment/kiali -n istio-system --context kind-east; \
	kubectl rollout status deployment/kiali -n istio-system --context kind-east --timeout=300s; \
	$(MAKE) write-kiali-multicluster-mcp-config; \
	KIALI_URL=$$(kubectl get svc kiali -n istio-system --context kind-east \
		-o=jsonpath='http://{.status.loadBalancer.ingress[0].ip}/kiali/'); \
	start=$$(date +%s); \
	while ! curl -sf "$${KIALI_URL}healthz" >/dev/null 2>&1; do \
		elapsed=$$(( $$(date +%s) - start )); \
		if [ "$$elapsed" -ge 120 ]; then \
			echo "ERROR: timed out waiting for Kiali health at $${KIALI_URL}healthz" >&2; \
			exit 1; \
		fi; \
		sleep 2; \
	done; \
	list_clusters=$$(curl -sk -X POST "$${KIALI_URL}api/chat/mcp/list_clusters" \
		-H 'Content-Type: application/json' -d '{"mcp_mode":true}'); \
	if ! echo "$$list_clusters" | jq -e 'type == "array"' >/dev/null 2>&1; then \
		echo "ERROR: list_clusters still unavailable: $$list_clusters" >&2; \
		exit 1; \
	fi; \
	echo "list_clusters OK: $$list_clusters"

.PHONY: kind-delete-multicluster
kind-delete-multicluster: ## Delete primary-remote multicluster Kind clusters (east/west)
	@# Set KIND provider for podman on Linux
	@if [ "$(shell uname -s)" != "Darwin" ] && echo "$(CONTAINER_ENGINE)" | grep -q "podman"; then \
		export KIND_EXPERIMENTAL_PROVIDER=podman; \
	fi; \
	$(KIND) delete cluster --name east 2>/dev/null || true; \
	$(KIND) delete cluster --name west 2>/dev/null || true

KIALI_MC_EVAL_CONFIG ?= evals/tasks/kiali/multicluster/eval.yaml
KIALI_MC_EVAL_RESULTS ?= evals/results/openai-agent-multicluster-latest.json

.PHONY: run-evals-multicluster
run-evals-multicluster: mcpchecker ## Run mcpchecker multicluster Kiali evaluations
	$(MCPCHECKER) check $(KIALI_MC_EVAL_CONFIG) \
		$(if $(EVAL_TASK_FILTER),--run "$(EVAL_TASK_FILTER)",) \
		$(if $(filter true,$(EVAL_VERBOSE)),--verbose,) \
		--default-task-timeout 15m \
		--output json
	@if [ -f mcpchecker-kiali-multicluster-out.json ]; then \
		mkdir -p $(dir $(KIALI_MC_EVAL_RESULTS)); \
		mv -f mcpchecker-kiali-multicluster-out.json $(KIALI_MC_EVAL_RESULTS); \
	fi
