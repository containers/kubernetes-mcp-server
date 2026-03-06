package transforms

import (
	"encoding/json"
	"fmt"

	kialitypes "github.com/containers/kubernetes-mcp-server/pkg/kiali/types"
)

// TransformWorkloadsListResponse unmarshals the Kiali workloads list JSON and returns
// workloads grouped by cluster. Each workload includes name, namespace, health, type (gvk.Kind),
// labels, and details (istioReferences as name(symbol) plus missing label warnings).
func TransformWorkloadsListResponse(jsonPayload string) (kialitypes.WorkloadsByCluster, error) {
	var resp kialitypes.WorkloadsListResponse
	if err := json.Unmarshal([]byte(jsonPayload), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal workloads list response: %w", err)
	}

	out := make(kialitypes.WorkloadsByCluster)
	cluster := resp.Cluster
	if cluster == "" {
		cluster = "default"
	}

	summaries := make([]kialitypes.WorkloadSummary, 0, len(resp.Workloads))
	for _, w := range resp.Workloads {
		summaries = append(summaries, toWorkloadSummary(w))
	}

	out[cluster] = summaries
	return out, nil
}

func toWorkloadSummary(w kialitypes.WorkloadListItem) kialitypes.WorkloadSummary {
	health := ""
	if w.Health.Status.Status != "" {
		health = w.Health.Status.Status
	}

	details := joinDetailParts(formatDetails(w.IstioRefs), formatMissingLabelDetails(w.AppLabel, w.VersionLabel), formatMissingSidecarDetail(w.IstioSidecar))
	labels := formatLabels(w.Labels)

	return kialitypes.WorkloadSummary{
		Name:      w.Name,
		Namespace: w.Namespace,
		Health:    health,
		Type:      w.GVK.Kind,
		Labels:    labels,
		Details:   details,
	}
}
