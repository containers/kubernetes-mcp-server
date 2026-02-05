##@ Helm Chart build targets

HELM_CHART_DIR = ./charts/kubernetes-mcp-server
HELM_CHART_VERSION_BASE = $(shell grep '^version:' $(HELM_CHART_DIR)/Chart.yaml | awk '{print $$2}')
HELM_CHART_VERSION ?= $(HELM_CHART_VERSION_BASE)
HELM_PACKAGE_DIR = ./_output/helm-packages
HELM_REGISTRY ?= ghcr.io
HELM_REGISTRY_ORG ?= containers
HELM_CHART_NAME = kubernetes-mcp-server

KUBECONFORM = $(shell pwd)/_output/tools/bin/kubeconform
KUBECONFORM_VERSION ?= latest

CLEAN_TARGETS += $(HELM_PACKAGE_DIR)

.PHONY: helm-lint
helm-lint: ## Lint the Helm chart
	helm lint $(HELM_CHART_DIR)

.PHONY: helm-template
helm-template: ## Render Helm chart templates (dry run)
	helm template test-release $(HELM_CHART_DIR) --set ingress.host=localhost --debug

# Download and install kubeconform if not already installed
.PHONY: kubeconform
kubeconform:
	@[ -f $(KUBECONFORM) ] || { \
		set -e ;\
		echo "Installing kubeconform to $(KUBECONFORM)..." ;\
		mkdir -p $(shell dirname $(KUBECONFORM)) ;\
		TMPDIR=$$(mktemp -d) ;\
		curl -L https://github.com/yannh/kubeconform/releases/$(KUBECONFORM_VERSION)/download/kubeconform-$(shell uname -s | tr '[:upper:]' '[:lower:]')-$(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz | tar xz -C $$TMPDIR ;\
		mv $$TMPDIR/kubeconform $(KUBECONFORM) ;\
		rm -rf $$TMPDIR ;\
	}

.PHONY: helm-validate
helm-validate: kubeconform ## Validate Helm chart manifests with kubeconform
	@echo "Validating with default values..."
	@bash -o pipefail -c 'helm template test-release $(HELM_CHART_DIR) --set ingress.host=localhost | $(KUBECONFORM) -strict -summary -ignore-missing-schemas'
	@echo ""
	@echo "Validating with tpl-exercising values..."
	@bash -o pipefail -c 'helm template test-release $(HELM_CHART_DIR) -f $(HELM_CHART_DIR)/ci/tpl-test-values.yaml | $(KUBECONFORM) -strict -summary -ignore-missing-schemas'

.PHONY: helm-package
helm-package: helm-lint helm-template ## Package the Helm chart (supports HELM_CHART_VERSION override)
	@mkdir -p $(HELM_PACKAGE_DIR)
	@echo "Updating Chart.yaml for packaging..."
	@sed -i.bak -e "s/version: .*/version: $(HELM_CHART_VERSION)/" \
	             -e "s/appVersion: .*/appVersion: \"$(GIT_TAG_VERSION)\"/" \
	             $(HELM_CHART_DIR)/Chart.yaml
	@echo "Updated Chart.yaml:"
	@cat $(HELM_CHART_DIR)/Chart.yaml
	helm package $(HELM_CHART_DIR) --destination $(HELM_PACKAGE_DIR)
	@mv $(HELM_CHART_DIR)/Chart.yaml.bak $(HELM_CHART_DIR)/Chart.yaml
	@echo "Chart packaged as version $(HELM_CHART_VERSION)"

.PHONY: helm-push
helm-push: helm-package ## Push Helm chart to OCI registry (assumes helm registry login has been performed)
	@chart_package=$$(ls $(HELM_PACKAGE_DIR)/$(HELM_CHART_NAME)-*.tgz 2>/dev/null | head -n 1); \
	if [ -z "$$chart_package" ]; then echo "Error: No chart package found in $(HELM_PACKAGE_DIR)"; exit 1; fi; \
	echo "Pushing chart package: $$chart_package"; \
	helm push "$$chart_package" oci://$(HELM_REGISTRY)/$(HELM_REGISTRY_ORG)/charts


.PHONY: helm-verify
helm-verify: ## Verify chart installation from OCI registry
	@echo "Testing chart template rendering from OCI registry..."
	helm template test-install oci://$(HELM_REGISTRY)/$(HELM_REGISTRY_ORG)/charts/$(HELM_CHART_NAME) \
		--set ingress.host=localhost --version $(HELM_CHART_VERSION) --debug

.PHONY: helm-publish
helm-publish: helm-package helm-push helm-verify ## Package, push, and verify Helm chart release
	@echo "Helm chart $(HELM_CHART_NAME) version $(HELM_CHART_VERSION) published successfully"

# Print the Helm chart version
.PHONY: helm-print-chart-version
helm-print-chart-version:
	@echo $(HELM_CHART_VERSION)

# Print the Helm chart name
.PHONY: helm-print-chart-name
helm-print-chart-name:
	@echo $(HELM_CHART_NAME)

# Print the Helm registry
.PHONY: helm-print-registry
helm-print-registry:
	@echo $(HELM_REGISTRY)
