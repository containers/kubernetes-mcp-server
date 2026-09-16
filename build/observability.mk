# Observability backends for development and e2e testing

TEMPO_NAMESPACE = tempo

##@ Observability

.PHONY: tempo-install
tempo-install: kubectl ## Deploy Tempo for e2e tracing tests
	@echo "Installing Tempo..."
	@$(KUBECTL) apply -f dev/config/tempo/deployment.yaml
	@echo "Waiting for Tempo to be ready..."
	@$(KUBECTL) rollout status deployment/tempo -n $(TEMPO_NAMESPACE) --timeout=120s
	@echo "Tempo installed and ready."

.PHONY: tempo-uninstall
tempo-uninstall: kubectl ## Remove Tempo
	@$(KUBECTL) delete -f dev/config/tempo/deployment.yaml --ignore-not-found || true
