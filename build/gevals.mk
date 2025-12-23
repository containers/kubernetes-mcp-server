# Gevals evaluation support

MCP_PORT ?= 8080
MCP_HEALTH_TIMEOUT ?= 60
MCP_HEALTH_INTERVAL ?= 2

##@ Gevals

.PHONY: run-server
run-server: build ## Start MCP server in background and wait for health check
	@echo "Starting MCP server on port $(MCP_PORT)..."
	@if [ -n "$(TOOLSETS)" ]; then \
		./$(BINARY_NAME) --port $(MCP_PORT) --toolsets $(TOOLSETS) & echo $$! > .mcp-server.pid; \
	else \
		./$(BINARY_NAME) --port $(MCP_PORT) & echo $$! > .mcp-server.pid; \
	fi
	@echo "MCP server started with PID $$(cat .mcp-server.pid)"
	@echo "Waiting for MCP server to be ready..."
	@elapsed=0; \
	while [ $$elapsed -lt $(MCP_HEALTH_TIMEOUT) ]; do \
		if curl -s http://localhost:$(MCP_PORT)/health > /dev/null 2>&1; then \
			echo "MCP server is ready"; \
			exit 0; \
		fi; \
		echo "  Waiting... ($$elapsed/$(MCP_HEALTH_TIMEOUT)s)"; \
		sleep $(MCP_HEALTH_INTERVAL); \
		elapsed=$$((elapsed + $(MCP_HEALTH_INTERVAL))); \
	done; \
	echo "ERROR: MCP server failed to start within $(MCP_HEALTH_TIMEOUT) seconds"; \
	exit 1

.PHONY: stop-server
stop-server: ## Stop the MCP server started by run-server
	@if [ -f .mcp-server.pid ]; then \
		PID=$$(cat .mcp-server.pid); \
		echo "Stopping MCP server (PID: $$PID)"; \
		kill $$PID 2>/dev/null || true; \
		rm -f .mcp-server.pid; \
	else \
		echo "No .mcp-server.pid file found"; \
	fi
