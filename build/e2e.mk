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
	E2E_SPOKE_CONTEXT=$$($(KUBECTL) --kubeconfig $(shell pwd)/_output/kubeconfig config get-contexts -o name 2>/dev/null | grep -m1 '$(MINIKUBE_SPOKE_PROFILE)' || true) \
	E2E_SPOKE_SERVER_URL=$$(SPOKE_IP=$$($(MINIKUBE) ip --profile $(MINIKUBE_SPOKE_PROFILE) 2>/dev/null) \
		&& [ -n "$$SPOKE_IP" ] && echo "https://$$SPOKE_IP:6444" || true) \
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

##@ Multicluster E2E
# Multicluster tests use two minikube clusters (hub + spoke) sharing a Keycloak
# instance. The spoke cluster's API server validates OIDC tokens with a different
# audience ("spoke" vs "openshift"), exercising per-target token exchange.
# Tests skip gracefully when E2E_SPOKE_CONTEXT is empty (single-cluster runs).

.PHONY: e2e-multicluster-setup
e2e-multicluster-setup: ## Full e2e setup with hub + spoke clusters for multicluster testing
	$(MAKE) e2e-setup
	$(MAKE) keycloak-install
	$(MAKE) minikube-create-spoke-cluster
	$(MAKE) minikube-load-image-spoke
	$(MAKE) keycloak-setup-spoke
	$(MAKE) minikube-merge-kubeconfigs

.PHONY: e2e-multicluster-teardown
e2e-multicluster-teardown: ## Delete both hub and spoke clusters
	$(MAKE) minikube-delete-spoke-cluster
	$(MAKE) minikube-delete-cluster
