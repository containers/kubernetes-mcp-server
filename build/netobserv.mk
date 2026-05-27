##@ NetObserv (eval mock)

NETOBSERV_PORT ?= 9001
NETOBSERV_MOCK_MANIFEST ?= $(shell pwd)/evals/tasks/netobserv/shared/mock-plugin.yaml

.PHONY: install-netobserv-mock
install-netobserv-mock: ## Deploy mock NetObserv console plugin API in the cluster
	kubectl apply -f $(NETOBSERV_MOCK_MANIFEST)
	kubectl wait --namespace netobserv --for=condition=available deployment/netobserv-plugin --timeout=120s

.PHONY: expose-netobserv
expose-netobserv: install-netobserv-mock ## Port-forward mock plugin to localhost for MCP server
	@echo "Port-forwarding netobserv-plugin to http://127.0.0.1:$(NETOBSERV_PORT)..."
	@kubectl -n netobserv port-forward svc/netobserv-plugin $(NETOBSERV_PORT):9001 >/dev/null 2>&1 & echo $$! > .netobserv-pf.pid
	@timeout 30s bash -c 'until curl -sf http://127.0.0.1:$(NETOBSERV_PORT)/api/resources/namespaces >/dev/null; do sleep 1; done' \
		&& echo "NetObserv mock plugin is ready at http://127.0.0.1:$(NETOBSERV_PORT)"

.PHONY: setup-netobserv
setup-netobserv: expose-netobserv ## Install mock NetObserv plugin and expose it locally

.PHONY: stop-netobserv
stop-netobserv: ## Stop NetObserv port-forward started by expose-netobserv
	@if [ -f .netobserv-pf.pid ]; then \
		PID=$$(cat .netobserv-pf.pid); \
		echo "Stopping NetObserv port-forward (PID: $$PID)"; \
		kill $$PID 2>/dev/null || true; \
		rm -f .netobserv-pf.pid; \
	else \
		echo "No .netobserv-pf.pid file found"; \
	fi

.PHONY: teardown-netobserv
teardown-netobserv: stop-netobserv ## Remove mock NetObserv plugin from the cluster
	kubectl delete -f $(NETOBSERV_MOCK_MANIFEST) --ignore-not-found
