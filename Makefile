# If you update this file, please follow
# https://suva.sh/posts/well-documented-makefiles

.DEFAULT_GOAL := help

PACKAGE = $(shell go list -m)
GIT_COMMIT_HASH = $(shell git rev-parse HEAD)
GIT_VERSION = $(shell git describe --tags --always --dirty)
BUILD_TIME = $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
BINARY_NAME = kubernetes-mcp-server
LD_FLAGS = -s -w \
	-X '$(PACKAGE)/pkg/version.CommitHash=$(GIT_COMMIT_HASH)' \
	-X '$(PACKAGE)/pkg/version.Version=$(GIT_VERSION)' \
	-X '$(PACKAGE)/pkg/version.BuildTime=$(BUILD_TIME)' \
	-X '$(PACKAGE)/pkg/version.BinaryName=$(BINARY_NAME)'
COMMON_BUILD_ARGS = -ldflags "$(LD_FLAGS)"

GOLANGCI_LINT = $(shell pwd)/_output/tools/bin/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.6.1

# NPM version should not append the -dirty flag
GIT_TAG_VERSION ?= $(shell echo $(shell git describe --tags --always) | sed 's/^v//')
OSES = darwin linux windows
ARCHS = amd64 arm64

CLEAN_TARGETS :=
CLEAN_TARGETS += '$(BINARY_NAME)'
CLEAN_TARGETS += $(foreach os,$(OSES),$(foreach arch,$(ARCHS),$(BINARY_NAME)-$(os)-$(arch)$(if $(findstring windows,$(os)),.exe,)))

# The help will print out all targets with their descriptions organized bellow their categories. The categories are represented by `##@` and the target descriptions by `##`.
# The awk commands is responsible to read the entire set of makefiles included in this invocation, looking for lines of the file as xyz: ## something, and then pretty-format the target and help. Then, if there's a line with ##@ something, that gets pretty-printed as a category.
# More info over the usage of ANSI control characters for terminal formatting: https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info over awk command: http://linuxcommand.org/lc3_adv_awk.php
#
# Notice that we have a little modification on the awk command to support slash in the recipe name:
# origin: /^[a-zA-Z_0-9-]+:.*?##/
# modified /^[a-zA-Z_0-9\/\.-]+:.*?##/
.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9\/\.-]+:.*?##/ { printf "  \033[36m%-21s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: clean
clean: ## Clean up all build artifacts
	rm -rf $(CLEAN_TARGETS)

.PHONY: build
build: clean tidy format lint ## Build the project
	go build $(COMMON_BUILD_ARGS) -o $(BINARY_NAME) ./cmd/kubernetes-mcp-server


.PHONY: build-all-platforms
build-all-platforms: clean tidy format lint ## Build the project for all platforms
	$(foreach os,$(OSES),$(foreach arch,$(ARCHS), \
		GOOS=$(os) GOARCH=$(arch) go build $(COMMON_BUILD_ARGS) -o $(BINARY_NAME)-$(os)-$(arch)$(if $(findstring windows,$(os)),.exe,) ./cmd/kubernetes-mcp-server; \
	))

.PHONY: test
test: ## Run the tests
	go test -count=1 -v ./...

.PHONY: test-update-snapshots
test-update-snapshots: ## Update test snapshots for toolset tests
	UPDATE_TOOLSETS_JSON=1 go test -count=1 -v ./pkg/mcp

.PHONY: format
format: ## Format the code
	go fmt ./...

.PHONY: tidy
tidy: ## Tidy up the go modules
	go mod tidy

.PHONY: golangci-lint
golangci-lint: ## Download and install golangci-lint if not already installed
		@[ -f $(GOLANGCI_LINT) ] || { \
    	set -e ;\
    	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell dirname $(GOLANGCI_LINT)) $(GOLANGCI_LINT_VERSION) ;\
    	}

.PHONY: lint
lint: golangci-lint ## Lint the code
	$(GOLANGCI_LINT) run --verbose --print-resources-usage

.PHONY: update-readme-tools
update-readme-tools: ## Update the README.md file with the latest toolsets
	go run ./internal/tools/update-readme/main.go README.md

##@ Tools

.PHONY: tools
tools: ## Install all required tools (kind) to ./_output/bin/
	@echo "Checking and installing required tools to ./_output/bin/ ..."
	@if [ -f _output/bin/kind ]; then echo "[OK] kind already installed"; else echo "Installing kind..."; $(MAKE) -s kind; fi
	@echo "All tools ready!"

##@ Local Development

.PHONY: local-env-setup
local-env-setup: ## Setup complete local development environment with Kind cluster
	@echo "========================================="
	@echo "Kubernetes MCP Server - Local Setup"
	@echo "========================================="
	$(MAKE) tools
	$(MAKE) kind-create-cluster
	$(MAKE) keycloak-install
	$(MAKE) build
	@echo ""
	@echo "========================================="
	@echo "Local environment ready!"
	@echo "========================================="
	@echo ""
	@echo "Configuration file generated:"
	@echo "  _output/config.toml"
	@echo ""
	@echo "Run the MCP server with:"
	@echo "  ./$(BINARY_NAME) --port 8008 --config _output/config.toml"
	@echo ""
	@echo "Or run with MCP inspector:"
	@echo "  npx @modelcontextprotocol/inspector@latest \$$(pwd)/$(BINARY_NAME) --config _output/config.toml"

.PHONY: local-env-teardown
local-env-teardown: ## Tear down the local Kind cluster
	$(MAKE) kind-delete-cluster

# Include build configuration files
-include build/*.mk
