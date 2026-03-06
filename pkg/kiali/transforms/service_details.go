package transforms

import (
	"encoding/json"
	"fmt"
	"sort"

	kialitypes "github.com/containers/kubernetes-mcp-server/pkg/kiali/types"
)

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
			if addr.Kind != "Pod" {
				podName = addr.Name
			}
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
