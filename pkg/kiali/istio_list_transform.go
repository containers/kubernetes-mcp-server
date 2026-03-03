package kiali

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	kialitypes "github.com/containers/kubernetes-mcp-server/pkg/kiali/types"
)

// Raw config API response: configs.resources and configs.validations
type rawIstioConfigs struct {
	Resources   map[string][]rawIstioResourceItem   `json:"resources"`
	Validations map[string]map[string]rawValidation `json:"validations"`
}

type rawIstioResourceItem struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Kind string `json:"kind"`
}

type rawValidation struct {
	Valid  bool       `json:"valid"`
	Checks []rawCheck `json:"checks"`
}

type rawCheck struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
}

// Validations list API response: array of per-namespace summary
type rawValidationsListEntry struct {
	Namespace   string `json:"namespace"`
	Cluster     string `json:"cluster"`
	Errors      int    `json:"errors"`
	Warnings    int    `json:"warnings"`
	ObjectCount int    `json:"objectCount"`
}

// gvkKey matches "networking.istio.io/v1, Kind=Gateway" to extract group, version, kind
var gvkKeyRe = regexp.MustCompile(`^(.+)/([^/]+),\s*Kind=(.+)$`)

// TransformIstioConfigList converts raw configs + validations list JSON into IstioListSummaryFormatted.
// Omits empty resource types and only includes validation_alerts for namespaces with warnings or errors.
func TransformIstioConfigList(configsJSON, validationsListJSON string) (*kialitypes.IstioListSummaryFormatted, error) {
	out := &kialitypes.IstioListSummaryFormatted{
		Summary:          kialitypes.IstioListSummary{UnhealthyNamespaces: []string{}},
		ActiveResources:  []kialitypes.IstioListActiveResource{},
		ValidationAlerts: []kialitypes.IstioListValidationAlert{},
	}

	var configs rawIstioConfigs
	if configsJSON != "" {
		if err := json.Unmarshal([]byte(configsJSON), &configs); err != nil {
			return nil, fmt.Errorf("unmarshal configs: %w", err)
		}
		// Some Kiali responses wrap in a "configs" key
		if len(configs.Resources) == 0 && len(configs.Validations) == 0 {
			var wrapped struct {
				Configs rawIstioConfigs `json:"configs"`
			}
			if err := json.Unmarshal([]byte(configsJSON), &wrapped); err == nil && (len(wrapped.Configs.Resources) > 0 || len(wrapped.Configs.Validations) > 0) {
				configs = wrapped.Configs
			}
		}
	}
	if configs.Resources == nil {
		configs.Resources = make(map[string][]rawIstioResourceItem)
	}
	if configs.Validations == nil {
		configs.Validations = make(map[string]map[string]rawValidation)
	}

	var listEntries []rawValidationsListEntry
	if validationsListJSON != "" {
		if err := json.Unmarshal([]byte(validationsListJSON), &listEntries); err != nil {
			return nil, fmt.Errorf("unmarshal validations list: %w", err)
		}
	}

	totalObjects := 0
	for gvkKey, items := range configs.Resources {
		if len(items) == 0 {
			continue
		}
		totalObjects += len(items)
		group, kind := parseGVKKey(gvkKey)
		names := make([]string, 0, len(items))
		for _, r := range items {
			if r.Metadata.Name != "" {
				names = append(names, r.Metadata.Name)
			}
		}
		status := statusForGVK(gvkKey, configs.Validations)
		out.ActiveResources = append(out.ActiveResources, kialitypes.IstioListActiveResource{
			Kind:   kind,
			Group:  group,
			Count:  len(items),
			Status: status,
			Items:  names,
		})
	}
	out.Summary.TotalObjects = totalObjects

	unhealthy := make(map[string]struct{})
	for _, e := range listEntries {
		if e.Warnings > 0 || e.Errors > 0 {
			if e.Namespace != "" {
				unhealthy[e.Namespace] = struct{}{}
			}
			out.ValidationAlerts = append(out.ValidationAlerts, kialitypes.IstioListValidationAlert{
				Namespace: e.Namespace,
				Cluster:   e.Cluster,
				Warnings:  e.Warnings,
				Errors:    e.Errors,
			})
		}
	}
	for ns := range unhealthy {
		out.Summary.UnhealthyNamespaces = append(out.Summary.UnhealthyNamespaces, ns)
	}
	sort.Strings(out.Summary.UnhealthyNamespaces)
	sort.Slice(out.ActiveResources, func(i, j int) bool {
		a, b := out.ActiveResources[i], out.ActiveResources[j]
		if a.Group != b.Group {
			return a.Group < b.Group
		}
		return a.Kind < b.Kind
	})

	return out, nil
}

func parseGVKKey(gvkKey string) (group, kind string) {
	m := gvkKeyRe.FindStringSubmatch(strings.TrimSpace(gvkKey))
	if len(m) != 4 {
		// fallback: split on ", Kind="
		if idx := strings.Index(gvkKey, ", Kind="); idx != -1 {
			groupPart := strings.TrimSpace(gvkKey[:idx])
			kind = strings.TrimSpace(gvkKey[idx+len(", Kind="):])
			if slash := strings.LastIndex(groupPart, "/"); slash != -1 {
				group = strings.TrimSpace(groupPart[:slash])
			} else {
				group = groupPart
			}
			return group, kind
		}
		return "", ""
	}
	group = strings.TrimSpace(m[1])
	kind = strings.TrimSpace(m[3])
	return group, kind
}

func statusForGVK(gvkKey string, validations map[string]map[string]rawValidation) string {
	byKey, ok := validations[gvkKey]
	if !ok {
		return "ok"
	}
	hasError := false
	hasWarning := false
	for _, v := range byKey {
		for _, c := range v.Checks {
			switch strings.ToLower(c.Severity) {
			case "error":
				hasError = true
			case "warning":
				hasWarning = true
			}
		}
	}
	if hasError {
		return "error"
	}
	if hasWarning {
		return "warning"
	}
	return "ok"
}
