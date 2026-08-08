# Keycloak IdP for development and testing

KEYCLOAK_NAMESPACE = keycloak
KEYCLOAK_ADMIN_USER = admin
KEYCLOAK_ADMIN_PASSWORD = admin

##@ Keycloak

.PHONY: keycloak-install
keycloak-install:
	@echo "Installing Keycloak (dev mode using official image)..."
	@kubectl apply -f dev/config/keycloak/deployment.yaml
	@echo "Applying Keycloak ingress (cert-manager will create TLS certificate)..."
	@kubectl apply -f dev/config/keycloak/ingress.yaml
	@echo "Waiting for cert-manager CA secret to be created..."
	@for i in $$(seq 1 30); do \
		if kubectl get secret selfsigned-ca-secret -n cert-manager >/dev/null 2>&1; then \
			break; \
		fi; \
		if [ $$i -eq 30 ]; then \
			echo "ERROR: selfsigned-ca-secret not found after 30 attempts"; \
			echo "  Check cert-manager status: kubectl get pods -n cert-manager"; \
			echo "  Check ClusterIssuer: kubectl get clusterissuer"; \
			exit 1; \
		fi; \
		echo "  Attempt $$i/30: waiting for selfsigned-ca-secret..."; \
		sleep 2; \
	done
	@echo "Extracting cert-manager CA certificate..."
	@mkdir -p _output/cert-manager-ca
	@kubectl get secret selfsigned-ca-secret -n cert-manager -o jsonpath='{.data.ca\.crt}' | base64 -d > _output/cert-manager-ca/ca.crt || \
		{ echo "ERROR: failed to extract cert-manager CA certificate"; exit 1; }
	@echo "✅ cert-manager CA certificate extracted to _output/cert-manager-ca/ca.crt (bind-mounted to API server)"
	@echo "Restarting Kubernetes API server to pick up new CA..."
	@# Delete the static pod manifest and restore it so the kubelet restarts
	@# the API server cleanly.  Do NOT pkill the process — on rootless podman
	@# with pasta networking, killing a process breaks the host port mapping
	@# permanently.
	@$(CONTAINER_ENGINE) exec $(KIND_CLUSTER_NAME)-control-plane bash -c '\
		MANIFEST=/etc/kubernetes/manifests/kube-apiserver.yaml; \
		cp $$MANIFEST /tmp/kube-apiserver.yaml.bak; \
		rm $$MANIFEST; \
		sleep 2; \
		cp /tmp/kube-apiserver.yaml.bak $$MANIFEST'
	@echo "Waiting for API server to restart..."
	@for i in $$(seq 1 60); do \
		if kubectl get --raw /healthz >/dev/null 2>&1; then \
			echo "✅ Kubernetes API server updated with cert-manager CA"; \
			break; \
		fi; \
		if [ $$i -eq 60 ]; then \
			echo "ERROR: API server did not restart within 120s"; \
			exit 1; \
		fi; \
		sleep 2; \
	done
	@echo "Flushing conntrack and re-patching CNI portmap rules..."
	@$(CONTAINER_ENGINE) exec $(KIND_CLUSTER_NAME)-control-plane bash -c '\
		if nft list chain ip cni_hostport prerouting 2>/dev/null | grep -q "fib daddr type local jump hostports"; then \
			echo "nft rules OK"; \
		else \
			HANDLE=$$(nft -a list chain ip cni_hostport prerouting 2>/dev/null | grep "jump hostports" | grep -v fib | sed "s/.*handle //"); \
			if [ -n "$$HANDLE" ]; then \
				nft delete rule ip cni_hostport prerouting handle $$HANDLE; \
				nft add rule ip cni_hostport prerouting fib daddr type local jump hostports; \
				echo "re-patched prerouting chain"; \
			fi; \
		fi; \
		conntrack -F 2>/dev/null || true'
	@echo "✅ Network state refreshed"
	@echo "Waiting for Keycloak to be ready..."
	@kubectl wait --for=condition=ready pod -l app=keycloak -n $(KEYCLOAK_NAMESPACE) --timeout=120s || true
	@echo "Waiting for Keycloak HTTP endpoint to be available..."
	@for i in $$(seq 1 30); do \
		STATUS=$$(curl -sk -o /dev/null -w "%{http_code}" https://keycloak.127-0-0-1.sslip.io:8443/realms/master 2>/dev/null || echo "000"); \
		if [ "$$STATUS" = "200" ]; then \
			echo "✅ Keycloak HTTP endpoint ready"; \
			break; \
		fi; \
		echo "  Attempt $$i/30: Waiting for Keycloak (status: $$STATUS)..."; \
		sleep 3; \
	done
	@echo ""
	@echo "Setting up OpenShift realm..."
	@$(MAKE) -s keycloak-setup-realm
	@echo ""
	@echo "✅ Keycloak installed and configured!"
	@echo "Access at: https://keycloak.127-0-0-1.sslip.io:8443"

.PHONY: keycloak-uninstall
keycloak-uninstall:
	@kubectl delete -f dev/config/keycloak/deployment.yaml 2>/dev/null || true

.PHONY: keycloak-status
keycloak-status: ## Show Keycloak status and connection info
	@if kubectl get svc -n $(KEYCLOAK_NAMESPACE) keycloak >/dev/null 2>&1; then \
		echo "========================================"; \
		echo "Keycloak Status"; \
		echo "========================================"; \
		echo ""; \
		echo "Status: Installed"; \
		echo ""; \
		echo "Admin Console:"; \
		echo "  URL: https://keycloak.127-0-0-1.sslip.io:8443"; \
		echo "  Username: $(KEYCLOAK_ADMIN_USER)"; \
		echo "  Password: $(KEYCLOAK_ADMIN_PASSWORD)"; \
		echo ""; \
		echo "OIDC Endpoints (openshift realm):"; \
		echo "  Discovery: https://keycloak.127-0-0-1.sslip.io:8443/realms/openshift/.well-known/openid-configuration"; \
		echo "  Token:     https://keycloak.127-0-0-1.sslip.io:8443/realms/openshift/protocol/openid-connect/token"; \
		echo "  Authorize: https://keycloak.127-0-0-1.sslip.io:8443/realms/openshift/protocol/openid-connect/auth"; \
		echo "  UserInfo:  https://keycloak.127-0-0-1.sslip.io:8443/realms/openshift/protocol/openid-connect/userinfo"; \
		echo "  JWKS:      https://keycloak.127-0-0-1.sslip.io:8443/realms/openshift/protocol/openid-connect/certs"; \
		echo ""; \
		echo "========================================"; \
	else \
		echo "Keycloak is not installed. Run: make keycloak-install"; \
	fi

.PHONY: keycloak-logs
keycloak-logs: ## Tail Keycloak logs
	@kubectl logs -n $(KEYCLOAK_NAMESPACE) -l app=keycloak -f --tail=100

.PHONY: keycloak-setup-realm
keycloak-setup-realm:
	@echo "========================================="
	@echo "Setting up OpenShift Realm for Token Exchange"
	@echo "========================================="
	@echo "Using Keycloak at https://keycloak.127-0-0-1.sslip.io:8443"
	@echo ""
	@echo "Getting admin access token..."
	@RESPONSE=$$(curl -sk -X POST "https://keycloak.127-0-0-1.sslip.io:8443/realms/master/protocol/openid-connect/token" \
		-H "Content-Type: application/x-www-form-urlencoded" \
		-d "username=$(KEYCLOAK_ADMIN_USER)" \
		-d "password=$(KEYCLOAK_ADMIN_PASSWORD)" \
		-d "grant_type=password" \
		-d "client_id=admin-cli"); \
	TOKEN=$$(echo "$$RESPONSE" | jq -r '.access_token // empty' 2>/dev/null); \
	if [ -z "$$TOKEN" ] || [ "$$TOKEN" = "null" ]; then \
		echo "❌ Failed to get access token"; \
		echo "Response was: $$RESPONSE" | head -c 200; \
		echo ""; \
		echo "Check if:"; \
		echo "  - Keycloak is running (make keycloak-install)"; \
		echo "  - Keycloak is accessible at https://keycloak.127-0-0-1.sslip.io:8443"; \
		echo "  - Admin credentials are correct: $(KEYCLOAK_ADMIN_USER)/$(KEYCLOAK_ADMIN_PASSWORD)"; \
		exit 1; \
	fi; \
	echo "✅ Successfully obtained access token"; \
	echo ""; \
	echo "Creating OpenShift realm..."; \
	REALM_RESPONSE=$$(curl -sk -w "%{http_code}" -X POST "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d @dev/config/keycloak/realm/realm-create.json); \
	REALM_CODE=$$(echo "$$REALM_RESPONSE" | tail -c 4); \
	if [ "$$REALM_CODE" = "201" ] || [ "$$REALM_CODE" = "409" ]; then \
		if [ "$$REALM_CODE" = "201" ]; then echo "✅ OpenShift realm created"; \
		else echo "✅ OpenShift realm already exists"; fi; \
	else \
		echo "❌ Failed to create OpenShift realm (HTTP $$REALM_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Configuring realm events..."; \
	EVENT_CONFIG_RESPONSE=$$(curl -sk -w "HTTPCODE:%{http_code}" -X PUT "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d @dev/config/keycloak/realm/realm-events-config.json); \
	EVENT_CONFIG_CODE=$$(echo "$$EVENT_CONFIG_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	if [ "$$EVENT_CONFIG_CODE" = "204" ]; then \
		echo "✅ User and admin event logging enabled"; \
	else \
		echo "⚠️  Could not configure event logging (HTTP $$EVENT_CONFIG_CODE)"; \
	fi; \
	echo ""; \
	echo "Creating mcp:openshift client scope..."; \
	SCOPE_RESPONSE=$$(curl -sk -w "HTTPCODE:%{http_code}" -X POST "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/client-scopes" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d @dev/config/keycloak/client-scopes/mcp-openshift.json); \
	SCOPE_CODE=$$(echo "$$SCOPE_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	if [ "$$SCOPE_CODE" = "201" ] || [ "$$SCOPE_CODE" = "409" ]; then \
		if [ "$$SCOPE_CODE" = "201" ]; then echo "✅ mcp:openshift client scope created"; \
		else echo "✅ mcp:openshift client scope already exists"; fi; \
	else \
		echo "❌ Failed to create mcp:openshift scope (HTTP $$SCOPE_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Adding audience mapper to mcp:openshift scope..."; \
	SCOPES_LIST=$$(curl -sk -X GET "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/client-scopes" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Accept: application/json"); \
	SCOPE_ID=$$(echo "$$SCOPES_LIST" | jq -r '.[] | select(.name == "mcp:openshift") | .id // empty' 2>/dev/null); \
	if [ -z "$$SCOPE_ID" ]; then \
		echo "❌ Failed to find mcp:openshift scope"; \
		exit 1; \
	fi; \
	MAPPER_RESPONSE=$$(curl -sk -w "HTTPCODE:%{http_code}" -X POST "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/client-scopes/$$SCOPE_ID/protocol-mappers/models" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d @dev/config/keycloak/mappers/openshift-audience.json); \
	MAPPER_CODE=$$(echo "$$MAPPER_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	if [ "$$MAPPER_CODE" = "201" ] || [ "$$MAPPER_CODE" = "409" ]; then \
		if [ "$$MAPPER_CODE" = "201" ]; then echo "✅ Audience mapper added"; \
		else echo "✅ Audience mapper already exists"; fi; \
	else \
		echo "❌ Failed to create audience mapper (HTTP $$MAPPER_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Creating groups client scope..."; \
	GROUPS_SCOPE_RESPONSE=$$(curl -sk -w "HTTPCODE:%{http_code}" -X POST "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/client-scopes" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d @dev/config/keycloak/client-scopes/groups.json); \
	GROUPS_SCOPE_CODE=$$(echo "$$GROUPS_SCOPE_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	if [ "$$GROUPS_SCOPE_CODE" = "201" ] || [ "$$GROUPS_SCOPE_CODE" = "409" ]; then \
		if [ "$$GROUPS_SCOPE_CODE" = "201" ]; then echo "✅ groups client scope created"; \
		else echo "✅ groups client scope already exists"; fi; \
	else \
		echo "❌ Failed to create groups scope (HTTP $$GROUPS_SCOPE_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Adding group membership mapper to groups scope..."; \
	SCOPES_LIST=$$(curl -sk -X GET "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/client-scopes" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Accept: application/json"); \
	GROUPS_SCOPE_ID=$$(echo "$$SCOPES_LIST" | jq -r '.[] | select(.name == "groups") | .id // empty' 2>/dev/null); \
	if [ -z "$$GROUPS_SCOPE_ID" ]; then \
		echo "❌ Failed to find groups scope"; \
		exit 1; \
	fi; \
	GROUPS_MAPPER_RESPONSE=$$(curl -sk -w "HTTPCODE:%{http_code}" -X POST "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/client-scopes/$$GROUPS_SCOPE_ID/protocol-mappers/models" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d @dev/config/keycloak/mappers/groups-membership.json); \
	GROUPS_MAPPER_CODE=$$(echo "$$GROUPS_MAPPER_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	if [ "$$GROUPS_MAPPER_CODE" = "201" ] || [ "$$GROUPS_MAPPER_CODE" = "409" ]; then \
		if [ "$$GROUPS_MAPPER_CODE" = "201" ]; then echo "✅ Group membership mapper added"; \
		else echo "✅ Group membership mapper already exists"; fi; \
	else \
		echo "❌ Failed to create group mapper (HTTP $$GROUPS_MAPPER_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Creating mcp-server client scope..."; \
	MCP_SERVER_SCOPE_RESPONSE=$$(curl -sk -w "HTTPCODE:%{http_code}" -X POST "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/client-scopes" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d @dev/config/keycloak/client-scopes/mcp-server.json); \
	MCP_SERVER_SCOPE_CODE=$$(echo "$$MCP_SERVER_SCOPE_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	if [ "$$MCP_SERVER_SCOPE_CODE" = "201" ] || [ "$$MCP_SERVER_SCOPE_CODE" = "409" ]; then \
		if [ "$$MCP_SERVER_SCOPE_CODE" = "201" ]; then echo "✅ mcp-server client scope created"; \
		else echo "✅ mcp-server client scope already exists"; fi; \
	else \
		echo "❌ Failed to create mcp-server scope (HTTP $$MCP_SERVER_SCOPE_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Adding audience mapper to mcp-server scope..."; \
	SCOPES_LIST=$$(curl -sk -X GET "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/client-scopes" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Accept: application/json"); \
	MCP_SERVER_SCOPE_ID=$$(echo "$$SCOPES_LIST" | jq -r '.[] | select(.name == "mcp-server") | .id // empty' 2>/dev/null); \
	if [ -z "$$MCP_SERVER_SCOPE_ID" ]; then \
		echo "❌ Failed to find mcp-server scope"; \
		exit 1; \
	fi; \
	MCP_SERVER_MAPPER_RESPONSE=$$(curl -sk -w "HTTPCODE:%{http_code}" -X POST "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/client-scopes/$$MCP_SERVER_SCOPE_ID/protocol-mappers/models" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d @dev/config/keycloak/mappers/mcp-server-audience.json); \
	MCP_SERVER_MAPPER_CODE=$$(echo "$$MCP_SERVER_MAPPER_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	if [ "$$MCP_SERVER_MAPPER_CODE" = "201" ] || [ "$$MCP_SERVER_MAPPER_CODE" = "409" ]; then \
		if [ "$$MCP_SERVER_MAPPER_CODE" = "201" ]; then echo "✅ mcp-server audience mapper added"; \
		else echo "✅ mcp-server audience mapper already exists"; fi; \
	else \
		echo "❌ Failed to create mcp-server audience mapper (HTTP $$MCP_SERVER_MAPPER_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Creating openshift service client..."; \
	OPENSHIFT_CLIENT_RESPONSE=$$(curl -sk -w "HTTPCODE:%{http_code}" -X POST "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/clients" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d @dev/config/keycloak/clients/openshift.json); \
	OPENSHIFT_CLIENT_CODE=$$(echo "$$OPENSHIFT_CLIENT_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	if [ "$$OPENSHIFT_CLIENT_CODE" = "201" ] || [ "$$OPENSHIFT_CLIENT_CODE" = "409" ]; then \
		if [ "$$OPENSHIFT_CLIENT_CODE" = "201" ]; then echo "✅ openshift client created"; \
		else echo "✅ openshift client already exists"; fi; \
	else \
		echo "❌ Failed to create openshift client (HTTP $$OPENSHIFT_CLIENT_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Adding username mapper to openshift client..."; \
	OPENSHIFT_CLIENTS_LIST=$$(curl -sk -X GET "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/clients" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Accept: application/json"); \
	OPENSHIFT_CLIENT_ID=$$(echo "$$OPENSHIFT_CLIENTS_LIST" | jq -r '.[] | select(.clientId == "openshift") | .id // empty' 2>/dev/null); \
	OPENSHIFT_USERNAME_MAPPER_RESPONSE=$$(curl -sk -w "HTTPCODE:%{http_code}" -X POST "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/clients/$$OPENSHIFT_CLIENT_ID/protocol-mappers/models" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d @dev/config/keycloak/mappers/username.json); \
	OPENSHIFT_USERNAME_MAPPER_CODE=$$(echo "$$OPENSHIFT_USERNAME_MAPPER_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	if [ "$$OPENSHIFT_USERNAME_MAPPER_CODE" = "201" ] || [ "$$OPENSHIFT_USERNAME_MAPPER_CODE" = "409" ]; then \
		if [ "$$OPENSHIFT_USERNAME_MAPPER_CODE" = "201" ]; then echo "✅ Username mapper added to openshift client"; \
		else echo "✅ Username mapper already exists on openshift client"; fi; \
	else \
		echo "❌ Failed to create username mapper (HTTP $$OPENSHIFT_USERNAME_MAPPER_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Creating mcp-client public client..."; \
	MCP_PUBLIC_CLIENT_RESPONSE=$$(curl -sk -w "HTTPCODE:%{http_code}" -X POST "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/clients" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d @dev/config/keycloak/clients/mcp-client.json); \
	MCP_PUBLIC_CLIENT_CODE=$$(echo "$$MCP_PUBLIC_CLIENT_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	if [ "$$MCP_PUBLIC_CLIENT_CODE" = "201" ] || [ "$$MCP_PUBLIC_CLIENT_CODE" = "409" ]; then \
		if [ "$$MCP_PUBLIC_CLIENT_CODE" = "201" ]; then echo "✅ mcp-client public client created"; \
		else echo "✅ mcp-client public client already exists"; fi; \
	else \
		echo "❌ Failed to create mcp-client public client (HTTP $$MCP_PUBLIC_CLIENT_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Adding username mapper to mcp-client..."; \
	MCP_PUBLIC_CLIENTS_LIST=$$(curl -sk -X GET "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/clients" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Accept: application/json"); \
	MCP_PUBLIC_CLIENT_ID=$$(echo "$$MCP_PUBLIC_CLIENTS_LIST" | jq -r '.[] | select(.clientId == "mcp-client") | .id // empty' 2>/dev/null); \
	MCP_PUBLIC_USERNAME_MAPPER_RESPONSE=$$(curl -sk -w "HTTPCODE:%{http_code}" -X POST "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/clients/$$MCP_PUBLIC_CLIENT_ID/protocol-mappers/models" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d @dev/config/keycloak/mappers/username.json); \
	MCP_PUBLIC_USERNAME_MAPPER_CODE=$$(echo "$$MCP_PUBLIC_USERNAME_MAPPER_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	if [ "$$MCP_PUBLIC_USERNAME_MAPPER_CODE" = "201" ] || [ "$$MCP_PUBLIC_USERNAME_MAPPER_CODE" = "409" ]; then \
		if [ "$$MCP_PUBLIC_USERNAME_MAPPER_CODE" = "201" ]; then echo "✅ Username mapper added to mcp-client"; \
		else echo "✅ Username mapper already exists on mcp-client"; fi; \
	else \
		echo "❌ Failed to create username mapper (HTTP $$MCP_PUBLIC_USERNAME_MAPPER_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Creating mcp-server client with token exchange..."; \
	MCP_CLIENT_RESPONSE=$$(curl -sk -w "HTTPCODE:%{http_code}" -X POST "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/clients" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d @dev/config/keycloak/clients/mcp-server.json); \
	MCP_CLIENT_CODE=$$(echo "$$MCP_CLIENT_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	if [ "$$MCP_CLIENT_CODE" = "201" ] || [ "$$MCP_CLIENT_CODE" = "409" ]; then \
		if [ "$$MCP_CLIENT_CODE" = "201" ]; then echo "✅ mcp-server client created"; \
		else echo "✅ mcp-server client already exists"; fi; \
	else \
		echo "❌ Failed to create mcp-server client (HTTP $$MCP_CLIENT_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Enabling standard token exchange for mcp-server..."; \
	CLIENTS_LIST=$$(curl -sk -X GET "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/clients" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Accept: application/json"); \
	MCP_CLIENT_ID=$$(echo "$$CLIENTS_LIST" | jq -r '.[] | select(.clientId == "mcp-server") | .id // empty' 2>/dev/null); \
	if [ -z "$$MCP_CLIENT_ID" ]; then \
		echo "❌ Failed to find mcp-server client"; \
		exit 1; \
	fi; \
	UPDATE_CLIENT_RESPONSE=$$(curl -sk -w "HTTPCODE:%{http_code}" -X PUT "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/clients/$$MCP_CLIENT_ID" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d @dev/config/keycloak/clients/mcp-server-update.json); \
	UPDATE_CLIENT_CODE=$$(echo "$$UPDATE_CLIENT_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	if [ "$$UPDATE_CLIENT_CODE" = "204" ]; then \
		echo "✅ Standard token exchange enabled for mcp-server client"; \
	else \
		echo "⚠️  Could not enable token exchange (HTTP $$UPDATE_CLIENT_CODE)"; \
	fi; \
	echo ""; \
	echo "Getting mcp-server client secret..."; \
	SECRET_RESPONSE=$$(curl -sk -X GET "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/clients/$$MCP_CLIENT_ID/client-secret" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Accept: application/json"); \
	CLIENT_SECRET=$$(echo "$$SECRET_RESPONSE" | jq -r '.value // empty' 2>/dev/null); \
	if [ -z "$$CLIENT_SECRET" ]; then \
		echo "❌ Failed to get client secret"; \
	else \
		echo "✅ Client secret retrieved"; \
	fi; \
	echo ""; \
	echo "Adding username mapper to mcp-server client..."; \
	MCP_USERNAME_MAPPER_RESPONSE=$$(curl -sk -w "HTTPCODE:%{http_code}" -X POST "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/clients/$$MCP_CLIENT_ID/protocol-mappers/models" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d @dev/config/keycloak/mappers/username.json); \
	MCP_USERNAME_MAPPER_CODE=$$(echo "$$MCP_USERNAME_MAPPER_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	if [ "$$MCP_USERNAME_MAPPER_CODE" = "201" ] || [ "$$MCP_USERNAME_MAPPER_CODE" = "409" ]; then \
		if [ "$$MCP_USERNAME_MAPPER_CODE" = "201" ]; then echo "✅ Username mapper added to mcp-server client"; \
		else echo "✅ Username mapper already exists on mcp-server client"; fi; \
	else \
		echo "❌ Failed to create username mapper (HTTP $$MCP_USERNAME_MAPPER_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Creating test user mcp/mcp..."; \
	USER_RESPONSE=$$(curl -sk -w "%{http_code}" -X POST "https://keycloak.127-0-0-1.sslip.io:8443/admin/realms/openshift/users" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d @dev/config/keycloak/users/mcp.json); \
	USER_CODE=$$(echo "$$USER_RESPONSE" | tail -c 4); \
	if [ "$$USER_CODE" = "201" ] || [ "$$USER_CODE" = "409" ]; then \
		if [ "$$USER_CODE" = "201" ]; then echo "✅ mcp user created"; \
		else echo "✅ mcp user already exists"; fi; \
	else \
		echo "❌ Failed to create mcp user (HTTP $$USER_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Setting up RBAC for mcp user..."; \
	kubectl apply -f dev/config/keycloak/rbac.yaml; \
	echo "✅ RBAC binding created for mcp user"; \
	echo ""; \
	echo "🎉 OpenShift realm setup complete!"; \
	echo ""; \
	echo "========================================"; \
	echo "Configuration Summary"; \
	echo "========================================"; \
	echo "Realm: openshift"; \
	echo "Authorization URL: https://keycloak.127-0-0-1.sslip.io:8443/realms/openshift"; \
	echo "Issuer URL (for config.toml): https://keycloak.127-0-0-1.sslip.io:8443/realms/openshift"; \
	echo ""; \
	echo "Test User:"; \
	echo "  Username: mcp"; \
	echo "  Password: mcp"; \
	echo "  Email: mcp@example.com"; \
	echo "  RBAC: cluster-admin (full cluster access)"; \
	echo ""; \
	echo "Clients:"; \
	echo "  mcp-client (public, for browser-based auth)"; \
	echo "    Client ID: mcp-client"; \
	echo "    Optional Scopes: mcp-server"; \
	echo "  mcp-server (confidential, token exchange enabled)"; \
	echo "    Client ID: mcp-server"; \
	echo "    Client Secret: $$CLIENT_SECRET"; \
	echo "  openshift (service account)"; \
	echo "    Client ID: openshift"; \
	echo ""; \
	echo "Client Scopes:"; \
	echo "  mcp-server (default) - Audience: mcp-server"; \
	echo "  mcp:openshift (optional) - Audience: openshift"; \
	echo "  groups (default) - Group membership mapper"; \
	echo ""; \
	echo "TOML Configuration (config.toml):"; \
	echo "  require_oauth = true"; \
	echo "  oauth_audience = \"mcp-server\""; \
	echo "  oauth_scopes = [\"openid\", \"mcp-server\"]"; \
	echo "  validate_token = false"; \
	echo "  authorization_url = \"https://keycloak.127-0-0-1.sslip.io:8443/realms/openshift\""; \
	echo "  sts_client_id = \"mcp-server\""; \
	echo "  sts_client_secret = \"$$CLIENT_SECRET\""; \
	echo "  sts_audience = \"openshift\""; \
	echo "  sts_scopes = [\"mcp:openshift\"]"; \
	echo "  certificate_authority = \"_output/cert-manager-ca/ca.crt\""; \
	echo "========================================"; \
	echo ""; \
	echo "Note: The Kubernetes API server is configured with:"; \
	echo "  --oidc-issuer-url=https://keycloak.127-0-0-1.sslip.io:8443/realms/openshift"; \
	echo ""; \
	echo "Important: The cert-manager CA certificate was extracted to:"; \
	echo "  _output/cert-manager-ca/ca.crt"; \
	echo ""; \
	echo "Writing configuration to _output/config.toml..."; \
	mkdir -p _output; \
	printf '%s\n' \
		'require_oauth = true' \
		'oauth_audience = "mcp-server"' \
		'oauth_scopes = ["openid", "mcp-server"]' \
		'validate_token = false' \
		'authorization_url = "https://keycloak.127-0-0-1.sslip.io:8443/realms/openshift"' \
		'sts_client_id = "mcp-server"' \
		"sts_client_secret = \"$$CLIENT_SECRET\"" \
		'sts_audience = "openshift"' \
		'sts_scopes = ["mcp:openshift"]' \
		'certificate_authority = "_output/cert-manager-ca/ca.crt"' \
		> _output/config.toml; \
	echo "✅ Configuration written to _output/config.toml"
