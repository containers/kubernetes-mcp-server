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
	@$(MINIKUBE) kubectl --profile $(MINIKUBE_PROFILE) -- config view --flatten --minify > _output/kubeconfig
	@$(KUBECTL) apply -f dev/config/keycloak/rbac.yaml
	@mkdir -p _output
	@cp dev/config/keycloak/config.toml _output/config.toml
	@echo ""
	@echo "Keycloak installed and configured!"
	@echo "  Admin console: make keycloak-port-forward, then https://localhost:8443"
	@echo "  Test user: mcp / mcp"
	@echo "  Config: _output/config.toml"

# keycloak-setup-spoke configures the spoke minikube cluster so its API server
# validates OIDC tokens issued by the Keycloak running on the hub cluster.
#
# The spoke's kube-apiserver needs to reach Keycloak for OIDC discovery, but
# Keycloak runs inside the hub cluster. Cross-cluster networking is handled at
# creation time (--network in minikube-create-spoke-cluster places the spoke on
# the hub's container network). This target handles the remaining configuration:
#
#   1. Copy the Keycloak CA cert into the spoke node (persists on a minikube
#      volume across container restarts).
#   2. Expose Keycloak on the hub via NodePort 30443.
#   3. Reconfigure the spoke API server with OIDC flags via minikube start.
#      This restarts the spoke container, wiping ephemeral state — all steps
#      below MUST come after this point.
#   4. Set up /etc/hosts (keycloak.keycloak.svc → 127.0.0.2) and nsswitch.conf
#      (files before dns) so the spoke's kube-apiserver resolves the Keycloak
#      hostname via /etc/hosts rather than cluster DNS.
#   5. Run a socat proxy on 127.0.0.2:8443 → hub:30443. Bound to 127.0.0.2
#      (not 0.0.0.0) to avoid conflicting with the kube-apiserver on *:6444.
#   6. Restart the kube-apiserver so it retries OIDC discovery now that the
#      Keycloak path is live.
.PHONY: keycloak-setup-spoke
keycloak-setup-spoke: minikube kubectl ## Configure spoke cluster to use Keycloak for OIDC (hub must be running)
	$(eval HUB_IP := $(shell $(KUBECTL) --kubeconfig $(shell pwd)/_output/kubeconfig get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}'))
	@if [ -z "$(HUB_IP)" ]; then echo "ERROR: could not determine hub node IP — is the hub cluster running?"; exit 1; fi
	@echo "Configuring spoke cluster for Keycloak OIDC..."
	@echo "Hub node IP: $(HUB_IP)"
# --- Step 1: CA cert (persists on minikube volume) ---
	@echo "Copying CA certificate into spoke Minikube node..."
	@$(MINIKUBE) cp _output/cert-manager-ca/ca.crt $(MINIKUBE_SPOKE_PROFILE):/var/lib/minikube/certs/keycloak-ca.crt --profile $(MINIKUBE_SPOKE_PROFILE)
# --- Step 2: NodePort for cross-cluster Keycloak access ---
	@echo "Exposing Keycloak on hub via NodePort (port 30443)..."
	@$(KUBECTL) --kubeconfig $(shell pwd)/_output/kubeconfig apply -f dev/config/keycloak/keycloak-nodeport.yaml
# --- Step 3: OIDC config (restarts spoke container — ephemeral state lost) ---
	@echo "Restarting spoke API server with OIDC (client-id=spoke)..."
	@$(MINIKUBE) start --profile $(MINIKUBE_SPOKE_PROFILE) \
		--extra-config=apiserver.oidc-issuer-url=https://keycloak.keycloak.svc:8443/realms/openshift \
		--extra-config=apiserver.oidc-client-id=spoke \
		--extra-config=apiserver.oidc-username-claim=preferred_username \
		--extra-config=apiserver.oidc-groups-claim=groups \
		--extra-config=apiserver.oidc-ca-file=/var/lib/minikube/certs/keycloak-ca.crt
# --- Step 4: DNS resolution for keycloak.keycloak.svc inside the spoke ---
	@echo "Adding Keycloak DNS entry to spoke node /etc/hosts (127.0.0.2)..."
	@$(MINIKUBE) ssh --profile $(MINIKUBE_SPOKE_PROFILE) -- \
		"grep -v keycloak.keycloak.svc /etc/hosts | sudo tee /etc/hosts.tmp > /dev/null && sudo cp /etc/hosts.tmp /etc/hosts && sudo rm /etc/hosts.tmp && echo '127.0.0.2 keycloak.keycloak.svc' | sudo tee -a /etc/hosts > /dev/null"
	@$(MINIKUBE) ssh --profile $(MINIKUBE_SPOKE_PROFILE) -- \
		"grep -q '^hosts:.*files' /etc/nsswitch.conf 2>/dev/null || { sudo sed -i '/^hosts:/d' /etc/nsswitch.conf 2>/dev/null; echo 'hosts: files dns' | sudo tee -a /etc/nsswitch.conf > /dev/null; }"
# --- Step 5: socat proxy (127.0.0.2:8443 → hub NodePort) ---
	@echo "Starting socat proxy on spoke node (127.0.0.2:8443 -> hub:30443)..."
	-@$(MINIKUBE) ssh --profile $(MINIKUBE_SPOKE_PROFILE) -- "sudo pkill -f 'socat.*TCP-LISTEN:8443' 2>/dev/null"
	-@$(MINIKUBE) ssh --profile $(MINIKUBE_SPOKE_PROFILE) -- \
		"sudo sh -c 'socat TCP-LISTEN:8443,fork,reuseaddr,bind=127.0.0.2 TCP:$(HUB_IP):30443 </dev/null >/dev/null 2>&1 &'"
	@sleep 1
	@$(MINIKUBE) ssh --profile $(MINIKUBE_SPOKE_PROFILE) -- "pgrep -f 'socat.*TCP-LISTEN:8443' >/dev/null"
# --- Step 6: restart apiserver so OIDC discovery succeeds now that socat is live ---
	@echo "Restarting kube-apiserver to pick up OIDC discovery..."
	-@$(MINIKUBE) ssh --profile $(MINIKUBE_SPOKE_PROFILE) -- \
		"sudo crictl ps -q --name kube-apiserver | xargs -r sudo crictl stop" 2>/dev/null
	@$(MINIKUBE) kubectl --profile $(MINIKUBE_SPOKE_PROFILE) -- wait --for=condition=ready node --all --timeout=120s 2>/dev/null || true
# --- Export kubeconfig and apply RBAC ---
	@echo "Exporting spoke kubeconfig..."
	@$(MINIKUBE) kubectl --profile $(MINIKUBE_SPOKE_PROFILE) -- config view --flatten --minify 2>/dev/null > _output/kubeconfig-spoke
	@echo "Applying spoke RBAC..."
	@$(MINIKUBE) kubectl --profile $(MINIKUBE_SPOKE_PROFILE) -- apply -f dev/config/keycloak/rbac-spoke.yaml
	@echo "Spoke cluster OIDC configuration complete"

.PHONY: keycloak-uninstall
keycloak-uninstall: kubectl ## Uninstall Keycloak
	@$(KUBECTL) delete -f dev/config/keycloak/rbac.yaml --ignore-not-found || true
	@$(KUBECTL) delete -f dev/config/keycloak/keycloak-nodeport.yaml --ignore-not-found || true
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
