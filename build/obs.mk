# Observability toolset e2e infrastructure (Prometheus, Tempo Operator, Loki Operator)
# Installs the operator-managed backends required by the obs toolsets.

OBS_SETUP_SCRIPT = hack/e2e/obs/setup.sh

##@ Obs Toolset E2E

.PHONY: obs-setup
obs-setup: kubectl ## Install obs stacks (Prometheus, Tempo, Loki) into existing cluster
	KUBECONFIG=$(shell pwd)/_output/kubeconfig $(OBS_SETUP_SCRIPT) up

.PHONY: obs-setup-prometheus
obs-setup-prometheus: kubectl ## Install only Prometheus stack
	KUBECONFIG=$(shell pwd)/_output/kubeconfig $(OBS_SETUP_SCRIPT) up --stacks prometheus

.PHONY: obs-setup-tempo
obs-setup-tempo: kubectl ## Install only Tempo stack (Tempo Operator + TempoStack CRs)
	KUBECONFIG=$(shell pwd)/_output/kubeconfig $(OBS_SETUP_SCRIPT) up --stacks tempo

.PHONY: obs-setup-loki
obs-setup-loki: kubectl ## Install only Loki stack (Loki Operator + LokiStack CR)
	KUBECONFIG=$(shell pwd)/_output/kubeconfig $(OBS_SETUP_SCRIPT) up --stacks loki

.PHONY: obs-teardown
obs-teardown: kubectl ## Remove obs stacks from cluster
	KUBECONFIG=$(shell pwd)/_output/kubeconfig $(OBS_SETUP_SCRIPT) down

.PHONY: e2e-setup-obs
e2e-setup-obs: e2e-setup obs-setup ## Full e2e setup with obs stacks (cluster + image + Prometheus + Tempo + Loki)
