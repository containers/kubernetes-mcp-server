#!/usr/bin/env bash
# Setup observability stacks (Prometheus, Tempo, Loki) for obs toolset E2E tests.
#
# This script installs the operator-managed observability backends that the
# obs toolsets (observability/metrics, observability/traces, observability/logs,
# observability/otelcol) require for end-to-end testing.
#
# Cluster provisioning and MCP server deployment are handled separately by the
# kubernetes-mcp-server Makefile (make e2e-setup / deployServer in test helpers).

set -euo pipefail

# ---------------------------------------------------------------------------
# Utils
# ---------------------------------------------------------------------------

SCRIPT_DIR="$(dirname "$(realpath "${BASH_SOURCE[0]}")")"
RED='\033[0;31m' GREEN='\033[0;32m' CYAN='\033[0;36m' YELLOW='\033[0;33m' BOLD='\033[1m' NC='\033[0m'
phase() { echo -e "\n${BOLD}${YELLOW}==== $1 ====${NC}"; }
step()  { echo -e "\n${CYAN}==> $1${NC}"; }
ok()    { echo -e "    ${GREEN}✓${NC} $1"; }
run_info() { echo -e "    ${CYAN}»${NC} $*"; }
info()  { echo -e "    ${CYAN}i${NC} $1"; }
warn()  { echo -e "    ${YELLOW}!${NC} $1"; }
fail()  { echo -e "    ${RED}✗${NC} $1"; exit 1; }

# Wait for a resource to be created by an operator, then wait for its rollout to complete.
_wait_rollout() {
    local namespace="$1"
    local resource="$2"
    local timeout="${3:-5m}"
    _run $KUBECTL -n "${namespace}" wait --for=create "${resource}" --timeout="${timeout}"
    _run $KUBECTL -n "${namespace}" rollout status "${resource}" --timeout="${timeout}"
}

# Execute the command while capturing the output. Print the output to stderr on fail.
_run() {
    local _out
    _out=$(mktemp)
    run_info "$@"
    if "$@" >"${_out}" 2>&1; then
        rm -f "${_out}"
    else
        local _rc=$?
        echo -e "    ${RED}✗${NC} Command failed: $*" >&2
        cat "${_out}" >&2
        rm -f "${_out}"
        return ${_rc}
    fi
}

_relativepath() {
    local _abs
    _abs="$(realpath "$1")"
    if [[ "${_abs}" == "${PWD}" ]]; then
        echo "."
    else
        echo "${_abs#"${PWD}"/}"
    fi
}

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------

PHASE_DEFAULT="up"
STACKS="prometheus,tempo,loki"
SUPPORTED_PHASES=(prereqs extras clean)
SUPPORTED_STACKS=(prometheus tempo loki)

KUBE_PROMETHEUS_VERSION="${KUBE_PROMETHEUS_VERSION:-release-0.19}"

# ROOT_DIR is the repo root (three levels up from hack/e2e/obs/).
ROOT_DIR="$(_relativepath "${SCRIPT_DIR}/../../..")"

# Manifests live under test/e2e/manifests/obs/ in the kubernetes-mcp-server repo.
MANIFESTS_DIR="${ROOT_DIR}/test/e2e/manifests/obs"

KUBE_PROMETHEUS_DIR="${ROOT_DIR}/tmp/kube-prometheus"

KUBECTL="kubectl"

usage() {
    cat <<EOF
Usage: $(basename "$0") [PHASE_EXP] [--stacks STACKS]

PHASE_EXP   Phase alias or space-separated list of concrete phases.
            Aliases  : up | down
            Phases   : ${SUPPORTED_PHASES[*]}
            Default  : ${PHASE_DEFAULT}
--stacks    Comma-separated list of observability stacks to install.
            Supported: ${SUPPORTED_STACKS[*]// /, }
            Default  : ${STACKS}

Cluster provisioning is handled by the kubernetes-mcp-server Makefile:
  make e2e-setup          # Creates Minikube cluster and builds e2e image
  hack/e2e/obs/setup.sh   # Installs obs stacks into the existing cluster
  make e2e-test           # Runs all e2e tests
EOF
}

PHASE_EXP=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --stacks)
            STACKS="$2"; shift 2 ;;
        --help|-h)
            usage; exit 0 ;;
        --*)
            fail "Unknown argument: $1" ;;
        *)
            PHASE_EXP+=("$1"); shift ;;
    esac
done

# Expand aliases / defaults
if [[ ${#PHASE_EXP[@]} -eq 0 ]]; then
    PHASE_EXP=("${PHASE_DEFAULT}")
fi

PHASES=()
for arg in "${PHASE_EXP[@]}"; do
    case "${arg}" in
        up)   PHASES+=(prereqs extras) ;;
        down) PHASES+=(clean) ;;
        *)    PHASES+=("${arg}") ;;
    esac
done

for phase in "${PHASES[@]}"; do
    if [[ " ${SUPPORTED_PHASES[*]} " != *" ${phase} "* ]]; then
        fail "Unknown phase: '${phase}'. Supported aliases: up, down. Supported phases: ${SUPPORTED_PHASES[*]// /, }"
    fi
done

has_stack() { [[ ",${STACKS}," == *",${1},"* ]]; }

info "Stacks   : ${STACKS}"
info "Phases   : ${PHASES[*]}"
info "Manifests: ${MANIFESTS_DIR}"

# ---------------------------------------------------------------------------
# Loki helper functions
# ---------------------------------------------------------------------------

# Detect the cluster's default StorageClass.
_detect_storage_class() {
    local _sc="${LOKI_STORAGE_CLASS:-}"
    if [[ -n "${_sc}" ]]; then
        echo "${_sc}"
        return
    fi
    _sc="$($KUBECTL get storageclass -o json | jq -r '.items[] | select(.metadata.annotations["storageclass.kubernetes.io/is-default-class"] == "true" or .metadata.annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true") | .metadata.name' | head -n1)"
    if [[ -z "${_sc}" ]]; then
        echo "Unable to detect a default StorageClass. Set LOKI_STORAGE_CLASS explicitly and retry." >&2
        echo "Available StorageClasses:" >&2
        $KUBECTL get storageclass >&2
        return 1
    fi
    echo "${_sc}"
}

# ---------------------------------------------------------------------------
# Phase implementations
# ---------------------------------------------------------------------------

phase_prereqs() {
    phase "prereqs — installing operators"

    if has_stack prometheus; then
        step "Installing kube-prometheus stack (${KUBE_PROMETHEUS_VERSION})"
        if [ ! -d "${KUBE_PROMETHEUS_DIR}" ]; then
            mkdir -p "${ROOT_DIR}/tmp"
            _run git clone --depth 1 --branch "${KUBE_PROMETHEUS_VERSION}" \
                https://github.com/prometheus-operator/kube-prometheus.git "${KUBE_PROMETHEUS_DIR}"
        else
            info "${KUBE_PROMETHEUS_DIR} already present, skipping clone."
        fi

        step "Applying kube-prometheus CRDs and namespace setup"
        _run $KUBECTL apply --server-side -f "${KUBE_PROMETHEUS_DIR}/manifests/setup"
        _run $KUBECTL wait --for condition=Established --all CustomResourceDefinition --namespace=monitoring --timeout=5m

        step "Installing Prometheus, Alertmanager, and Prometheus Operator"
        _run $KUBECTL apply -f "${KUBE_PROMETHEUS_DIR}/manifests/prometheusOperator-*.yaml";
        _run $KUBECTL apply -f "${KUBE_PROMETHEUS_DIR}/manifests/prometheus-*.yaml";
        _run $KUBECTL apply -f "${KUBE_PROMETHEUS_DIR}/manifests/alertmanager-*.yaml";

        step "Waiting for deployments"
        _wait_rollout monitoring deployment/prometheus-operator 5m
        _wait_rollout monitoring statefulset/prometheus-k8s 5m
        _wait_rollout monitoring statefulset/alertmanager-main 5m
    fi

    if has_stack tempo || has_stack loki; then
        if ! $KUBECTL get crd certificates.cert-manager.io &>/dev/null; then
            step "Installing Cert Manager"
            _run $KUBECTL apply -f https://github.com/jetstack/cert-manager/releases/download/v1.19.4/cert-manager.yaml
            _wait_rollout cert-manager deployment/cert-manager 5m
            _wait_rollout cert-manager deployment/cert-manager-cainjector 5m
            _wait_rollout cert-manager deployment/cert-manager-webhook 5m
        fi
    fi

    if has_stack loki; then
        step "Installing Loki operator"
        _run $KUBECTL apply --server-side -k "${MANIFESTS_DIR}/loki/prereqs/kubernetes/"
        _wait_rollout loki-operator deployment/loki-operator-controller-manager 5m

        step "Waiting for LokiStack CRD"
        _run $KUBECTL wait --for=condition=Established crd/lokistacks.loki.grafana.com --timeout=5m
    fi

    if has_stack tempo; then
        step "Installing OpenTelemetry operator"
        _run $KUBECTL apply -f https://github.com/open-telemetry/opentelemetry-operator/releases/download/v0.146.0/opentelemetry-operator.yaml
        _wait_rollout opentelemetry-operator-system deployment/opentelemetry-operator-controller-manager 5m

        step "Installing Tempo operator"
        _run $KUBECTL apply -f https://github.com/grafana/tempo-operator/releases/download/v0.20.0/tempo-operator.yaml
        _wait_rollout tempo-operator-system deployment/tempo-operator-controller 5m
    fi
}

phase_extras() {
    phase "extras — deploying workloads"

    if has_stack prometheus; then
        if [ ! -d "${KUBE_PROMETHEUS_DIR}" ]; then
            fail "${KUBE_PROMETHEUS_DIR} not found. Run prereqs phase first."
        fi

        step "Applying NetworkPolicy to allow e2e test access to Prometheus/Alertmanager"
        _run $KUBECTL apply -f "${MANIFESTS_DIR}/prometheus/deploy/kubernetes/network_policy.yaml"

        step "Installing kube-state-metrics"
        _run $KUBECTL apply -f "${KUBE_PROMETHEUS_DIR}/manifests/kubeStateMetrics-*.yaml"
        _wait_rollout monitoring deployment/kube-state-metrics 3m

        step "Installing node-exporter"
        _run $KUBECTL apply -f "${KUBE_PROMETHEUS_DIR}/manifests/nodeExporter-*.yaml"
        _wait_rollout monitoring daemonset/node-exporter 3m

        step "Installing kubelet/cAdvisor ServiceMonitors"
        _run $KUBECTL apply -f "${KUBE_PROMETHEUS_DIR}/manifests/kubernetesControlPlane-*.yaml"
    fi

    if has_stack tempo; then
        step "Deploying Tempo stacks (TempoStack CRs, MinIO, OTel Collector, tracegen)"
        _run $KUBECTL apply -k "${MANIFESTS_DIR}/tempo/extras/kubernetes"
        _wait_rollout tracing deployment/minio 5m
        _wait_rollout tracing statefulset/tempo-tempo1-ingester 5m
        _wait_rollout tracing statefulset/tempo-tempo2-ingester 5m
        _wait_rollout tracing deployment/tempo-tempo1-query-frontend 5m
        _wait_rollout tracing deployment/tempo-tempo2-query-frontend 5m

        step "Waiting for traces to appear in Tempo"
        $KUBECTL delete pods -n tracing curl-check 2>/dev/null || true
        for i in $(seq 1 20); do
            run_info "attempt ${i}"
            output=$($KUBECTL run -n tracing curl-check --image=quay.io/curl/curl --rm -q --restart=Never -i -- \
                curl -vvsf http://tempo-tempo1-query-frontend.tracing:3200/api/search 2>&1) || true
            if echo "$output" | grep -q '"traceID"'; then
                ok "Traces found in Tempo"
                break
            fi
            warn "Attempt ${i}/20: no traces yet, retrying in 30s..."
            sleep 30
            [[ $i -eq 20 ]] && fail "No traces found after 20 attempts"
        done
    fi

    if has_stack loki; then
        step "Deploying Loki stack (LokiStack CR, MinIO, log-generator)"

        _loki_sc=$(_detect_storage_class)
        info "Using StorageClass: ${_loki_sc}"

        # Build a temporary kustomize overlay to patch storageClassName into the LokiStack CR.
        _overlay="${MANIFESTS_DIR}/loki/extras/overlay"
        mkdir -p "${_overlay}"
        cat > "${_overlay}/kustomization.yaml" <<EOF
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../kubernetes
patches:
  - patch: |
      apiVersion: loki.grafana.com/v1
      kind: LokiStack
      metadata:
        name: obs-loki
        namespace: obs-loki
      spec:
        storageClassName: "${_loki_sc}"
EOF
        _run $KUBECTL apply -k "${_overlay}"

        step "Waiting for MinIO"
        _wait_rollout obs-loki deployment/minio 5m

        step "Waiting for LokiStack Ready"
        _run $KUBECTL wait --for=jsonpath='{.status.conditions[?(@.type=="Ready")].status}'=True \
            lokistack/obs-loki -n obs-loki --timeout=10m

        step "Waiting for log generator"
        _wait_rollout obs-loki deployment/obs-log-generator 2m
    fi
}

phase_clean() {
    phase "clean — removing obs stacks"

    if has_stack loki; then
        step "Removing Loki stack"
        _overlay="${MANIFESTS_DIR}/loki/extras/overlay"
        if [ -d "${_overlay}" ]; then
            $KUBECTL delete -k "${_overlay}" --ignore-not-found=true 2>/dev/null || true
            rm -rf "${_overlay}"
        fi
        $KUBECTL delete -k "${MANIFESTS_DIR}/loki/prereqs/kubernetes/" --ignore-not-found=true 2>/dev/null || true
    fi

    if has_stack tempo; then
        step "Removing Tempo stacks"
        $KUBECTL delete -k "${MANIFESTS_DIR}/tempo/extras/kubernetes" --ignore-not-found=true 2>/dev/null || true
    fi

    if has_stack prometheus; then
        step "Removing kube-prometheus"
        if [ -d "${KUBE_PROMETHEUS_DIR}" ]; then
            $KUBECTL delete -f "${KUBE_PROMETHEUS_DIR}/manifests/" --ignore-not-found=true 2>/dev/null || true
            $KUBECTL delete -f "${KUBE_PROMETHEUS_DIR}/manifests/setup" --ignore-not-found=true 2>/dev/null || true
            rm -rf "${KUBE_PROMETHEUS_DIR}"
        fi
    fi

    ok "Cleanup complete"
}

# ---------------------------------------------------------------------------
# Phase dispatch
# ---------------------------------------------------------------------------

for phase in "${PHASES[@]}"; do
    case "${phase}" in
        prereqs)     phase_prereqs ;;
        extras)      phase_extras ;;
        clean)       phase_clean ;;
        *)           fail "Unknown phase: ${phase}" ;;
    esac
done
