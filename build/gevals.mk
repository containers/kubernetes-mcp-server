# Gevals integration for testing the MCP server

# Gevals configuration
# Look for gevals in PATH or standard locations
GEVALS_BIN ?= $(shell command -v gevals 2>/dev/null || echo gevals)
EVALS_DIR = $(shell pwd)/evals

# MCP server configuration for gevals
MCP_SERVER_PORT ?= 8008
MCP_SERVER_URL ?= http://localhost:$(MCP_SERVER_PORT)/mcp

# Keycloak configuration for OAuth tokens
KEYCLOAK_URL = https://keycloak.127-0-0-1.sslip.io:8443
KEYCLOAK_REALM = openshift
KEYCLOAK_CLIENT_ID = mcp-client
KEYCLOAK_USERNAME ?= mcp
KEYCLOAK_PASSWORD ?= mcp

# Model configuration (override via environment variables)
MODEL_NAME ?= gemini-2.0-flash

##@ Gevals

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

.PHONY: gevals-config
gevals-config: ## Generate MCP config for gevals with OAuth authentication
	@echo "Generating MCP config for gevals..."
	@mkdir -p _output/gevals
	@printf 'mcpServers:\n  kubernetes:\n    type: http\n    url: $${MCP_SERVER_URL}\n    headers:\n      Authorization: Bearer $${MCP_ACCESS_TOKEN}\n    enableAllTools: true\n' > _output/gevals/mcp-config.yaml
	@echo "✅ MCP config generated at _output/gevals/mcp-config.yaml"

.PHONY: gevals-get-token
gevals-get-token: ## Get OAuth token from Keycloak and display it
	@echo "Getting OAuth token from Keycloak..."
	@TOKEN_RESPONSE=$$(curl -sk --resolve keycloak.127-0-0-1.sslip.io:8443:127.0.0.1 \
		-X POST "$(KEYCLOAK_URL)/realms/$(KEYCLOAK_REALM)/protocol/openid-connect/token" \
		-H "Content-Type: application/x-www-form-urlencoded" \
		-d "grant_type=password" \
		-d "client_id=$(KEYCLOAK_CLIENT_ID)" \
		-d "username=$(KEYCLOAK_USERNAME)" \
		-d "password=$(KEYCLOAK_PASSWORD)" \
		-d "scope=openid mcp-server" 2>/dev/null); \
	ACCESS_TOKEN=$$(echo "$$TOKEN_RESPONSE" | jq -r '.access_token'); \
	if [ "$$ACCESS_TOKEN" = "null" ] || [ -z "$$ACCESS_TOKEN" ]; then \
		echo "❌ Failed to get token from Keycloak"; \
		echo "Response: $$TOKEN_RESPONSE" | jq; \
		exit 1; \
	fi; \
	echo "✅ OAuth token obtained"; \
	echo ""; \
	echo "Export this token to use with gevals:"; \
	echo "  export MCP_ACCESS_TOKEN=\"$$ACCESS_TOKEN\""; \
	echo ""; \
	echo "Token:"; \
	echo "$$ACCESS_TOKEN"

.PHONY: gevals-run
gevals-run: gevals-check gevals-config ## Run gevals tests against the MCP server with OAuth
	@echo "========================================="
	@echo "Running Gevals Tests"
	@echo "========================================="
	@echo ""
	@echo "Getting OAuth token from Keycloak..."
	@TOKEN_RESPONSE=$$(curl -sk --resolve keycloak.127-0-0-1.sslip.io:8443:127.0.0.1 \
		-X POST "$(KEYCLOAK_URL)/realms/$(KEYCLOAK_REALM)/protocol/openid-connect/token" \
		-H "Content-Type: application/x-www-form-urlencoded" \
		-d "grant_type=password" \
		-d "client_id=$(KEYCLOAK_CLIENT_ID)" \
		-d "username=$(KEYCLOAK_USERNAME)" \
		-d "password=$(KEYCLOAK_PASSWORD)" \
		-d "scope=openid mcp-server" 2>/dev/null); \
	ACCESS_TOKEN=$$(echo "$$TOKEN_RESPONSE" | jq -r '.access_token'); \
	if [ "$$ACCESS_TOKEN" = "null" ] || [ -z "$$ACCESS_TOKEN" ]; then \
		echo "❌ Failed to get OAuth token"; \
		echo "Response: $$TOKEN_RESPONSE" | jq; \
		exit 1; \
	fi; \
	echo "✅ Token obtained"; \
	echo ""; \
	echo "Checking if MCP server is running..."; \
	HTTP_CODE=$$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $$ACCESS_TOKEN" $(MCP_SERVER_URL)); \
	if [ "$$HTTP_CODE" != "200" ] && [ "$$HTTP_CODE" != "405" ]; then \
		echo "❌ MCP server not responding at $(MCP_SERVER_URL) (HTTP $$HTTP_CODE)"; \
		echo ""; \
		echo "Start the MCP server with:"; \
		echo "  make local-mcp-server"; \
		exit 1; \
	fi; \
	echo "✅ MCP server is running"; \
	echo ""; \
	echo "Creating MCP config with actual token..."; \
	TEMP_MCP_CONFIG=$$(mktemp); \
	printf 'mcpServers:\n  kubernetes:\n    type: http\n    url: %s\n    headers:\n      Authorization: Bearer %s\n    enableAllTools: true\n' \
		"$(MCP_SERVER_URL)" "$$ACCESS_TOKEN" > $$TEMP_MCP_CONFIG; \
	echo "Creating temporary eval config..."; \
	TEMP_EVAL=$$(mktemp); \
	trap "rm -f $$TEMP_EVAL $$TEMP_MCP_CONFIG" EXIT; \
	if [ -z "$$JUDGE_BASE_URL" ] || [ -z "$$JUDGE_API_KEY" ] || [ -z "$$JUDGE_MODEL_NAME" ]; then \
		echo "Disabling LLM judge (JUDGE_* environment variables not set)"; \
		sed -e "s|mcpConfigFile:.*|mcpConfigFile: $$TEMP_MCP_CONFIG|" \
		    -e "s|glob: ../tasks|glob: $(EVALS_DIR)/tasks|" \
		    -e 's|model: ".*"|model: "$(MODEL_NAME)"|' \
		    -e '/llmJudge:/,/modelNameKey:/d' \
			$(EVALS_DIR)/openai-agent/eval-inline.yaml > $$TEMP_EVAL; \
	else \
		echo "Enabling LLM judge"; \
		sed -e "s|mcpConfigFile:.*|mcpConfigFile: $$TEMP_MCP_CONFIG|" \
		    -e "s|glob: ../tasks|glob: $(EVALS_DIR)/tasks|" \
		    -e 's|model: ".*"|model: "$(MODEL_NAME)"|' \
			$(EVALS_DIR)/openai-agent/eval-inline.yaml > $$TEMP_EVAL; \
	fi; \
	echo ""; \
	echo "Running gevals..."; \
	echo "  Eval file: $(EVALS_DIR)/openai-agent/eval-inline.yaml"; \
	echo "  MCP config: $$TEMP_MCP_CONFIG"; \
	echo "  MCP server: $(MCP_SERVER_URL)"; \
	echo "  Model: $(MODEL_NAME)"; \
	echo ""; \
	$(GEVALS_BIN) eval $$TEMP_EVAL

.PHONY: gevals-run-claude
gevals-run-claude: gevals-check gevals-config ## Run gevals tests with Claude Code agent
	@echo "========================================="
	@echo "Running Gevals Tests (Claude Code)"
	@echo "========================================="
	@echo ""
	@echo "Getting OAuth token from Keycloak..."
	@TOKEN_RESPONSE=$$(curl -sk --resolve keycloak.127-0-0-1.sslip.io:8443:127.0.0.1 \
		-X POST "$(KEYCLOAK_URL)/realms/$(KEYCLOAK_REALM)/protocol/openid-connect/token" \
		-H "Content-Type: application/x-www-form-urlencoded" \
		-d "grant_type=password" \
		-d "client_id=$(KEYCLOAK_CLIENT_ID)" \
		-d "username=$(KEYCLOAK_USERNAME)" \
		-d "password=$(KEYCLOAK_PASSWORD)" \
		-d "scope=openid mcp-server" 2>/dev/null); \
	ACCESS_TOKEN=$$(echo "$$TOKEN_RESPONSE" | jq -r '.access_token'); \
	if [ "$$ACCESS_TOKEN" = "null" ] || [ -z "$$ACCESS_TOKEN" ]; then \
		echo "❌ Failed to get OAuth token"; \
		echo "Response: $$TOKEN_RESPONSE" | jq; \
		exit 1; \
	fi; \
	echo "✅ Token obtained"; \
	echo ""; \
	echo "Checking if MCP server is running..."; \
	HTTP_CODE=$$(curl -s -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $$ACCESS_TOKEN" $(MCP_SERVER_URL)); \
	if [ "$$HTTP_CODE" != "200" ] && [ "$$HTTP_CODE" != "405" ]; then \
		echo "❌ MCP server not responding at $(MCP_SERVER_URL) (HTTP $$HTTP_CODE)"; \
		echo ""; \
		echo "Start the MCP server with:"; \
		echo "  make local-mcp-server"; \
		exit 1; \
	fi; \
	echo "✅ MCP server is running"; \
	echo ""; \
	echo "Creating MCP config with actual token..."; \
	TEMP_MCP_CONFIG=$$(mktemp); \
	printf 'mcpServers:\n  kubernetes:\n    type: http\n    url: %s\n    headers:\n      Authorization: Bearer %s\n    enableAllTools: true\n' \
		"$(MCP_SERVER_URL)" "$$ACCESS_TOKEN" > $$TEMP_MCP_CONFIG; \
	echo "Creating temporary eval config..."; \
	TEMP_EVAL=$$(mktemp); \
	trap "rm -f $$TEMP_EVAL $$TEMP_MCP_CONFIG" EXIT; \
	if [ -z "$$JUDGE_BASE_URL" ] || [ -z "$$JUDGE_API_KEY" ] || [ -z "$$JUDGE_MODEL_NAME" ]; then \
		echo "Disabling LLM judge (JUDGE_* environment variables not set)"; \
		sed -e "s|mcpConfigFile:.*|mcpConfigFile: $$TEMP_MCP_CONFIG|" \
		    -e "s|glob: ../tasks|glob: $(EVALS_DIR)/tasks|" \
		    -e '/llmJudge:/,/modelNameKey:/d' \
			$(EVALS_DIR)/claude-code/eval-inline.yaml > $$TEMP_EVAL; \
	else \
		echo "Enabling LLM judge"; \
		sed -e "s|mcpConfigFile:.*|mcpConfigFile: $$TEMP_MCP_CONFIG|" \
		    -e "s|glob: ../tasks|glob: $(EVALS_DIR)/tasks|" \
			$(EVALS_DIR)/claude-code/eval-inline.yaml > $$TEMP_EVAL; \
	fi; \
	echo ""; \
	echo "Running gevals with Claude Code..."; \
	echo "  Eval file: $(EVALS_DIR)/claude-code/eval-inline.yaml"; \
	echo "  MCP config: $$TEMP_MCP_CONFIG"; \
	echo "  MCP server: $(MCP_SERVER_URL)"; \
	echo ""; \
	$(GEVALS_BIN) eval $$TEMP_EVAL

.PHONY: local-mcp-server
local-mcp-server: build ## Start the MCP server locally with OAuth enabled
	@echo "========================================="
	@echo "Starting MCP Server"
	@echo "========================================="
	@echo ""
	@echo "Server will be available at: $(MCP_SERVER_URL)"
	@echo ""
	@echo "Press Ctrl+C to stop the server"
	@echo ""
	./$(BINARY_NAME) --port $(MCP_SERVER_PORT) --config _output/config.toml
