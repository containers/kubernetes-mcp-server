##@ Node/NPM build targets

NPM_PACKAGE = kubernetes-mcp-server

CLEAN_TARGETS += $(foreach os,$(OSES),$(foreach arch,$(ARCHS),./npm/$(BINARY_NAME)-$(os)-$(arch)))
CLEAN_TARGETS += ./npm/$(NPM_PACKAGE)/LICENSE ./npm/$(NPM_PACKAGE)/package.json ./npm/$(NPM_PACKAGE)/README.md

.PHONY: npm-copy-binaries
npm-copy-binaries: build-all-platforms ## Copy the binaries to each npm package
	$(foreach os,$(OSES),$(foreach arch,$(ARCHS), \
		EXECUTABLE=./$(BINARY_NAME)-$(os)-$(arch)$(if $(findstring windows,$(os)),.exe,); \
		NPM_EXECUTABLE=$(NPM_PACKAGE)-$(os)-$(arch)$(if $(findstring windows,$(os)),.exe,); \
		DIRNAME=$(NPM_PACKAGE)-$(os)-$(arch); \
		mkdir -p ./npm/$$DIRNAME/bin; \
		cp $$EXECUTABLE ./npm/$$DIRNAME/bin/$$NPM_EXECUTABLE; \
	))


MAIN_PACKAGE_JSON=./npm/$(NPM_PACKAGE)/package.json
.PHONY: npm-copy-project-files
npm-copy-project-files: npm-copy-binaries ## Copy the project files to the main npm package and generate all package.json files
	cp README.md LICENSE ./npm/$(NPM_PACKAGE)/
	@echo '{"name": "$(NPM_PACKAGE)",' > $(MAIN_PACKAGE_JSON)
	@echo '"version": "$(GIT_TAG_VERSION)",' >> $(MAIN_PACKAGE_JSON)
	@echo '"description": "Model Context Protocol (MCP) server for Kubernetes and OpenShift",' >> $(MAIN_PACKAGE_JSON)
	@echo '"main": "./bin/index.js",' >> $(MAIN_PACKAGE_JSON)
	@echo '"bin": {"$(NPM_PACKAGE)": "bin/index.js"},' >> $(MAIN_PACKAGE_JSON)
	@echo '"optionalDependencies": {' >> $(MAIN_PACKAGE_JSON)
	@for os in $(OSES); do \
		for arch in $(ARCHS); do \
			if [ "$$os" = "$(lastword $(OSES))" ] && [ "$$arch" = "$(lastword $(ARCHS))" ]; then \
				echo "  \"$(NPM_PACKAGE)-$$os-$$arch\": \"$(GIT_TAG_VERSION)\""; \
			else \
				echo "  \"$(NPM_PACKAGE)-$$os-$$arch\": \"$(GIT_TAG_VERSION)\","; \
			fi \
		done; \
	done >> $(MAIN_PACKAGE_JSON)
	@echo '},' >> $(MAIN_PACKAGE_JSON)
	@echo '"repository": {"type": "git", "url": "git+https://github.com/containers/kubernetes-mcp-server.git"},' >> $(MAIN_PACKAGE_JSON)
	@echo '"keywords": ["mcp","kubernetes","openshift","model context protocol","model","context","protocol"],' >> $(MAIN_PACKAGE_JSON)
	@echo '"author": {"name": "Marc Nuri", "url": "https://www.marcnuri.com"},' >> $(MAIN_PACKAGE_JSON)
	@echo '"license": "Apache-2.0",' >> $(MAIN_PACKAGE_JSON)
	@echo '"bugs": {"url": "https://github.com/containers/kubernetes-mcp-server/issues"},' >> $(MAIN_PACKAGE_JSON)
	@echo '"homepage": "https://github.com/containers/kubernetes-mcp-server#readme",' >> $(MAIN_PACKAGE_JSON)
	@echo '"mcpName": "io.github.containers/kubernetes-mcp-server"' >> $(MAIN_PACKAGE_JSON)
	@echo '}' >> $(MAIN_PACKAGE_JSON)
	$(foreach os,$(OSES),$(foreach arch,$(ARCHS), \
		OS_PACKAGE_JSON=./npm/$(NPM_PACKAGE)-$(os)-$(arch)/package.json; \
		echo '{"name": "$(NPM_PACKAGE)-$(os)-$(arch)",' > $$OS_PACKAGE_JSON; \
		echo '"version": "$(GIT_TAG_VERSION)",' >> $$OS_PACKAGE_JSON; \
		echo '"description": "Model Context Protocol (MCP) server for Kubernetes and OpenShift",' >> $$OS_PACKAGE_JSON; \
		echo '"repository": {"type": "git", "url": "git+https://github.com/containers/kubernetes-mcp-server.git"},' >> $$OS_PACKAGE_JSON; \
		OS="$(os)"; \
		if [ "$$OS" = "windows" ]; then OS="win32"; fi; \
		echo '"os": ["'$$OS'"],' >> $$OS_PACKAGE_JSON; \
		NPM_ARCH="$(arch)"; \
		if [ "$$NPM_ARCH" = "amd64" ]; then NPM_ARCH="x64"; fi; \
		echo '"cpu": ["'$$NPM_ARCH'"]' >> $$OS_PACKAGE_JSON; \
		echo '}' >> $$OS_PACKAGE_JSON; \
	))

.PHONY: npm-publish
npm-publish: npm-copy-project-files ## Publish the npm packages
	$(foreach os,$(OSES),$(foreach arch,$(ARCHS), \
		DIRNAME="$(BINARY_NAME)-$(os)-$(arch)"; \
		cd npm/$$DIRNAME; \
		jq '.version = "$(GIT_TAG_VERSION)"' package.json > tmp.json && mv tmp.json package.json; \
		npm publish --tag latest; \
		cd ../..; \
	))
	cp README.md LICENSE ./npm/$(NPM_PACKAGE)/
	jq '.version = "$(GIT_TAG_VERSION)"' ./npm/$(NPM_PACKAGE)/package.json > tmp.json && mv tmp.json ./npm/$(NPM_PACKAGE)/package.json; \
	jq '.optionalDependencies |= with_entries(.value = "$(GIT_TAG_VERSION)")' ./npm/$(NPM_PACKAGE)/package.json > tmp.json && mv tmp.json ./npm/$(NPM_PACKAGE)/package.json; \
	cd npm/$(NPM_PACKAGE) && npm publish --tag latest

