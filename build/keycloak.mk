# Keycloak IdP for development and testing

KEYCLOAK_NAMESPACE = keycloak
KEYCLOAK_ADMIN_USER = admin
KEYCLOAK_ADMIN_PASSWORD = admin

.PHONY: keycloak-install
keycloak-install: ## Install Keycloak for local development
	@echo "Installing Keycloak (dev mode using official image)..."
	@kubectl apply -f config/keycloak/deployment.yaml
	@echo "Applying Keycloak ingress (cert-manager will create TLS certificate)..."
	@kubectl apply -f config/keycloak/ingress.yaml
	@echo "Extracting cert-manager CA certificate..."
	@mkdir -p hack/cert-manager-ca
	@kubectl get secret selfsigned-ca-secret -n cert-manager -o jsonpath='{.data.ca\.crt}' | base64 -d > hack/cert-manager-ca/ca.crt
	@echo "✅ cert-manager CA certificate extracted to hack/cert-manager-ca/ca.crt (bind-mounted to API server)"
	@echo "Restarting Kubernetes API server to pick up new CA..."
	@docker exec kubernetes-mcp-server-control-plane pkill -f kube-apiserver || \
		podman exec kubernetes-mcp-server-control-plane pkill -f kube-apiserver
	@echo "Waiting for API server to restart..."
	@sleep 5
	@echo "Waiting for API server to be ready..."
	@for i in $$(seq 1 30); do \
		if kubectl get --raw /healthz >/dev/null 2>&1; then \
			echo "✅ Kubernetes API server updated with cert-manager CA"; \
			break; \
		fi; \
		sleep 2; \
	done
	@echo "Waiting for Keycloak to be ready..."
	@kubectl wait --for=condition=ready pod -l app=keycloak -n $(KEYCLOAK_NAMESPACE) --timeout=120s || true
	@echo ""
	@echo "Keycloak installed!"
	@echo "Admin credentials: $(KEYCLOAK_ADMIN_USER) / $(KEYCLOAK_ADMIN_PASSWORD)"
	@echo "Access at: https://keycloak.127-0-0-1.sslip.io:8443"

.PHONY: keycloak-uninstall
keycloak-uninstall: ## Uninstall Keycloak
	@kubectl delete -f config/keycloak/deployment.yaml 2>/dev/null || true

.PHONY: keycloak-forward
keycloak-forward: ## Port forward Keycloak (optional - ingress provides direct access)
	@echo "⚠️  Port forwarding not needed - Keycloak accessible via ingress"
	@echo "Access at: https://keycloak.127-0-0-1.sslip.io:8443"
	@echo "Login: $(KEYCLOAK_ADMIN_USER) / $(KEYCLOAK_ADMIN_PASSWORD)"
	@echo ""
	@echo "If you still want port forwarding, use:"
	@echo "  kubectl port-forward -n $(KEYCLOAK_NAMESPACE) svc/keycloak 8090:80"

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
keycloak-setup-realm: ## Setup OpenShift realm with token exchange support
	@echo "========================================="
	@echo "Setting up OpenShift Realm for Token Exchange"
	@echo "========================================="
	@echo "Using Keycloak at https://keycloak.127-0-0-1.sslip.io:8443"
	@echo ""
	@echo "Getting admin access token..."
	@TOKEN=$$(curl -sk -X POST "https://keycloak.127-0-0-1.sslip.io:8443/realms/master/protocol/openid-connect/token" \
		-H "Content-Type: application/x-www-form-urlencoded" \
		-d "username=$(KEYCLOAK_ADMIN_USER)" \
		-d "password=$(KEYCLOAK_ADMIN_PASSWORD)" \
		-d "grant_type=password" \
		-d "client_id=admin-cli" \
		2>/dev/null | jq -r '.access_token // empty'); \
	if [ -z "$$TOKEN" ] || [ "$$TOKEN" = "null" ]; then \
		echo "❌ Failed to get access token. Check if:"; \
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
		-d '{"realm":"openshift","enabled":true}'); \
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
		-d '{"realm":"openshift","enabled":true,"eventsEnabled":true,"eventsListeners":["jboss-logging"],"adminEventsEnabled":true,"adminEventsDetailsEnabled":true}'); \
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
		-d '{"name":"mcp:openshift","protocol":"openid-connect","attributes":{"display.on.consent.screen":"false","include.in.token.scope":"true"}}'); \
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
		-d '{"name":"openshift-audience","protocol":"openid-connect","protocolMapper":"oidc-audience-mapper","config":{"included.client.audience":"openshift","id.token.claim":"true","access.token.claim":"true"}}'); \
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
		-d '{"name":"groups","protocol":"openid-connect","attributes":{"display.on.consent.screen":"false","include.in.token.scope":"true"}}'); \
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
		-d '{"name":"groups","protocol":"openid-connect","protocolMapper":"oidc-group-membership-mapper","config":{"claim.name":"groups","full.path":"false","id.token.claim":"true","access.token.claim":"true","userinfo.token.claim":"true"}}'); \
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
		-d '{"name":"mcp-server","protocol":"openid-connect","attributes":{"display.on.consent.screen":"false","include.in.token.scope":"true"}}'); \
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
		-d '{"name":"mcp-server-audience","protocol":"openid-connect","protocolMapper":"oidc-audience-mapper","config":{"included.client.audience":"mcp-server","id.token.claim":"true","access.token.claim":"true"}}'); \
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
		-d '{"clientId":"openshift","enabled":true,"publicClient":false,"standardFlowEnabled":true,"directAccessGrantsEnabled":true,"serviceAccountsEnabled":true,"authorizationServicesEnabled":false,"redirectUris":["*"],"defaultClientScopes":["profile","email","groups"],"optionalClientScopes":[]}'); \
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
		-d '{	"name":"username","protocol":"openid-connect","protocolMapper":"oidc-usermodel-property-mapper","config":{"userinfo.token.claim":"true","user.attribute":"username","id.token.claim":"true","access.token.claim":"true","claim.name":"preferred_username","jsonType.label":"String"}}'); \
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
		-d '{"clientId":"mcp-client","enabled":true,"publicClient":true,"standardFlowEnabled":true,"directAccessGrantsEnabled":true,"serviceAccountsEnabled":false,"authorizationServicesEnabled":false,"redirectUris":["*"],"defaultClientScopes":["profile","email"],"optionalClientScopes":["mcp-server"]}'); \
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
		-d '{"name":"username","protocol":"openid-connect","protocolMapper":"oidc-usermodel-property-mapper","config":{"userinfo.token.claim":"true","user.attribute":"username","id.token.claim":"true","access.token.claim":"true","claim.name":"preferred_username","jsonType.label":"String"}}'); \
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
		-d '{"clientId":"mcp-server","enabled":true,"publicClient":false,"standardFlowEnabled":true,"directAccessGrantsEnabled":true,"serviceAccountsEnabled":true,"authorizationServicesEnabled":false,"redirectUris":["*"],"defaultClientScopes":["profile","email","groups","mcp-server"],"optionalClientScopes":["mcp:openshift"],"attributes":{"oauth2.device.authorization.grant.enabled":"false","oidc.ciba.grant.enabled":"false","backchannel.logout.session.required":"true","backchannel.logout.revoke.offline.tokens":"false"}}'); \
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
		-d '{"clientId":"mcp-server","enabled":true,"publicClient":false,"standardFlowEnabled":true,"directAccessGrantsEnabled":true,"serviceAccountsEnabled":true,"authorizationServicesEnabled":false,"redirectUris":["*"],"defaultClientScopes":["profile","email","groups","mcp-server"],"optionalClientScopes":["mcp:openshift"],"attributes":{"oauth2.device.authorization.grant.enabled":"false","oidc.ciba.grant.enabled":"false","backchannel.logout.session.required":"true","backchannel.logout.revoke.offline.tokens":"false","standard.token.exchange.enabled":"true"}}'); \
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
		-d '{"name":"username","protocol":"openid-connect","protocolMapper":"oidc-usermodel-property-mapper","config":{"userinfo.token.claim":"true","user.attribute":"username","id.token.claim":"true","access.token.claim":"true","claim.name":"preferred_username","jsonType.label":"String"}}'); \
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
		-d '{"username":"mcp","email":"mcp@example.com","firstName":"MCP","lastName":"User","enabled":true,"emailVerified":true,"credentials":[{"type":"password","value":"mcp","temporary":false}]}'); \
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
	kubectl apply -f config/keycloak/rbac.yaml; \
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
	echo "  certificate_authority = \"hack/cert-manager-ca/ca.crt\""; \
	echo "========================================"; \
	echo ""; \
	echo "Note: The Kubernetes API server is configured with:"; \
	echo "  --oidc-issuer-url=https://keycloak.127-0-0-1.sslip.io:8443/realms/openshift"; \
	echo ""; \
	echo "Important: The cert-manager CA certificate was extracted to:"; \
	echo "  hack/cert-manager-ca/ca.crt"
