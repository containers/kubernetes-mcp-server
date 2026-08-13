# Keycloak IdP for development and testing

KEYCLOAK_NAMESPACE = keycloak
KEYCLOAK_ADMIN_USER = admin
KEYCLOAK_ADMIN_PASSWORD = admin

##@ Keycloak

.PHONY: keycloak-gen-sts-keypair
keycloak-gen-sts-keypair: ## Generate the throwaway D4 STS-assertion keypair (gitignored, never committed)
	@dev/config/keycloak/gen-sts-assertion-keypair.sh

.PHONY: keycloak-install
keycloak-install: minikube kubectl install-cert-manager keycloak-gen-sts-keypair ## Install Keycloak with pre-configured OpenShift realm
	@echo "Installing Keycloak..."
	@$(KUBECTL) create namespace $(KEYCLOAK_NAMESPACE) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	@echo "Rendering realm with the generated STS-assertion cert..."
	@mkdir -p _output/keycloak
	@sed "s|@@STS_ASSERTION_CERT_DER@@|$$(grep -v -- '-----' test/e2e/testdata/generated/sts-assertion.crt | tr -d '\n')|" \
		dev/config/keycloak/realm-import.yaml > _output/keycloak/realm-import.rendered.yaml
	@$(KUBECTL) apply -f _output/keycloak/realm-import.rendered.yaml
	@$(KUBECTL) apply -f dev/config/keycloak/deployment.yaml
	@echo "Restarting Keycloak to re-import the realm..."
	@$(KUBECTL) rollout restart deployment/keycloak -n $(KEYCLOAK_NAMESPACE)
	@echo "Waiting for Keycloak TLS certificate to be issued..."
	@$(KUBECTL) wait --for=condition=ready certificate/keycloak-tls -n $(KEYCLOAK_NAMESPACE) --timeout=120s
	@echo "Waiting for Keycloak to be ready..."
	@$(KUBECTL) rollout status deployment/keycloak -n $(KEYCLOAK_NAMESPACE) --timeout=180s
	@echo "Extracting cert-manager CA certificate..."
	@mkdir -p _output/cert-manager-ca
	@$(KUBECTL) get secret selfsigned-ca-secret -n cert-manager -o jsonpath='{.data.ca\.crt}' | base64 -d > _output/cert-manager-ca/ca.crt
	@echo "Copying CA certificate into Minikube node..."
	@$(MINIKUBE) cp _output/cert-manager-ca/ca.crt $(MINIKUBE_PROFILE):/var/lib/minikube/certs/keycloak-ca.crt --profile $(MINIKUBE_PROFILE)
	@echo "Adding Keycloak DNS entry to Minikube node /etc/hosts..."
	@KEYCLOAK_CLUSTER_IP=$$($(KUBECTL) get svc keycloak -n $(KEYCLOAK_NAMESPACE) -o jsonpath='{.spec.clusterIP}'); \
		$(MINIKUBE) ssh --profile $(MINIKUBE_PROFILE) -- \
			"grep -v keycloak.keycloak.svc /etc/hosts | sudo tee /etc/hosts.tmp > /dev/null && sudo cp /etc/hosts.tmp /etc/hosts && sudo rm /etc/hosts.tmp && echo \"$$KEYCLOAK_CLUSTER_IP keycloak.keycloak.svc\" | sudo tee -a /etc/hosts > /dev/null"
	@echo "Restarting API server with OIDC CA..."
	@$(MINIKUBE) start --profile $(MINIKUBE_PROFILE) \
		--extra-config=apiserver.oidc-issuer-url=https://keycloak.keycloak.svc:8443/realms/openshift \
		--extra-config=apiserver.oidc-client-id=openshift \
		--extra-config=apiserver.oidc-username-claim=preferred_username \
		--extra-config=apiserver.oidc-groups-claim=groups \
		--extra-config=apiserver.oidc-ca-file=/var/lib/minikube/certs/keycloak-ca.crt
	@echo "Re-exporting kubeconfig..."
	@$(MINIKUBE) kubectl --profile $(MINIKUBE_PROFILE) -- config view --flatten > _output/kubeconfig
	@$(KUBECTL) apply -f dev/config/keycloak/rbac.yaml
	@mkdir -p _output
	@cp dev/config/keycloak/config.toml _output/config.toml
	@echo ""
	@echo "Keycloak installed and configured!"
	@echo "  Admin console: make keycloak-port-forward, then https://localhost:8443"
	@echo "  Test user: mcp / mcp"
	@echo "  Config: _output/config.toml"

.PHONY: keycloak-uninstall
keycloak-uninstall: kubectl ## Uninstall Keycloak
	@$(KUBECTL) delete -f dev/config/keycloak/rbac.yaml --ignore-not-found || true
	@$(KUBECTL) delete -f dev/config/keycloak/deployment.yaml --ignore-not-found || true
	@$(KUBECTL) delete -f dev/config/keycloak/realm-import.yaml --ignore-not-found || true

.PHONY: keycloak-status
keycloak-status: kubectl ## Show Keycloak status and connection info
	@if $(KUBECTL) get svc -n $(KEYCLOAK_NAMESPACE) keycloak >/dev/null 2>&1; then \
		echo "========================================"; \
		echo "Keycloak Status: Installed"; \
		echo "========================================"; \
		echo ""; \
		echo "Admin Console: make keycloak-port-forward, then https://localhost:8443"; \
		echo "  Username: $(KEYCLOAK_ADMIN_USER)"; \
		echo "  Password: $(KEYCLOAK_ADMIN_PASSWORD)"; \
		echo ""; \
		echo "OIDC Discovery: https://keycloak.keycloak.svc:8443/realms/openshift/.well-known/openid-configuration"; \
		echo "========================================"; \
	else \
		echo "Keycloak is not installed. Run: make keycloak-install"; \
	fi

.PHONY: keycloak-logs
keycloak-logs: kubectl ## Tail Keycloak logs
	@$(KUBECTL) logs -n $(KEYCLOAK_NAMESPACE) -l app=keycloak -f --tail=100

.PHONY: keycloak-port-forward
keycloak-port-forward: kubectl ## Port-forward to Keycloak for browser access
	@echo "Add to /etc/hosts (one-time):  echo '127.0.0.1 keycloak.keycloak.svc' | sudo tee -a /etc/hosts"
	@echo ""
	@echo "Forwarding https://keycloak.keycloak.svc:8443 -> keycloak pod:8443"
	@echo "Press Ctrl+C to stop"
	@$(KUBECTL) port-forward -n $(KEYCLOAK_NAMESPACE) svc/keycloak 8443:8443
