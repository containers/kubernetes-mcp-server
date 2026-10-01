##@ Browser Tests

BROWSER_MCP_PORT ?= 3001

.PHONY: browser-test
browser-test: build ## Run MCP Apps browser tests with a local HTTPS server
	@set -eu; \
	mkdir -p _output/browser; \
	cert="$$PWD/_output/browser/proxy-cert.pem"; \
	key="$$PWD/_output/browser/proxy-key.pem"; \
	(umask 077; openssl req -x509 -newkey rsa:2048 -nodes -days 1 -keyout "$$key" -out "$$cert" -subj "/CN=127.0.0.1" -addext "subjectAltName=IP:127.0.0.1"); \
	config=_output/browser/server.toml; \
	printf 'port = "$(BROWSER_MCP_PORT)"\nbind_address = "127.0.0.1"\napps_enabled = true\nlist_output = "table"\ntls_cert = "%s"\ntls_key = "%s"\n' "$$cert" "$$key" > $$config; \
	./$(BINARY_NAME) --config $$config & server_pid=$$!; \
	cleanup() { kill $$server_pid 2>/dev/null || true; wait $$server_pid 2>/dev/null || true; }; \
	trap cleanup EXIT; \
	for attempt in $$(seq 1 30); do \
		if curl --cacert "$$cert" -fsS https://127.0.0.1:$(BROWSER_MCP_PORT)/healthz >/dev/null; then break; fi; \
		sleep 1; \
	done; \
	curl --cacert "$$cert" -fsS https://127.0.0.1:$(BROWSER_MCP_PORT)/healthz >/dev/null || { echo "MCP server did not become ready"; exit 1; }; \
	MCP_SERVER_URL="https://127.0.0.1:$(BROWSER_MCP_PORT)/mcp" ./test/browser/run.sh
