package transforms

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	kialitypes "github.com/containers/kubernetes-mcp-server/pkg/kiali/types"
)

// Istio kind to detail initial (e.g. Gateway -> GW, VirtualService -> VS).
var istioKindToInitial = map[string]string{
	"Gateway":             "GW",
	"VirtualService":      "VS",
	"DestinationRule":     "DR",
	"ServiceEntry":        "SE",
	"PeerAuthentication":  "PA",
	"AuthorizationPolicy": "AP",
}

// TransformServicesListResponse unmarshals the Kiali services list JSON and returns
// services grouped by cluster. Each service includes name, namespace, health,
// configuration (from validations), details (from istioReferences, e.g. "bookinfo(VS), bookinfo-gateway(GW)"), and labels.
func TransformServicesListResponse(jsonPayload string) (kialitypes.ServicesByCluster, error) {
	var resp kialitypes.ServicesListResponse
	if err := json.Unmarshal([]byte(jsonPayload), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal services list response: %w", err)
	}

	out := make(kialitypes.ServicesByCluster)
	cluster := resp.Cluster
	if cluster == "" {
		cluster = "default"
	}

	summaries := make([]kialitypes.ServiceSummary, 0, len(resp.Services))
	for _, svc := range resp.Services {
		summaries = append(summaries, toServiceSummary(svc, resp.Validations))
	}

	out[cluster] = summaries
	return out, nil
}

func toServiceSummary(svc kialitypes.ServiceListItem, val kialitypes.ServicesValidations) kialitypes.ServiceSummary {
	health := ""
	if svc.Health.Status.Status != "" {
		health = svc.Health.Status.Status
	}

	configStr := "True"
	if val.Service != nil {
		key := svc.Name + "." + svc.Namespace
		if v, ok := val.Service[key]; ok {
			if !v.Valid && len(v.Checks) > 0 {
				parts := make([]string, 0, len(v.Checks))
				for _, c := range v.Checks {
					if c.Code != "" {
						if c.Message != "" {
							parts = append(parts, c.Code+"("+c.Message+")")
						} else {
							parts = append(parts, c.Code)
						}
					}
				}
				configStr = strings.Join(parts, ", ")
			} else if !v.Valid {
				configStr = "False"
			}
		}
	}

	details := joinDetailParts(formatDetails(svc.IstioRefs), formatMissingLabelDetails(svc.AppLabel, svc.VersionLabel))
	labels := formatLabels(svc.Labels)

	return kialitypes.ServiceSummary{
		Name:          svc.Name,
		Namespace:     svc.Namespace,
		Health:        health,
		Configuration: configStr,
		Details:       details,
		Labels:        labels,
	}
}

func formatDetails(refs []kialitypes.IstioRef) string {
	if len(refs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		kind := r.ObjectGVK.Kind
		initial, ok := istioKindToInitial[kind]
		if !ok {
			initial = kind
		}
		name := r.Name
		if name == "" {
			name = "<no name>"
		}
		parts = append(parts, name+"("+initial+")")
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// formatMissingLabelDetails returns a detail string when app and/or version label are missing.
// Used in both services and workloads details.
func formatMissingLabelDetails(appLabel, versionLabel bool) string {
	if appLabel && versionLabel {
		return ""
	}
	if !appLabel && !versionLabel {
		return "Missing App and Version label (This workload won't be linked with an application. The label is recommended as it affects telemetry. Missing labels may impact telemetry reported by the Istio proxy.)"
	}
	if !appLabel {
		return "Missing App label (This workload won't be linked with an application.)"
	}
	return "Missing Version label (The label is recommended as it affects telemetry. Missing labels may impact telemetry reported by the Istio proxy.)"
}

// joinDetailParts joins non-empty detail parts with ", ".
func joinDetailParts(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ", ")
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return "None"
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(labels[k])
	}
	return b.String()
}

// TransformServiceDetailsResponse unmarshals the Kiali service details JSON and returns
// a simplified formatted structure (service, istio_config, workloads, health_status, inbound_success_rate_2xx, endpoints).
func TransformServiceDetailsResponse(jsonPayload string) (*kialitypes.ServiceDetailsFormatted, error) {
	var raw kialitypes.ServiceDetailsRaw
	if err := json.Unmarshal([]byte(jsonPayload), &raw); err != nil {
		return nil, fmt.Errorf("unmarshal service details response: %w", err)
	}

	// Service
	ports := make([]kialitypes.ServiceDetailsPortFormatted, 0, len(raw.Service.Ports))
	for _, p := range raw.Service.Ports {
		ports = append(ports, kialitypes.ServiceDetailsPortFormatted(p))
	}
	selectors := raw.Service.Selectors
	if selectors == nil {
		selectors = make(map[string]string)
	}

	// Istio config: validations = names from Istio resource validations (skip "service" and "workload")
	var validationNames []string
	if raw.Validations != nil {
		for key, inner := range raw.Validations {
			if key == "service" || key == "workload" {
				continue
			}
			for _, entry := range inner {
				if entry.Name != "" {
					validationNames = append(validationNames, entry.Name)
				}
			}
		}
		sort.Strings(validationNames)
	}

	vsNames := make([]string, 0, len(raw.VirtualServices))
	for _, vs := range raw.VirtualServices {
		if vs.Name != "" {
			vsNames = append(vsNames, vs.Name)
		}
	}
	drNames := make([]string, 0, len(raw.DestinationRules))
	for _, dr := range raw.DestinationRules {
		if dr.Name != "" {
			drNames = append(drNames, dr.Name)
		}
	}

	mtlsMode := raw.NamespaceMTLS.Status
	if raw.NamespaceMTLS.AutoMTLSEnabled {
		mtlsMode = "AUTO_ENABLED"
	} else if mtlsMode == "" {
		mtlsMode = "MTLS_NOT_ENABLED"
	}

	// Workloads
	workloads := make([]kialitypes.ServiceDetailsWorkloadFormatted, 0, len(raw.Workloads))
	for _, w := range raw.Workloads {
		sa := ""
		if len(w.ServiceAccountNames) > 0 {
			sa = w.ServiceAccountNames[0]
		}
		labels := w.Labels
		if labels == nil {
			labels = make(map[string]string)
		}
		workloads = append(workloads, kialitypes.ServiceDetailsWorkloadFormatted{
			Name:           w.Name,
			Kind:           w.GVK.Kind,
			Labels:         labels,
			ServiceAccount: sa,
			PodCount:       w.PodCount,
		})
	}

	// Health and inbound rate
	healthStatus := raw.Health.Status.Status
	inboundRate := ""
	if raw.Health.Requests.Inbound != nil {
		if httpRates, ok := raw.Health.Requests.Inbound["http"]; ok && httpRates != nil {
			if r200, ok := httpRates["200"]; ok {
				inboundRate = formatPercent(r200)
			}
		}
	}

	// Endpoints (pod name + ip)
	endpoints := make([]kialitypes.ServiceDetailsEndpointFormatted, 0)
	for _, ep := range raw.Endpoints {
		for _, addr := range ep.Addresses {
			podName := addr.Name
			if addr.IP != "" || podName != "" {
				endpoints = append(endpoints, kialitypes.ServiceDetailsEndpointFormatted{
					PodName: podName,
					IP:      addr.IP,
				})
			}
		}
	}

	return &kialitypes.ServiceDetailsFormatted{
		Service: kialitypes.ServiceDetailsServiceFormatted{
			Name:      raw.Service.Name,
			Namespace: raw.Service.Namespace,
			Type:      raw.Service.Type,
			IP:        raw.Service.IP,
			Ports:     ports,
			Selectors: selectors,
		},
		IstioConfig: kialitypes.ServiceDetailsIstioConfig{
			IsAmbient:        raw.IsAmbient,
			HasSidecar:       raw.IstioSidecar,
			MTLSMode:         mtlsMode,
			VirtualServices:  vsNames,
			DestinationRules: drNames,
			Validations:      validationNames,
		},
		Workloads:             workloads,
		HealthStatus:          healthStatus,
		InboundSuccessRate2xx: inboundRate,
		Endpoints:             endpoints,
	}, nil
}

// formatPercent formats a 0-1 rate as a percentage string (e.g. 0.977 -> "97.7%").
func formatPercent(rate float64) string {
	if rate <= 0 {
		return "0%"
	}
	if rate >= 1 {
		return "100%"
	}
	return fmt.Sprintf("%.1f%%", rate*100)
}
