# Observability toolset e2e / eval infrastructure (Prometheus, Tempo Operator, Loki Operator)
# Installs the operator-managed backends required by the obs toolsets.

OBS_SETUP_SCRIPT = hack/e2e/obs/setup.sh
OBS_PF_PID_FILE ?= .obs-pf.pids

##@ Obs Toolset E2E

.PHONY: obs-setup
obs-setup: kubectl ## Install obs stacks (Prometheus, Tempo, Loki) into existing cluster
	KUBECONFIG=$(shell pwd)/_output/kubeconfig $(OBS_SETUP_SCRIPT) up

.PHONY: obs-setup-prometheus
obs-setup-prometheus: kubectl ## Install only Prometheus stack
	KUBECONFIG=$(shell pwd)/_output/kubeconfig $(OBS_SETUP_SCRIPT) up --stacks prometheus

.PHONY: obs-setup-tempo
obs-setup-tempo: kubectl ## Install only Tempo stack (Tempo Operator + TempoStack CRs)
	KUBECONFIG=$(shell pwd)/_output/kubeconfig $(OBS_SETUP_SCRIPT) up --stacks tempo

.PHONY: obs-setup-loki
obs-setup-loki: kubectl ## Install only Loki stack (Loki Operator + LokiStack CR)
	KUBECONFIG=$(shell pwd)/_output/kubeconfig $(OBS_SETUP_SCRIPT) up --stacks loki

.PHONY: obs-teardown
obs-teardown: kubectl ## Remove obs stacks from cluster
	KUBECONFIG=$(shell pwd)/_output/kubeconfig $(OBS_SETUP_SCRIPT) down

.PHONY: e2e-setup-obs
e2e-setup-obs: e2e-setup obs-setup ## Full e2e setup with obs stacks (cluster + image + Prometheus + Tempo + Loki)

##@ Obs Toolset Evals

# Port-forward backends so a host-local MCP server (make run-server) can reach them.
# Port-forward layout for local MCP + evals. Loki uses host :3100 (MCP uses :8080).
.PHONY: expose-obs
expose-obs: kubectl ## Port-forward Prometheus, Alertmanager, Tempo, Loki for local MCP server
	@if [ -f $(OBS_PF_PID_FILE) ]; then $(MAKE) stop-obs; fi
	@rm -f $(OBS_PF_PID_FILE)
	@echo "Port-forwarding obs backends..."
	@$(KUBECTL) -n monitoring port-forward svc/prometheus-k8s 9090:9090 >/dev/null 2>&1 & echo $$! >> $(OBS_PF_PID_FILE)
	@$(KUBECTL) -n monitoring port-forward svc/alertmanager-main 9093:9093 >/dev/null 2>&1 & echo $$! >> $(OBS_PF_PID_FILE)
	@$(KUBECTL) -n tracing port-forward svc/tempo-tempo1-query-frontend 3200:3200 >/dev/null 2>&1 & echo $$! >> $(OBS_PF_PID_FILE)
	@$(KUBECTL) -n obs-loki port-forward svc/obs-loki-gateway-http 3100:8080 >/dev/null 2>&1 & echo $$! >> $(OBS_PF_PID_FILE)
	@sleep 2
	@for pid in $$(cat $(OBS_PF_PID_FILE)); do \
		if ! kill -0 $$pid 2>/dev/null; then \
			echo "Port-forward PID $$pid failed to start"; \
			$(MAKE) stop-obs; \
			exit 1; \
		fi; \
	done
	@timeout 60s bash -c 'until curl -sf http://127.0.0.1:9090/-/ready >/dev/null; do sleep 1; done' \
		&& echo "Prometheus ready at http://127.0.0.1:9090"
	@timeout 30s bash -c 'until curl -sf http://127.0.0.1:9093/-/ready >/dev/null; do sleep 1; done' \
		&& echo "Alertmanager ready at http://127.0.0.1:9093"
	@timeout 30s bash -c 'until curl -sf http://127.0.0.1:3200/ready >/dev/null; do sleep 1; done' \
		&& echo "Tempo ready at http://127.0.0.1:3200"
	@timeout 60s bash -c 'until curl -sf http://127.0.0.1:3100/ready >/dev/null || curl -sf http://127.0.0.1:3100/ >/dev/null; do sleep 1; done' \
		&& echo "Loki gateway ready at http://127.0.0.1:3100"

.PHONY: stop-obs
stop-obs: ## Stop obs port-forwards started by expose-obs
	@if [ -f $(OBS_PF_PID_FILE) ]; then \
		while read -r PID; do \
			echo "Stopping obs port-forward (PID: $$PID)"; \
			kill $$PID 2>/dev/null || true; \
		done < $(OBS_PF_PID_FILE); \
		rm -f $(OBS_PF_PID_FILE); \
	else \
		echo "No $(OBS_PF_PID_FILE) file found"; \
	fi

.PHONY: setup-obs-evals
setup-obs-evals: obs-setup expose-obs ## Install obs stacks and expose backends for local evals

OBS_TOOLSETS ?= observability/metrics,observability/traces,observability/logs,observability/otelcol

.PHONY: run-obs-evals
run-obs-evals: build expose-obs ## Run obs mcpchecker suite (requires obs-setup; exposes backends + MCP server + evals)
	@set -e; \
	trap '$(MAKE) stop-server stop-obs' EXIT; \
	$(MAKE) run-server TOOLSETS='$(OBS_TOOLSETS)' MCP_CONFIG_DIR=dev/config/mcp-configs; \
	$(MAKE) run-evals SUITE=obs; \
	echo ""; \
	echo "Obs evals finished. Target pass rate: >= 80% tasks and assertions."
