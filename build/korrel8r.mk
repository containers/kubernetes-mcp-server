##@ Korrel8r Evals

KORREL8R_EVAL_PF_PID := _output/korrel8r-eval-pf.pid

.PHONY: stop-korrel8r-eval
stop-korrel8r-eval: ## Stop the Korrel8r eval localhost port-forward
	@if [ -f $(KORREL8R_EVAL_PF_PID) ]; then \
		kill "$$(cat $(KORREL8R_EVAL_PF_PID))" 2>/dev/null || true; \
		rm -f $(KORREL8R_EVAL_PF_PID); \
	fi

.PHONY: teardown-korrel8r-eval
teardown-korrel8r-eval: ## Remove the Korrel8r backend
	bash evals/tasks/korrel8r/shared/teardown.sh

.PHONY: run-korrel8r-evals
run-korrel8r-evals: ## Run the combined Korrel8r resource and signal correlation task
	@set -e; \
	if [ -n "$(MCP_EVAL_KUBECONFIG)" ]; then export KUBECONFIG="$(MCP_EVAL_KUBECONFIG)"; fi; \
	trap '$(MAKE) stop-server; $(MAKE) teardown-korrel8r-eval' EXIT; \
	$(MAKE) run-server TOOLSETS='core,config,observability/korrel8r' MCP_CONFIG_DIR=dev/config/mcp-configs; \
	$(MAKE) run-evals SUITE=korrel8r; \
	echo "Korrel8r evals finished."

.PHONY: run-korrel8r-baseline
run-korrel8r-baseline: ## Run the same task using only core and config tools
	@set -e; \
	if [ -n "$(MCP_EVAL_KUBECONFIG)" ]; then export KUBECONFIG="$(MCP_EVAL_KUBECONFIG)"; fi; \
	trap '$(MAKE) stop-server; $(MAKE) teardown-korrel8r-eval' EXIT; \
	$(MAKE) run-server TOOLSETS='core,config' MCP_CONFIG_DIR=dev/config/mcp-configs; \
	$(MAKE) run-evals SUITE=korrel8r EVAL_CONFIG=evals/core-eval-testing/$(AGENT)/eval-korrel8r-baseline.yaml; \
	echo "Korrel8r core baseline finished."
