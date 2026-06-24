##@ E2E Tests

UV = $(shell pwd)/_output/tools/bin/uv
UV_VERSION = 0.7.15
E2E_IMAGE ?= localhost/kubernetes-mcp-server:e2e
E2E_DIR = test/e2e

.PHONY: uv
uv:
	@[ -f $(UV) ] || { \
		set -e ;\
		echo "Installing uv $(UV_VERSION) to $$(dirname $(UV))..." ;\
		mkdir -p $$(dirname $(UV)) ;\
		curl -LsSf https://astral.sh/uv/$(UV_VERSION)/install.sh | env UV_INSTALL_DIR=$$(dirname $(UV)) INSTALLER_NO_MODIFY_PATH=1 sh ;\
	}

.PHONY: e2e-image
e2e-image: ## Build the e2e container image and load it into the Kind cluster
	$(CONTAINER_ENGINE) build -t $(E2E_IMAGE) .
	$(KIND) load docker-image $(E2E_IMAGE) --name $(KIND_CLUSTER_NAME)

.PHONY: e2e-test
e2e-test: uv ## Run all e2e tests
	MCP_SERVER_IMAGE=$(E2E_IMAGE) $(UV) run --directory $(E2E_DIR) --locked pytest -v $(PYTEST_ARGS)
