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
