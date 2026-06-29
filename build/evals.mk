# Evals - mcpchecker evaluation support

MCP_PORT ?= 8080
MCP_HEALTH_TIMEOUT ?= 60
MCP_HEALTH_INTERVAL ?= 2
MCP_CONFIG_DIR ?= dev/config/mcp-configs

MCPCHECKER = $(shell pwd)/_output/tools/bin/mcpchecker
MCPCHECKER_VERSION ?= latest

# High-level knobs for local single-suite runs, e.g.:
#   make run-evals SUITE=kubevirt AGENT=claude-code MODEL=sonnet
# AGENT selects the eval config, SUITE selects the task suite label, and MODEL
# sets ANTHROPIC_MODEL for the claude-agent-acp adapter (the openai-agent ignores it).
AGENT ?= openai-agent
SUITE ?= core
MODEL ?=

# Prefer a per-suite eval config when one exists: those carry no llmJudge, so a
# local run needs no OpenAI key. Otherwise fall back to the agent's top-level
# config (the one CI uses; its llmJudge requires an OpenAI key).
EVAL_CONFIG ?= $(or $(wildcard evals/tasks/$(SUITE)/$(AGENT)/eval.yaml),evals/$(AGENT)/eval.yaml)
EVAL_LABEL_SELECTOR ?= suite=$(SUITE)
EVAL_TASK_FILTER ?=
EVAL_VERBOSE ?= false

# Download and install mcpchecker if not already installed
.PHONY: mcpchecker
mcpchecker:
	@[ -f $(MCPCHECKER) ] || { \
		set -e ;\
		echo "Installing mcpchecker $(MCPCHECKER_VERSION) to $(MCPCHECKER)..." ;\
		mkdir -p $(shell dirname $(MCPCHECKER)) ;\
		GOBIN=$(shell dirname $(MCPCHECKER)) go install github.com/mcpchecker/mcpchecker/cmd/mcpchecker@$(MCPCHECKER_VERSION) ;\
	}

##@ Evals

# Install the claude-agent-acp adapter, required by the claude-code eval agent
# (evals/claude-code/agent.yaml runs `claude-agent-acp`). No-op if already present.
.PHONY: claude-agent-acp
claude-agent-acp: ## Install the claude-agent-acp adapter for the claude-code eval agent
	@command -v claude-agent-acp >/dev/null 2>&1 || { \
		echo "Installing claude-agent-acp (npm install -g @agentclientprotocol/claude-agent-acp)..." ;\
		npm install -g @agentclientprotocol/claude-agent-acp ;\
	}

.PHONY: run-evals
run-evals: mcpchecker $(if $(filter claude-code,$(AGENT)),claude-agent-acp) ## Run mcpchecker evals (knobs: SUITE, AGENT, MODEL; see evals/README.md)
	$(if $(MODEL),ANTHROPIC_MODEL=$(MODEL) )$(MCPCHECKER) check $(EVAL_CONFIG) \
		$(if $(EVAL_LABEL_SELECTOR),--label-selector $(EVAL_LABEL_SELECTOR),) \
		$(if $(EVAL_TASK_FILTER),--run "$(EVAL_TASK_FILTER)",) \
		$(if $(filter true,$(EVAL_VERBOSE)),--verbose,) \
		--output json

.PHONY: diff-evals
diff-evals: mcpchecker ## Diff latest mcpchecker results against baseline
	@AGENT_NAME=$$(echo "$(EVAL_CONFIG)" | sed 's|evals/||; s|/eval\.yaml||'); \
	RESULTS_FILE=$$(ls -t mcpchecker-*-out.json 2>/dev/null | head -1); \
	BASELINE="evals/results/$${AGENT_NAME}-latest.json"; \
	if [ -z "$$RESULTS_FILE" ]; then \
		echo "Error: No mcpchecker results file found"; \
		exit 1; \
	fi; \
	if [ ! -f "$$BASELINE" ]; then \
		echo "No baseline results found at $$BASELINE, skipping diff"; \
		exit 0; \
	fi; \
	echo ""; \
	echo "=== Diff vs. baseline ($$BASELINE) ==="; \
	$(MCPCHECKER) diff --base "$$BASELINE" --current "$$RESULTS_FILE" --output markdown

.PHONY: run-server
run-server: build ## Start MCP server in background and wait for health check
	@echo "Starting MCP server on port $(MCP_PORT)..."
	@if [ -n "$(TOOLSETS)" ]; then \
		./$(BINARY_NAME) --port $(MCP_PORT) --toolsets $(TOOLSETS) --config-dir $(MCP_CONFIG_DIR) & echo $$! > .mcp-server.pid; \
	else \
		./$(BINARY_NAME) --port $(MCP_PORT) & echo $$! > .mcp-server.pid; \
	fi
	@echo "MCP server started with PID $$(cat .mcp-server.pid)"
	@echo "Waiting for MCP server to be ready..."
	@elapsed=0; \
	while [ $$elapsed -lt $(MCP_HEALTH_TIMEOUT) ]; do \
		if curl -fsS http://localhost:$(MCP_PORT)/healthz > /dev/null 2>&1; then \
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
