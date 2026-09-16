##@ E2E Tests

E2E_IMAGE ?= localhost/kubernetes-mcp-server:e2e

.PHONY: e2e-image
e2e-image: ## Build the e2e container image
	$(CONTAINER_ENGINE) build -t $(E2E_IMAGE) .

.PHONY: e2e-setup
e2e-setup: e2e-image minikube-create-cluster helm kubectl ## Create cluster, build and load image
	$(MAKE) minikube-load-image

.PHONY: e2e-setup-kuadrant
e2e-setup-kuadrant: e2e-setup ## E2E setup including Kuadrant stack
	$(MAKE) kuadrant-setup

.PHONY: e2e-test
e2e-test: helm kubectl ## Run all e2e tests against existing cluster
	KUBECONFIG=$(shell pwd)/_output/kubeconfig \
	KUBECTL_PATH=$(KUBECTL) \
	HELM_PATH=$(HELM) \
	MCP_SERVER_IMAGE=$(E2E_IMAGE) \
	go test -tags e2e -v -count=1 -timeout 20m ./test/e2e/ $(E2E_ARGS)

.PHONY: e2e-teardown
e2e-teardown: ## Delete the e2e Minikube cluster
	$(MAKE) minikube-delete-cluster

.PHONY: e2e-full-setup
e2e-full-setup: ## Full e2e setup with all components (cluster, image, cert-manager, Keycloak, Kuadrant, Tempo)
	$(MAKE) e2e-setup
	$(MAKE) keycloak-install
	$(MAKE) kuadrant-setup
	$(MAKE) tempo-install
