# Keycloak IdP for development and testing

KEYCLOAK_NAMESPACE = keycloak
KEYCLOAK_ADMIN_USER = admin
KEYCLOAK_ADMIN_PASSWORD = admin

.PHONY: keycloak-install
keycloak-install: ## Install Keycloak for local development
	@echo "Installing Keycloak (dev mode using official image)..."
	@kubectl apply -f config/keycloak/deployment.yaml
	@echo "Waiting for Keycloak to be ready..."
	@kubectl wait --for=condition=ready pod -l app=keycloak -n $(KEYCLOAK_NAMESPACE) --timeout=120s || true
	@echo ""
	@echo "Keycloak installed!"
	@echo "Admin credentials: $(KEYCLOAK_ADMIN_USER) / $(KEYCLOAK_ADMIN_PASSWORD)"
	@echo "Run 'make keycloak-forward' to access at http://localhost:8090"

.PHONY: keycloak-uninstall
keycloak-uninstall: ## Uninstall Keycloak
	@kubectl delete -f config/keycloak/deployment.yaml 2>/dev/null || true

.PHONY: keycloak-forward
keycloak-forward: ## Port forward Keycloak to localhost:8090
	@echo "Forwarding Keycloak to http://localhost:8090"
	@echo "Login: $(KEYCLOAK_ADMIN_USER) / $(KEYCLOAK_ADMIN_PASSWORD)"
	kubectl port-forward -n $(KEYCLOAK_NAMESPACE) svc/keycloak 8090:80

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
		echo "  URL: http://localhost:8090 (run: make keycloak-forward)"; \
		echo "  Username: $(KEYCLOAK_ADMIN_USER)"; \
		echo "  Password: $(KEYCLOAK_ADMIN_PASSWORD)"; \
		echo ""; \
		echo "OIDC Endpoints (master realm):"; \
		echo "  Discovery: http://localhost:8090/realms/master/.well-known/openid-configuration"; \
		echo "  Token:     http://localhost:8090/realms/master/protocol/openid-connect/token"; \
		echo "  Authorize: http://localhost:8090/realms/master/protocol/openid-connect/auth"; \
		echo "  UserInfo:  http://localhost:8090/realms/master/protocol/openid-connect/userinfo"; \
		echo "  JWKS:      http://localhost:8090/realms/master/protocol/openid-connect/certs"; \
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
	@echo "Using Keycloak at http://localhost:8090"
	@echo "(Ensure 'make keycloak-forward' is running in another terminal)"
	@echo ""
	@echo "Getting admin access token..."
	@TOKEN=$$(curl -s -X POST "http://localhost:8090/realms/master/protocol/openid-connect/token" \
		-H "Content-Type: application/x-www-form-urlencoded" \
		-d "username=$(KEYCLOAK_ADMIN_USER)" \
		-d "password=$(KEYCLOAK_ADMIN_PASSWORD)" \
		-d "grant_type=password" \
		-d "client_id=admin-cli" \
		2>/dev/null | jq -r '.access_token // empty'); \
	if [ -z "$$TOKEN" ] || [ "$$TOKEN" = "null" ]; then \
		echo "❌ Failed to get access token. Check if:"; \
		echo "  - Keycloak is running (make keycloak-install)"; \
		echo "  - Port forwarding is active (make keycloak-forward)"; \
		echo "  - Admin credentials are correct: $(KEYCLOAK_ADMIN_USER)/$(KEYCLOAK_ADMIN_PASSWORD)"; \
		exit 1; \
	fi; \
	echo "✅ Successfully obtained access token"; \
	echo ""; \
	echo "Creating OpenShift realm..."; \
	REALM_RESPONSE=$$(curl -s -w "%{http_code}" -X POST "http://localhost:8090/admin/realms" \
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
	EVENT_CONFIG_RESPONSE=$$(curl -s -w "HTTPCODE:%{http_code}" -X PUT "http://localhost:8090/admin/realms/openshift" \
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
	SCOPE_RESPONSE=$$(curl -s -w "HTTPCODE:%{http_code}" -X POST "http://localhost:8090/admin/realms/openshift/client-scopes" \
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
	SCOPES_LIST=$$(curl -s -X GET "http://localhost:8090/admin/realms/openshift/client-scopes" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Accept: application/json"); \
	SCOPE_ID=$$(echo "$$SCOPES_LIST" | jq -r '.[] | select(.name == "mcp:openshift") | .id // empty' 2>/dev/null); \
	if [ -z "$$SCOPE_ID" ]; then \
		echo "❌ Failed to find mcp:openshift scope"; \
		exit 1; \
	fi; \
	MAPPER_RESPONSE=$$(curl -s -w "HTTPCODE:%{http_code}" -X POST "http://localhost:8090/admin/realms/openshift/client-scopes/$$SCOPE_ID/protocol-mappers/models" \
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
	GROUPS_SCOPE_RESPONSE=$$(curl -s -w "HTTPCODE:%{http_code}" -X POST "http://localhost:8090/admin/realms/openshift/client-scopes" \
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
	SCOPES_LIST=$$(curl -s -X GET "http://localhost:8090/admin/realms/openshift/client-scopes" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Accept: application/json"); \
	GROUPS_SCOPE_ID=$$(echo "$$SCOPES_LIST" | jq -r '.[] | select(.name == "groups") | .id // empty' 2>/dev/null); \
	if [ -z "$$GROUPS_SCOPE_ID" ]; then \
		echo "❌ Failed to find groups scope"; \
		exit 1; \
	fi; \
	GROUPS_MAPPER_RESPONSE=$$(curl -s -w "HTTPCODE:%{http_code}" -X POST "http://localhost:8090/admin/realms/openshift/client-scopes/$$GROUPS_SCOPE_ID/protocol-mappers/models" \
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
	MCP_SERVER_SCOPE_RESPONSE=$$(curl -s -w "HTTPCODE:%{http_code}" -X POST "http://localhost:8090/admin/realms/openshift/client-scopes" \
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
	SCOPES_LIST=$$(curl -s -X GET "http://localhost:8090/admin/realms/openshift/client-scopes" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Accept: application/json"); \
	MCP_SERVER_SCOPE_ID=$$(echo "$$SCOPES_LIST" | jq -r '.[] | select(.name == "mcp-server") | .id // empty' 2>/dev/null); \
	if [ -z "$$MCP_SERVER_SCOPE_ID" ]; then \
		echo "❌ Failed to find mcp-server scope"; \
		exit 1; \
	fi; \
	MCP_SERVER_MAPPER_RESPONSE=$$(curl -s -w "HTTPCODE:%{http_code}" -X POST "http://localhost:8090/admin/realms/openshift/client-scopes/$$MCP_SERVER_SCOPE_ID/protocol-mappers/models" \
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
	OPENSHIFT_CLIENT_RESPONSE=$$(curl -s -w "HTTPCODE:%{http_code}" -X POST "http://localhost:8090/admin/realms/openshift/clients" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d '{"clientId":"openshift","enabled":true,"publicClient":false,"standardFlowEnabled":true,"directAccessGrantsEnabled":true,"serviceAccountsEnabled":true,"authorizationServicesEnabled":false,"redirectUris":["*"],"defaultClientScopes":["groups"],"optionalClientScopes":[]}'); \
	OPENSHIFT_CLIENT_CODE=$$(echo "$$OPENSHIFT_CLIENT_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	if [ "$$OPENSHIFT_CLIENT_CODE" = "201" ] || [ "$$OPENSHIFT_CLIENT_CODE" = "409" ]; then \
		if [ "$$OPENSHIFT_CLIENT_CODE" = "201" ]; then echo "✅ openshift client created"; \
		else echo "✅ openshift client already exists"; fi; \
	else \
		echo "❌ Failed to create openshift client (HTTP $$OPENSHIFT_CLIENT_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Creating mcp-client public client..."; \
	MCP_PUBLIC_CLIENT_RESPONSE=$$(curl -s -w "HTTPCODE:%{http_code}" -X POST "http://localhost:8090/admin/realms/openshift/clients" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d '{"clientId":"mcp-client","enabled":true,"publicClient":true,"standardFlowEnabled":true,"directAccessGrantsEnabled":true,"serviceAccountsEnabled":false,"authorizationServicesEnabled":false,"redirectUris":["*"],"defaultClientScopes":[],"optionalClientScopes":["mcp-server"]}'); \
	MCP_PUBLIC_CLIENT_CODE=$$(echo "$$MCP_PUBLIC_CLIENT_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	if [ "$$MCP_PUBLIC_CLIENT_CODE" = "201" ] || [ "$$MCP_PUBLIC_CLIENT_CODE" = "409" ]; then \
		if [ "$$MCP_PUBLIC_CLIENT_CODE" = "201" ]; then echo "✅ mcp-client public client created"; \
		else echo "✅ mcp-client public client already exists"; fi; \
	else \
		echo "❌ Failed to create mcp-client public client (HTTP $$MCP_PUBLIC_CLIENT_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "Creating mcp-server client with token exchange..."; \
	MCP_CLIENT_RESPONSE=$$(curl -s -w "HTTPCODE:%{http_code}" -X POST "http://localhost:8090/admin/realms/openshift/clients" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d '{"clientId":"mcp-server","enabled":true,"publicClient":false,"standardFlowEnabled":true,"directAccessGrantsEnabled":true,"serviceAccountsEnabled":true,"authorizationServicesEnabled":false,"redirectUris":["*"],"defaultClientScopes":["groups","mcp-server"],"optionalClientScopes":["mcp:openshift"],"attributes":{"oauth2.device.authorization.grant.enabled":"false","oidc.ciba.grant.enabled":"false","backchannel.logout.session.required":"true","backchannel.logout.revoke.offline.tokens":"false"}}'); \
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
	CLIENTS_LIST=$$(curl -s -X GET "http://localhost:8090/admin/realms/openshift/clients" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Accept: application/json"); \
	MCP_CLIENT_ID=$$(echo "$$CLIENTS_LIST" | jq -r '.[] | select(.clientId == "mcp-server") | .id // empty' 2>/dev/null); \
	if [ -z "$$MCP_CLIENT_ID" ]; then \
		echo "❌ Failed to find mcp-server client"; \
		exit 1; \
	fi; \
	UPDATE_CLIENT_RESPONSE=$$(curl -s -w "HTTPCODE:%{http_code}" -X PUT "http://localhost:8090/admin/realms/openshift/clients/$$MCP_CLIENT_ID" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d '{"clientId":"mcp-server","enabled":true,"publicClient":false,"standardFlowEnabled":true,"directAccessGrantsEnabled":true,"serviceAccountsEnabled":true,"authorizationServicesEnabled":false,"redirectUris":["*"],"defaultClientScopes":["groups","mcp-server"],"optionalClientScopes":["mcp:openshift"],"attributes":{"oauth2.device.authorization.grant.enabled":"false","oidc.ciba.grant.enabled":"false","backchannel.logout.session.required":"true","backchannel.logout.revoke.offline.tokens":"false","standard.token.exchange.enabled":"true"}}'); \
	UPDATE_CLIENT_CODE=$$(echo "$$UPDATE_CLIENT_RESPONSE" | grep -o "HTTPCODE:[0-9]*" | cut -d: -f2); \
	if [ "$$UPDATE_CLIENT_CODE" = "204" ]; then \
		echo "✅ Standard token exchange enabled for mcp-server client"; \
	else \
		echo "⚠️  Could not enable token exchange (HTTP $$UPDATE_CLIENT_CODE)"; \
	fi; \
	echo ""; \
	echo "Getting mcp-server client secret..."; \
	SECRET_RESPONSE=$$(curl -s -X GET "http://localhost:8090/admin/realms/openshift/clients/$$MCP_CLIENT_ID/client-secret" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Accept: application/json"); \
	CLIENT_SECRET=$$(echo "$$SECRET_RESPONSE" | jq -r '.value // empty' 2>/dev/null); \
	if [ -z "$$CLIENT_SECRET" ]; then \
		echo "❌ Failed to get client secret"; \
	else \
		echo "✅ Client secret retrieved"; \
	fi; \
	echo ""; \
	echo "Creating test user developer/developer..."; \
	USER_RESPONSE=$$(curl -s -w "%{http_code}" -X POST "http://localhost:8090/admin/realms/openshift/users" \
		-H "Authorization: Bearer $$TOKEN" \
		-H "Content-Type: application/json" \
		-d '{"username":"developer","email":"developer@example.com","firstName":"Developer","lastName":"User","enabled":true,"emailVerified":true,"credentials":[{"type":"password","value":"developer","temporary":false}]}'); \
	USER_CODE=$$(echo "$$USER_RESPONSE" | tail -c 4); \
	if [ "$$USER_CODE" = "201" ] || [ "$$USER_CODE" = "409" ]; then \
		if [ "$$USER_CODE" = "201" ]; then echo "✅ developer user created"; \
		else echo "✅ developer user already exists"; fi; \
	else \
		echo "❌ Failed to create developer user (HTTP $$USER_CODE)"; \
		exit 1; \
	fi; \
	echo ""; \
	echo "🎉 OpenShift realm setup complete!"; \
	echo ""; \
	echo "========================================"; \
	echo "Configuration Summary"; \
	echo "========================================"; \
	echo "Realm: openshift"; \
	echo "Authorization URL: http://localhost:8090/realms/openshift"; \
	echo ""; \
	echo "Test User:"; \
	echo "  Username: developer"; \
	echo "  Password: developer"; \
	echo "  Email: developer@example.com"; \
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
	echo "TOML Configuration:"; \
	echo "  require_oauth = true"; \
	echo "  oauth_audience = \"mcp-server\""; \
	echo "  authorization_url = \"http://localhost:8090/realms/openshift\""; \
	echo "  sts_client_id = \"mcp-server\""; \
	echo "  sts_client_secret = \"$$CLIENT_SECRET\""; \
	echo "  sts_audience = \"openshift\""; \
	echo "  sts_scopes = [\"mcp:openshift\"]"; \
	echo "========================================"
