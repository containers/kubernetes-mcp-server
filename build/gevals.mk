# Gevals evaluation support

MCP_PORT ?= 8008
MCP_HEALTH_TIMEOUT ?= 60
MCP_HEALTH_INTERVAL ?= 2

# Gevals configuration
GEVALS_BIN ?= $(shell command -v gevals 2>/dev/null || echo gevals)
EVALS_DIR = $(shell pwd)/evals

# Toolsets to enable for gevals tests (override via environment variable)
TOOLSETS ?= core,config,helm,kubevirt

# Model configuration (override via environment variables)
MODEL_NAME ?= gemini-2.0-flash

# Additional arguments to pass to gevals (e.g., -r "pattern")
GEVALS_ARGS ?=

##@ Gevals

.PHONY: run-server
run-server: build ## Start MCP server in background and wait for health check
	@echo "Starting MCP server on port $(MCP_PORT)..."
	@if [ -f _output/kubeconfig ]; then \
		echo "Using kubeconfig: $(shell pwd)/_output/kubeconfig (Kind cluster)"; \
		export KUBECONFIG=$(shell pwd)/_output/kubeconfig; \
	elif [ -n "$$KUBECONFIG" ]; then \
		echo "Using kubeconfig: $$KUBECONFIG (from environment)"; \
	else \
		echo "Using kubeconfig: ~/.kube/config (default)"; \
	fi; \
	echo ""; \
	if [ -n "$(TOOLSETS)" ]; then \
		./$(BINARY_NAME) --port $(MCP_PORT) --toolsets $(TOOLSETS) & echo $$! > .mcp-server.pid; \
	else \
		./$(BINARY_NAME) --port $(MCP_PORT) & echo $$! > .mcp-server.pid; \
	fi; \
	echo "MCP server started with PID $$(cat .mcp-server.pid)"; \
	echo "Waiting for MCP server to be ready..."; \
	elapsed=0; \
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

.PHONY: gevals-check
gevals-check: ## Check if gevals is available
	@if ! command -v $(GEVALS_BIN) >/dev/null 2>&1; then \
		echo "❌ Gevals not found in PATH"; \
		echo ""; \
		echo "Please install gevals:"; \
		echo "  - Download from: https://github.com/genmcp/gevals/releases"; \
		echo "  - Or build from source: git clone https://github.com/genmcp/gevals && cd gevals && make build"; \
		echo "  - Or set GEVALS_BIN to the path: export GEVALS_BIN=/path/to/gevals"; \
		exit 1; \
	fi
	@if [ ! -d "$(EVALS_DIR)" ]; then \
		echo "❌ Evals directory not found at $(EVALS_DIR)"; \
		echo ""; \
		echo "Ensure you are in the kubernetes-mcp-server repository root"; \
		exit 1; \
	fi
	@echo "✅ Gevals found: $(GEVALS_BIN)"

.PHONY: gevals-run
gevals-run: build gevals-check ## Run gevals tests against the MCP server
	@echo "========================================="
	@echo "Running Gevals Tests"
	@echo "========================================="
	@echo ""
	@if [ -f _output/kubeconfig ]; then \
		echo "Using kubeconfig: $(shell pwd)/_output/kubeconfig (Kind cluster)"; \
		export KUBECONFIG=$(shell pwd)/_output/kubeconfig; \
	elif [ -n "$$KUBECONFIG" ]; then \
		echo "Using kubeconfig: $$KUBECONFIG (from environment)"; \
	else \
		echo "Using kubeconfig: ~/.kube/config (default)"; \
	fi; \
	echo ""; \
	echo "Creating temporary MCP config..."; \
	TEMP_MCP_CONFIG=$$(mktemp); \
	printf 'mcpServers:\n  kubernetes:\n    command: %s\n    args: ["--toolsets", "%s"]\n    env: {}\n' \
		"$(shell pwd)/$(BINARY_NAME)" "$(TOOLSETS)" > $$TEMP_MCP_CONFIG; \
	echo "Creating temporary eval config..."; \
	TEMP_EVAL=$$(mktemp); \
	trap "rm -f $$TEMP_EVAL $$TEMP_MCP_CONFIG" EXIT; \
	if [ -z "$$JUDGE_BASE_URL" ] || [ -z "$$JUDGE_API_KEY" ] || [ -z "$$JUDGE_MODEL_NAME" ]; then \
		echo "Disabling LLM judge (JUDGE_* environment variables not set)"; \
		sed -e "s|glob: ../tasks|glob: $(EVALS_DIR)/tasks|" \
		    -e 's|model: ".*"|model: "$(MODEL_NAME)"|' \
		    -e "s|mcpConfigFile:.*|mcpConfigFile: $$TEMP_MCP_CONFIG|" \
		    -e '/llmJudge:/,/modelNameKey:/d' \
			$(EVALS_DIR)/openai-agent/eval-inline.yaml > $$TEMP_EVAL; \
	else \
		echo "Enabling LLM judge"; \
		sed -e "s|glob: ../tasks|glob: $(EVALS_DIR)/tasks|" \
		    -e 's|model: ".*"|model: "$(MODEL_NAME)"|' \
		    -e "s|mcpConfigFile:.*|mcpConfigFile: $$TEMP_MCP_CONFIG|" \
			$(EVALS_DIR)/openai-agent/eval-inline.yaml > $$TEMP_EVAL; \
	fi; \
	echo ""; \
	echo "Running gevals..."; \
	echo "  Eval file: $(EVALS_DIR)/openai-agent/eval-inline.yaml"; \
	echo "  MCP binary: $(shell pwd)/$(BINARY_NAME)"; \
	echo "  Toolsets: $(TOOLSETS)"; \
	echo "  Model: $(MODEL_NAME)"; \
	echo ""; \
	$(GEVALS_BIN) eval $(GEVALS_ARGS) $$TEMP_EVAL

.PHONY: gevals-run-claude
gevals-run-claude: build gevals-check ## Run gevals tests with Claude Code agent
	@echo "========================================="
	@echo "Running Gevals Tests (Claude Code)"
	@echo "========================================="
	@echo ""
	@if [ -f _output/kubeconfig ]; then \
		echo "Using kubeconfig: $(shell pwd)/_output/kubeconfig (Kind cluster)"; \
		export KUBECONFIG=$(shell pwd)/_output/kubeconfig; \
	elif [ -n "$$KUBECONFIG" ]; then \
		echo "Using kubeconfig: $$KUBECONFIG (from environment)"; \
	else \
		echo "Using kubeconfig: ~/.kube/config (default)"; \
	fi; \
	echo ""; \
	echo "Creating temporary MCP config..."; \
	TEMP_MCP_CONFIG=$$(mktemp); \
	printf 'mcpServers:\n  kubernetes:\n    command: %s\n    args: ["--toolsets", "%s"]\n    env: {}\n' \
		"$(shell pwd)/$(BINARY_NAME)" "$(TOOLSETS)" > $$TEMP_MCP_CONFIG; \
	echo "Creating temporary eval config..."; \
	TEMP_EVAL=$$(mktemp); \
	trap "rm -f $$TEMP_EVAL $$TEMP_MCP_CONFIG" EXIT; \
	if [ -z "$$JUDGE_BASE_URL" ] || [ -z "$$JUDGE_API_KEY" ] || [ -z "$$JUDGE_MODEL_NAME" ]; then \
		echo "Disabling LLM judge (JUDGE_* environment variables not set)"; \
		sed -e "s|glob: ../tasks|glob: $(EVALS_DIR)/tasks|" \
		    -e "s|mcpConfigFile:.*|mcpConfigFile: $$TEMP_MCP_CONFIG|" \
		    -e '/llmJudge:/,/modelNameKey:/d' \
			$(EVALS_DIR)/claude-code/eval-inline.yaml > $$TEMP_EVAL; \
	else \
		echo "Enabling LLM judge"; \
		sed -e "s|glob: ../tasks|glob: $(EVALS_DIR)/tasks|" \
		    -e "s|mcpConfigFile:.*|mcpConfigFile: $$TEMP_MCP_CONFIG|" \
			$(EVALS_DIR)/claude-code/eval-inline.yaml > $$TEMP_EVAL; \
	fi; \
	echo ""; \
	echo "Running gevals with Claude Code..."; \
	echo "  Eval file: $(EVALS_DIR)/claude-code/eval-inline.yaml"; \
	echo "  MCP binary: $(shell pwd)/$(BINARY_NAME)"; \
	echo "  Toolsets: $(TOOLSETS)"; \
	echo ""; \
	$(GEVALS_BIN) eval $(GEVALS_ARGS) $$TEMP_EVAL
