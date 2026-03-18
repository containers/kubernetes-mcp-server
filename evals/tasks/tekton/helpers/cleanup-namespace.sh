#!/usr/bin/env bash
# Shared namespace cleanup helper for Tekton eval tasks.
# Deletes the tekton-eval namespace and waits for it to be fully removed,
# stripping finalizers from TaskRuns and PipelineRuns to prevent stuck deletion.
#
# Usage: source this script from a task.yaml setup script:
#   source "$(dirname "${BASH_SOURCE[0]}")/../../helpers/cleanup-namespace.sh"
#   cleanup_tekton_namespace

cleanup_tekton_namespace() {
    local namespace="${1:-tekton-eval}"
    local timeout="${2:-60}"

    for i in $(seq 1 "$timeout"); do
        if ! kubectl get namespace "$namespace" > /dev/null 2>&1; then
            echo "Namespace $namespace is gone"
            return 0
        fi
        for tr in $(kubectl get taskrun -n "$namespace" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
            kubectl patch taskrun -n "$namespace" "$tr" --type=json -p='[{"op":"remove","path":"/metadata/finalizers"}]' 2>/dev/null || true
        done
        for pr in $(kubectl get pipelinerun -n "$namespace" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
            kubectl patch pipelinerun -n "$namespace" "$pr" --type=json -p='[{"op":"remove","path":"/metadata/finalizers"}]' 2>/dev/null || true
        done
        sleep 3
    done
    echo "Timeout waiting for namespace deletion"
    return 1
}
