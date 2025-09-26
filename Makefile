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
GOLANGCI_LINT_VERSION ?= v2.2.2

# NPM version should not append the -dirty flag
NPM_VERSION ?= $(shell echo $(shell git describe --tags --always) | sed 's/^v//')
OSES = darwin linux windows
ARCHS = amd64 arm64

CLEAN_TARGETS :=
CLEAN_TARGETS += '$(BINARY_NAME)'
CLEAN_TARGETS += $(foreach os,$(OSES),$(foreach arch,$(ARCHS),$(BINARY_NAME)-$(os)-$(arch)$(if $(findstring windows,$(os)),.exe,)))
CLEAN_TARGETS += $(foreach os,$(OSES),$(foreach arch,$(ARCHS),./npm/$(BINARY_NAME)-$(os)-$(arch)/bin/))
CLEAN_TARGETS += ./npm/kubernetes-mcp-server/.npmrc ./npm/kubernetes-mcp-server/LICENSE ./npm/kubernetes-mcp-server/README.md
CLEAN_TARGETS += $(foreach os,$(OSES),$(foreach arch,$(ARCHS),./npm/$(BINARY_NAME)-$(os)-$(arch)/.npmrc))

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

.PHONY: npm-copy-binaries
npm-copy-binaries: build-all-platforms ## Copy the binaries to each npm package
	$(foreach os,$(OSES),$(foreach arch,$(ARCHS), \
		EXECUTABLE=./$(BINARY_NAME)-$(os)-$(arch)$(if $(findstring windows,$(os)),.exe,); \
		DIRNAME=$(BINARY_NAME)-$(os)-$(arch); \
		mkdir -p ./npm/$$DIRNAME/bin; \
		cp $$EXECUTABLE ./npm/$$DIRNAME/bin/; \
	))

.PHONY: npm-publish
npm-publish: npm-copy-binaries ## Publish the npm packages
	$(foreach os,$(OSES),$(foreach arch,$(ARCHS), \
		DIRNAME="$(BINARY_NAME)-$(os)-$(arch)"; \
		cd npm/$$DIRNAME; \
		echo '//registry.npmjs.org/:_authToken=$(NPM_TOKEN)' >> .npmrc; \
		jq '.version = "$(NPM_VERSION)"' package.json > tmp.json && mv tmp.json package.json; \
		npm publish; \
		cd ../..; \
	))
	cp README.md LICENSE ./npm/kubernetes-mcp-server/
	echo '//registry.npmjs.org/:_authToken=$(NPM_TOKEN)' >> ./npm/kubernetes-mcp-server/.npmrc
	jq '.version = "$(NPM_VERSION)"' ./npm/kubernetes-mcp-server/package.json > tmp.json && mv tmp.json ./npm/kubernetes-mcp-server/package.json; \
	jq '.optionalDependencies |= with_entries(.value = "$(NPM_VERSION)")' ./npm/kubernetes-mcp-server/package.json > tmp.json && mv tmp.json ./npm/kubernetes-mcp-server/package.json; \
	cd npm/kubernetes-mcp-server && npm publish

.PHONY: python-publish
python-publish: ## Publish the python packages
	cd ./python && \
	sed -i "s/version = \".*\"/version = \"$(NPM_VERSION)\"/" pyproject.toml && \
	uv build && \
	uv publish

.PHONY: test
test: ## Run the tests
	go test -count=1 -v ./...

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
