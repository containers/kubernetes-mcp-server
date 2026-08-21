# MCP protocol conformance tests
# Runs the official MCP conformance suite (modelcontextprotocol/conformance)
# against the server binary started locally with a fake kubeconfig.

CONFORMANCE_PORT ?= 8085
CONFORMANCE_BASELINE ?= test/conformance/conformance-baseline.yml
CONFORMANCE_VERSION ?= 0.1.16
CONFORMANCE_HEALTH_TIMEOUT ?= 30
CONFORMANCE_HEALTH_INTERVAL ?= 1
CONFORMANCE_SUITE ?=

##@ Conformance

.PHONY: conformance-test
conformance-test: build ## Run MCP protocol conformance tests against a local server
	@echo "Starting MCP server on port $(CONFORMANCE_PORT) for conformance testing..."
	@mkdir -p _output
	@printf '%s\n' \
		'apiVersion: v1' \
		'kind: Config' \
		'clusters:' \
		'- cluster:' \
		'    server: https://127.0.0.1:6443' \
		'  name: conformance' \
		'contexts:' \
		'- context:' \
		'    cluster: conformance' \
		'    user: conformance' \
		'  name: conformance' \
		'current-context: conformance' \
		'users:' \
		'- name: conformance' \
		'  user:' \
		'    token: fake-token' \
		> _output/conformance-kubeconfig
	@./$(BINARY_NAME) --port $(CONFORMANCE_PORT) --kubeconfig _output/conformance-kubeconfig & echo $$! > .conformance-server.pid
	@echo "Waiting for server to be ready..."
	@elapsed=0; \
	while [ $$elapsed -lt $(CONFORMANCE_HEALTH_TIMEOUT) ]; do \
		if curl -fsS http://localhost:$(CONFORMANCE_PORT)/healthz > /dev/null 2>&1; then \
			echo "Server is ready"; \
			break; \
		fi; \
		sleep $(CONFORMANCE_HEALTH_INTERVAL); \
		elapsed=$$((elapsed + $(CONFORMANCE_HEALTH_INTERVAL))); \
	done; \
	if [ $$elapsed -ge $(CONFORMANCE_HEALTH_TIMEOUT) ]; then \
		echo "ERROR: Server failed to start within $(CONFORMANCE_HEALTH_TIMEOUT)s"; \
		kill $$(cat .conformance-server.pid) 2>/dev/null || true; \
		rm -f .conformance-server.pid; \
		exit 1; \
	fi
	@echo "Running MCP conformance tests..."
	@rc=0; \
	npx -y @modelcontextprotocol/conformance@$(CONFORMANCE_VERSION) server \
		--url http://localhost:$(CONFORMANCE_PORT)/mcp \
		--expected-failures $(CONFORMANCE_BASELINE) \
		$(if $(CONFORMANCE_SUITE),--suite $(CONFORMANCE_SUITE)) \
		|| rc=$$?; \
	echo "Stopping server..."; \
	kill $$(cat .conformance-server.pid) 2>/dev/null || true; \
	rm -f .conformance-server.pid; \
	exit $$rc
