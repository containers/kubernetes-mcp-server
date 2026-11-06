##@ Korrel8r Evals

KORREL8R_EVAL_MANIFEST := evals/tasks/korrel8r/shared/korrel8r.yaml
KORREL8R_EVAL_PF_PID := _output/korrel8r-eval-pf.pid

.PHONY: stop-korrel8r-eval
stop-korrel8r-eval: ## Stop the Korrel8r eval localhost port-forward
	@if [ -f $(KORREL8R_EVAL_PF_PID) ]; then \
		kill "$$(cat $(KORREL8R_EVAL_PF_PID))" 2>/dev/null || true; \
		rm -f $(KORREL8R_EVAL_PF_PID); \
	fi

.PHONY: teardown-korrel8r-eval
teardown-korrel8r-eval: stop-korrel8r-eval ## Remove the API-only Korrel8r eval backend
	kubectl delete -f $(KORREL8R_EVAL_MANIFEST) --ignore-not-found

.PHONY: run-korrel8r-evals
run-korrel8r-evals: ## Run Korrel8r correlation eval against its own deployment fixture
	@set -e; \
	if [ -n "$(MCP_EVAL_KUBECONFIG)" ]; then export KUBECONFIG="$(MCP_EVAL_KUBECONFIG)"; fi; \
	trap '$(MAKE) stop-server; $(MAKE) teardown-korrel8r-eval' EXIT; \
	bash evals/tasks/korrel8r/shared/setup.sh; \
	$(MAKE) run-server TOOLSETS='core,config,observability/korrel8r' MCP_CONFIG_DIR=dev/config/mcp-configs; \
	$(MAKE) run-evals SUITE=korrel8r; \
	echo "Korrel8r evals finished."
