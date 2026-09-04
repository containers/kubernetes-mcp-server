# Evals - mcpchecker evaluation support

MCP_PORT ?= 8080
MCP_HEALTH_TIMEOUT ?= 60
MCP_HEALTH_INTERVAL ?= 2
MCP_CONFIG_DIR ?= dev/config/mcp-configs

MCPCHECKER = $(shell pwd)/_output/tools/bin/mcpchecker
MCPCHECKER_VERSION ?= latest
CLAUDE_AGENT_ACP = $(shell pwd)/_output/tools/node_modules/.bin/claude-agent-acp
CLAUDE_AGENT_ACP_VERSION ?= latest
JQ = $(shell pwd)/_output/tools/bin/jq
JQ_VERSION ?= 1.7.1
# Derive the jq release asset for the host from the Go toolchain (already a
# dependency of this eval flow). jq names assets macos/i386 where Go says
# darwin/386, and suffixes Windows binaries with .exe.
JQ_GO_OS ?= $(shell go env GOHOSTOS)
JQ_GO_ARCH ?= $(shell go env GOHOSTARCH)
JQ_OS = $(patsubst darwin,macos,$(JQ_GO_OS))
JQ_ARCH = $(patsubst 386,i386,$(JQ_GO_ARCH))
JQ_ASSET = jq-$(JQ_OS)-$(JQ_ARCH)$(if $(filter windows,$(JQ_GO_OS)),.exe,)

# High-level knobs for local single-suite runs, e.g.:
#   make run-evals SUITE=kubevirt AGENT=acp-anthropic MODEL=sonnet
# AGENT selects the agent directory under evals/, SUITE selects the task suite
# label, and MODEL sets ANTHROPIC_MODEL for ACP agents (builtin agents ignore it).
# Available agents: builtin-openai, builtin-anthropic, builtin-google,
#                   acp-anthropic (Claude Code via ACP), acp-google (Gemini via ACP)
AGENT ?= builtin-openai
SUITE ?= core
MODEL ?=

# Prefer a per-suite eval config when one exists, then try the core-eval-testing
# suite config (what CI uses).
EVAL_CONFIG ?= $(or $(wildcard evals/tasks/$(SUITE)/$(AGENT)/eval.yaml),evals/core-eval-testing/$(AGENT)/eval-$(SUITE).yaml)
EVAL_LABEL_SELECTOR ?= suite=$(SUITE)
EVAL_TASK_FILTER ?=
EVAL_VERBOSE ?= false

# Download and install jq static binary if not already installed
.PHONY: jq
jq:
	@[ -f $(JQ) ] || { \
		set -e ;\
		echo "Installing jq $(JQ_VERSION) ($(JQ_ASSET)) to $(JQ)..." ;\
		mkdir -p $(shell dirname $(JQ)) ;\
		curl -fsSL "https://github.com/jqlang/jq/releases/download/jq-$(JQ_VERSION)/$(JQ_ASSET)" \
			-o $(JQ) ;\
		chmod +x $(JQ) ;\
	}

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

# Install the claude-agent-acp adapter locally under _output/tools, required by
# the acp-anthropic eval agent (runs `claude-agent-acp`).
.PHONY: claude-agent-acp
claude-agent-acp: ## Install the claude-agent-acp adapter for the acp-anthropic eval agent
	@[ -f $(CLAUDE_AGENT_ACP) ] || { \
		set -e ;\
		echo "Installing claude-agent-acp@$(CLAUDE_AGENT_ACP_VERSION) to $(CLAUDE_AGENT_ACP)..." ;\
		npm install --prefix $(shell pwd)/_output/tools @agentclientprotocol/claude-agent-acp@$(CLAUDE_AGENT_ACP_VERSION) ;\
		echo "✅ claude-agent-acp installed" ;\
	}

.PHONY: run-evals
run-evals: mcpchecker jq $(if $(filter acp-anthropic,$(AGENT)),claude-agent-acp) ## Run mcpchecker evals (knobs: SUITE, AGENT, MODEL; see evals/README.md)
	@# Prefer MCP_EVAL_KUBECONFIG when KUBECONFIG is unset so setup/verify kubectl
	@# targets the same cluster as make run-server (avoids alabama/.kube/config).
	@if [ -z "$${KUBECONFIG:-}" ] && [ -n "$(MCP_EVAL_KUBECONFIG)" ]; then \
		export KUBECONFIG="$(MCP_EVAL_KUBECONFIG)"; \
	fi; \
	$(if $(MODEL),ANTHROPIC_MODEL=$(MODEL) )PATH="$(shell pwd)/_output/tools/bin:$(shell pwd)/_output/tools/node_modules/.bin:$${PATH}" \
		$(MCPCHECKER) check $(EVAL_CONFIG) \
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
	@# When MCP_EVAL_KUBECONFIG is set (OCP CI / local evals), force the kubeconfig
	@# provider so in-cluster env cannot steal traffic and cause RBAC denials.
	./$(BINARY_NAME) --port $(MCP_PORT) $(if $(TOOLSETS),--toolsets "$(TOOLSETS)") --config-dir $(MCP_CONFIG_DIR) $(if $(MCP_EVAL_KUBECONFIG),--kubeconfig "$(MCP_EVAL_KUBECONFIG)" --cluster-provider kubeconfig) & echo $$! > .mcp-server.pid
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
