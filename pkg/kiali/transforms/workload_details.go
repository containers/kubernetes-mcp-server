package transforms

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	kialitypes "github.com/containers/kubernetes-mcp-server/pkg/kiali/types"
)

// TransformWorkloadDetailsResponse unmarshals the Kiali workload details JSON and returns
// a simplified formatted structure (workload, status, istio, pods, associated_services).
func TransformWorkloadDetailsResponse(jsonPayload string) (*kialitypes.WorkloadDetailsFormatted, error) {
	var raw kialitypes.WorkloadDetailsRaw
	if err := json.Unmarshal([]byte(jsonPayload), &raw); err != nil {
		return nil, fmt.Errorf("unmarshal workload details response: %w", err)
	}

	// Workload section
	labels := raw.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	serviceAccount := ""
	if len(raw.ServiceAccountNames) > 0 {
		serviceAccount = raw.ServiceAccountNames[0]
	}

	// Status section: traffic rates (inbound/outbound from health.requests)
	inboundRate := ""
	outboundRate := ""
	if raw.Health.Requests.Inbound != nil {
		if httpRates, ok := raw.Health.Requests.Inbound["http"]; ok && httpRates != nil {
			if r, ok := httpRates["200"]; ok {
				inboundRate = formatPercent(r)
			}
		}
	}
	if raw.Health.Requests.Outbound != nil {
		if httpRates, ok := raw.Health.Requests.Outbound["http"]; ok && httpRates != nil {
			if r, ok := httpRates["200"]; ok {
				outboundRate = formatPercent(r)
				if r > 1 {
					outboundRate = "100%"
				}
			}
		}
	}

	// Istio: mode, proxy version from first pod's istio container image, sync_status from first pod, validations
	istioMode := "None"
	if raw.IstioSidecar {
		istioMode = "Sidecar"
	} else if raw.IsAmbient {
		istioMode = "Ambient"
	}

	proxyVersion := ""
	syncStatus := make(map[string]string)
	for _, p := range raw.Pods {
		for _, c := range p.IstioInitContainers {
			if c.IsProxy && c.Image != "" {
				if tag := extractImageTag(c.Image); tag != "" {
					proxyVersion = tag
					break
				}
			}
		}
		if proxyVersion == "" {
			for _, c := range p.Containers {
				if c.IsProxy && c.Image != "" {
					if tag := extractImageTag(c.Image); tag != "" {
						proxyVersion = tag
						break
					}
				}
			}
		}
		if len(p.ProxyStatus) > 0 {
			syncStatus = p.ProxyStatus
			break
		}
	}

	validationNames := collectWorkloadValidationNames(raw.Validations)

	// Pods
	pods := make([]kialitypes.WorkloadDetailsPodFormatted, 0, len(raw.Pods))
	for _, p := range raw.Pods {
		containers := make([]string, 0)
		for _, c := range p.Containers {
			if !c.IsProxy {
				containers = append(containers, c.Name)
			}
		}
		istioInit := "—"
		istioProxy := "—"
		for _, c := range p.IstioInitContainers {
			if !c.IsProxy {
				continue
			}
			ready := "Not Ready"
			if c.IsReady {
				ready = "Ready"
			}
			nameLower := strings.ToLower(c.Name)
			if strings.Contains(nameLower, "init") {
				istioInit = ready
			} else if strings.Contains(nameLower, "proxy") {
				istioProxy = ready
			}
		}
		// If we only have one proxy container (e.g. combined), use it for both
		if istioProxy == "—" && istioInit != "—" {
			istioProxy = istioInit
		}
		if istioInit == "—" && istioProxy != "—" {
			istioInit = istioProxy
		}
		pods = append(pods, kialitypes.WorkloadDetailsPodFormatted{
			Name:       p.Name,
			Status:     p.Status,
			Containers: containers,
			IstioInit:  istioInit,
			IstioProxy: istioProxy,
		})
	}

	// Associated services
	associatedServices := make([]string, 0, len(raw.Services))
	for _, s := range raw.Services {
		if s.Name != "" {
			associatedServices = append(associatedServices, s.Name)
		}
	}
	sort.Strings(associatedServices)

	return &kialitypes.WorkloadDetailsFormatted{
		Workload: kialitypes.WorkloadDetailsWorkloadFormatted{
			Name:           raw.Name,
			Namespace:      raw.Namespace,
			Kind:           raw.GVK.Kind,
			Labels:         labels,
			ServiceAccount: serviceAccount,
			CreatedAt:      raw.CreatedAt,
		},
		Status: kialitypes.WorkloadDetailsStatusFormatted{
			Overall: raw.Health.Status.Status,
			Replicas: kialitypes.WorkloadDetailsReplicasFormatted{
				Desired:   raw.DesiredReplicas,
				Current:   raw.CurrentReplicas,
				Available: raw.AvailableReplicas,
			},
			TrafficSuccessRate: kialitypes.WorkloadDetailsTrafficFormatted{
				Inbound:  inboundRate,
				Outbound: outboundRate,
			},
		},
		Istio: kialitypes.WorkloadDetailsIstioFormatted{
			Mode:         istioMode,
			ProxyVersion: proxyVersion,
			SyncStatus:   syncStatus,
			Validations:  validationNames,
		},
		Pods:               pods,
		AssociatedServices: associatedServices,
	}, nil
}

// collectWorkloadValidationNames returns validation entry names from Istio config (excludes "workload" category).
func collectWorkloadValidationNames(validations map[string]map[string]kialitypes.ValidationEntry) []string {
	var names []string
	if validations == nil {
		return names
	}
	for key, inner := range validations {
		if key == "workload" {
			continue
		}
		for _, entry := range inner {
			if entry.Name != "" {
				names = append(names, entry.Name)
			}
		}
	}
	sort.Strings(names)
	return names
}

// extractImageTag returns the tag from an image string (e.g. "docker.io/istio/proxyv2:1.28.0" -> "1.28.0").
func extractImageTag(image string) string {
	if image == "" {
		return ""
	}
	last := strings.LastIndex(image, ":")
	if last < 0 {
		return ""
	}
	tag := strings.TrimSpace(image[last+1:])
	// Ignore digest (e.g. sha256:...)
	if strings.HasPrefix(tag, "sha256:") {
		return ""
	}
	return tag
}
