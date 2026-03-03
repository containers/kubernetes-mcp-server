package transforms

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	kialitypes "github.com/containers/kubernetes-mcp-server/pkg/kiali/types"
)

// Raw mesh graph structures (Kiali /api/mesh/graph response).

type rawMeshGraph struct {
	Elements  rawMeshElements `json:"elements"`
	MeshNames []string        `json:"meshNames"`
	Timestamp int64           `json:"timestamp"`
}

type rawMeshElements struct {
	Nodes []rawMeshNode `json:"nodes"`
	Edges []rawMeshEdge `json:"edges"`
}

type rawMeshNode struct {
	Data rawMeshNodeData `json:"data"`
}

type rawMeshNodeData struct {
	ID         string          `json:"id"`
	Parent     string          `json:"parent"`
	InfraName  string          `json:"infraName"`
	InfraType  string          `json:"infraType"`
	Namespace  string          `json:"namespace"`
	NodeType   string          `json:"nodeType"`
	HealthData interface{}     `json:"healthData"` // "Healthy", "Unreachable", or null
	InfraData  json.RawMessage `json:"infraData"`  // object or array
	Version    string          `json:"version"`
	IsBox      string          `json:"isBox"`
}

type rawMeshEdge struct {
	Data struct {
		Source string `json:"source"`
		Target string `json:"target"`
	} `json:"data"`
}

// For Data Plane node, infraData is an array of namespace objects.
type rawNamespaceInDataPlane struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
}

// TransformMeshStatus converts raw Kiali mesh graph JSON into MeshSummaryFormatted.
func TransformMeshStatus(rawJSON string) (*kialitypes.MeshSummaryFormatted, error) {
	var raw rawMeshGraph
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		return nil, fmt.Errorf("unmarshal mesh graph: %w", err)
	}

	out := &kialitypes.MeshSummaryFormatted{
		Environment: kialitypes.MeshSummaryEnvironment{
			TrustDomain:  firstOrEmpty(raw.MeshNames),
			IstioVersion: "",
			KialiVersion: "",
			Timestamp:    formatMeshTimestamp(raw.Timestamp),
		},
		Components: kialitypes.MeshSummaryComponents{
			ControlPlane:       kialitypes.MeshSummaryControlPlane{Nodes: []string{}},
			ObservabilityStack: kialitypes.MeshSummaryObservabilityStack{},
			DataPlane:          kialitypes.MeshSummaryDataPlane{MonitoredNamespaces: []string{}, IstioInjection: "unknown"},
		},
		ConnectivityGraph: []kialitypes.MeshSummaryEdge{},
		CriticalAlerts:    []kialitypes.MeshSummaryCriticalAlert{},
	}

	idToName := make(map[string]string)
	idToHealth := make(map[string]string)

	for i := range raw.Elements.Nodes {
		n := &raw.Elements.Nodes[i]
		d := &n.Data
		idToName[d.ID] = toComponentName(d.InfraName, d.InfraType)
		health := healthDataToString(d.HealthData)
		idToHealth[d.ID] = health

		switch d.InfraType {
		case "istiod":
			out.Environment.IstioVersion = d.Version
			out.Components.ControlPlane.Status = health
			out.Components.ControlPlane.Nodes = append(out.Components.ControlPlane.Nodes, "istiod")
			if out.Environment.TrustDomain == "" && len(d.InfraData) > 0 {
				out.Environment.TrustDomain = extractTrustDomain(d.InfraData)
			}
		case "kiali":
			out.Environment.KialiVersion = d.Version
		case "dataplane":
			nsNames, injection := extractDataPlaneInfo(d.InfraData)
			out.Components.DataPlane.MonitoredNamespaces = nsNames
			out.Components.DataPlane.IstioInjection = injection
		}
		// Observability by infraName (Prometheus, jaeger, Grafana)
		infraLower := strings.ToLower(d.InfraName)
		switch infraLower {
		case "prometheus":
			out.Components.ObservabilityStack.Prometheus = health
		case "jaeger":
			out.Components.ObservabilityStack.Jaeger = health
		case "grafana":
			out.Components.ObservabilityStack.Grafana = health
		case "tempo":
			out.Components.ObservabilityStack.Tempo = health
		case "otel":
			out.Components.ObservabilityStack.OTel = health
		case "zipkin":
			out.Components.ObservabilityStack.Zipkin = health
		}

		if health == "Unreachable" || health == "Unhealthy" {
			msg := fmt.Sprintf("%s is marked as %s", toComponentName(d.InfraName, d.InfraType), health)
			if health == "Unreachable" {
				msg += "/Inaccessible"
			}
			msg += "."
			out.CriticalAlerts = append(out.CriticalAlerts, kialitypes.MeshSummaryCriticalAlert{
				Impact:  "Observability",
				Message: msg,
			})
		}
	}

	for _, e := range raw.Elements.Edges {
		fromName := idToName[e.Data.Source]
		toName := idToName[e.Data.Target]
		if fromName == "" {
			fromName = e.Data.Source
		}
		if toName == "" {
			toName = e.Data.Target
		}
		toHealth := idToHealth[e.Data.Target]
		status := "ok"
		note := ""
		if toHealth == "Unreachable" || toHealth == "Unhealthy" {
			status = "error"
			note = fmt.Sprintf("%s is %s", toName, strings.ToLower(toHealth))
		}
		out.ConnectivityGraph = append(out.ConnectivityGraph, kialitypes.MeshSummaryEdge{
			From:   fromName,
			To:     toName,
			Status: status,
			Note:   note,
		})
	}

	return out, nil
}

func firstOrEmpty(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	return ss[0]
}

func formatMeshTimestamp(unix int64) string {
	if unix == 0 {
		return ""
	}
	return time.Unix(unix, 0).UTC().Format(time.RFC3339)
}

func healthDataToString(v interface{}) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func toComponentName(infraName, infraType string) string {
	name := infraName
	if name == "" {
		name = infraType
	}
	return strings.ToLower(strings.TrimSpace(name))
}

func extractTrustDomain(infraData json.RawMessage) string {
	var top struct {
		Config struct {
			EffectiveConfig struct {
				ConfigMap struct {
					Mesh struct {
						TrustDomain string `json:"trustDomain"`
					} `json:"mesh"`
				} `json:"configMap"`
			} `json:"effectiveConfig"`
		} `json:"config"`
	}
	if err := json.Unmarshal(infraData, &top); err != nil {
		return ""
	}
	return top.Config.EffectiveConfig.ConfigMap.Mesh.TrustDomain
}

func extractDataPlaneInfo(infraData json.RawMessage) (names []string, injection string) {
	var arr []rawNamespaceInDataPlane
	if err := json.Unmarshal(infraData, &arr); err != nil {
		return nil, "unknown"
	}
	for _, ns := range arr {
		if ns.Name != "" {
			names = append(names, ns.Name)
		}
		if injection == "unknown" && ns.Labels != nil {
			if v, ok := ns.Labels["istio-injection"]; ok && v != "" {
				injection = v
			}
		}
	}
	if injection == "" {
		injection = "unknown"
	}
	return names, injection
}
