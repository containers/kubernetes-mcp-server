##@ Browser Tests

BROWSER_MCP_PORT ?= 3001

.PHONY: browser-test
browser-test: build ## Run MCP Apps browser tests with a local HTTP server
	@set -eu; \
	mkdir -p _output/browser; \
	config=_output/browser/server.toml; \
	printf 'port = "$(BROWSER_MCP_PORT)"\nbind_address = "127.0.0.1"\napps_enabled = true\nlist_output = "table"\n' > $$config; \
	./$(BINARY_NAME) --config $$config & server_pid=$$!; \
	cleanup() { kill $$server_pid 2>/dev/null || true; wait $$server_pid 2>/dev/null || true; }; \
	trap cleanup EXIT; \
	for attempt in $$(seq 1 30); do \
		if curl -fsS http://127.0.0.1:$(BROWSER_MCP_PORT)/healthz >/dev/null; then break; fi; \
		sleep 1; \
	done; \
	curl -fsS http://127.0.0.1:$(BROWSER_MCP_PORT)/healthz >/dev/null || { echo "MCP server did not become ready"; exit 1; }; \
	MCP_SERVER_URL="http://127.0.0.1:$(BROWSER_MCP_PORT)/mcp" ./test/browser/run.sh
