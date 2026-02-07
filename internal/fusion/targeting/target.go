package targeting

import (
	"fmt"
	"strings"
)

// TargetType defines how clusters are targeted
type TargetType string

const (
	// TargetSingle targets a single cluster
	TargetSingle TargetType = "single"
	// TargetMulti targets multiple explicitly named clusters
	TargetMulti TargetType = "multi"
	// TargetFleet targets all clusters in a fleet/hub
	TargetFleet TargetType = "fleet"
	// TargetSelector targets clusters matching label selectors
	TargetSelector TargetType = "selector"
	// TargetAll targets all registered clusters
	TargetAll TargetType = "all"
)

// Target defines how to target clusters for an operation
type Target struct {
	// Type specifies the targeting strategy
	Type TargetType `json:"type"`

	// Cluster specifies a single cluster name (for TargetSingle)
	Cluster string `json:"cluster,omitempty"`

	// Clusters specifies multiple cluster names (for TargetMulti)
	Clusters []string `json:"clusters,omitempty"`

	// Fleet specifies a fleet/hub name (for TargetFleet)
	Fleet string `json:"fleet,omitempty"`

	// Selector specifies label selectors (for TargetSelector)
	// Format: "key1=value1,key2=value2"
	Selector string `json:"selector,omitempty"`

	// Timeout specifies operation timeout in seconds (optional)
	Timeout int `json:"timeout,omitempty"`
}

// Validate checks if the target configuration is valid
func (t *Target) Validate() error {
	if t == nil {
		return fmt.Errorf("target cannot be nil")
	}

	switch t.Type {
	case TargetSingle:
		if t.Cluster == "" {
			return fmt.Errorf("cluster name required for single target")
		}
	case TargetMulti:
		if len(t.Clusters) == 0 {
			return fmt.Errorf("at least one cluster required for multi target")
		}
	case TargetFleet:
		if t.Fleet == "" {
			return fmt.Errorf("fleet name required for fleet target")
		}
	case TargetSelector:
		if t.Selector == "" {
			return fmt.Errorf("selector required for selector target")
		}
	case TargetAll:
		// No additional validation needed
	case "":
		// Default to single cluster if not specified
		t.Type = TargetSingle
	default:
		return fmt.Errorf("invalid target type: %s", t.Type)
	}

	return nil
}

// GetClusterNames returns the list of cluster names to target
// This is a helper that resolves the target to actual cluster names
func (t *Target) GetClusterNames(availableClusters []string) ([]string, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}

	switch t.Type {
	case TargetSingle:
		return []string{t.Cluster}, nil

	case TargetMulti:
		return t.Clusters, nil

	case TargetAll:
		return availableClusters, nil

	case TargetFleet:
		// Filter clusters by fleet prefix or label
		// For now, simple prefix matching
		var fleetClusters []string
		fleetPrefix := t.Fleet + "-"
		for _, cluster := range availableClusters {
			if strings.HasPrefix(cluster, fleetPrefix) || cluster == t.Fleet {
				fleetClusters = append(fleetClusters, cluster)
			}
		}
		if len(fleetClusters) == 0 {
			return nil, fmt.Errorf("no clusters found for fleet: %s", t.Fleet)
		}
		return fleetClusters, nil

	case TargetSelector:
		// Parse selector and match clusters
		// For now, simple key=value matching
		// In production, this would use proper label matching
		var selectedClusters []string
		selectors := parseSelector(t.Selector)

		for _, cluster := range availableClusters {
			if matchesSelector(cluster, selectors) {
				selectedClusters = append(selectedClusters, cluster)
			}
		}

		if len(selectedClusters) == 0 {
			return nil, fmt.Errorf("no clusters match selector: %s", t.Selector)
		}
		return selectedClusters, nil

	default:
		return nil, fmt.Errorf("unsupported target type: %s", t.Type)
	}
}

// parseSelector parses a selector string into key-value pairs
func parseSelector(selector string) map[string]string {
	selectors := make(map[string]string)
	pairs := strings.Split(selector, ",")

	for _, pair := range pairs {
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) == 2 {
			selectors[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}

	return selectors
}

// matchesSelector checks if a cluster name matches selectors
// This is a simplified implementation
func matchesSelector(clusterName string, selectors map[string]string) bool {
	// In a real implementation, this would check cluster labels
	// For now, we do simple string matching on cluster name
	for key, value := range selectors {
		if key == "name" && !strings.Contains(clusterName, value) {
			return false
		}
		if key == "env" {
			// Check if cluster name contains environment indicator
			if !strings.Contains(strings.ToLower(clusterName), strings.ToLower(value)) {
				return false
			}
		}
	}
	return true
}

// Result represents the result of an operation across clusters
type Result struct {
	// Target describes how clusters were targeted
	Target Target `json:"target"`

	// ClusterResults contains per-cluster results
	ClusterResults map[string]ClusterResult `json:"clusterResults"`

	// Summary provides an aggregated summary
	Summary interface{} `json:"summary,omitempty"`

	// Errors contains any cluster-level errors
	Errors map[string]string `json:"errors,omitempty"`
}

// ClusterResult represents the result from a single cluster
type ClusterResult struct {
	// ClusterName identifies the cluster
	ClusterName string `json:"clusterName"`

	// Data contains the cluster-specific result
	Data interface{} `json:"data,omitempty"`

	// Error contains any error that occurred
	Error string `json:"error,omitempty"`

	// Success indicates if the operation succeeded
	Success bool `json:"success"`
}

// NewResult creates a new Result with the given target
func NewResult(target Target) *Result {
	return &Result{
		Target:         target,
		ClusterResults: make(map[string]ClusterResult),
		Errors:         make(map[string]string),
	}
}

// AddClusterResult adds a result for a specific cluster
func (r *Result) AddClusterResult(clusterName string, data interface{}, err error) {
	result := ClusterResult{
		ClusterName: clusterName,
		Data:        data,
		Success:     err == nil,
	}

	if err != nil {
		result.Error = err.Error()
		r.Errors[clusterName] = err.Error()
	}

	r.ClusterResults[clusterName] = result
}

// HasErrors returns true if any cluster operation failed
func (r *Result) HasErrors() bool {
	return len(r.Errors) > 0
}

// SuccessCount returns the number of successful cluster operations
func (r *Result) SuccessCount() int {
	count := 0
	for _, result := range r.ClusterResults {
		if result.Success {
			count++
		}
	}
	return count
}

// FailureCount returns the number of failed cluster operations
func (r *Result) FailureCount() int {
	return len(r.Errors)
}

// TotalCount returns the total number of cluster operations
func (r *Result) TotalCount() int {
}

// TargetSchema returns the JSON schema for the target input parameter
func TargetSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: jsonschema.Type{jsonschema.TypeObject},
		Properties: map[string]*jsonschema.Schema{
			"type": {
				Type: jsonschema.Type{jsonschema.TypeString},
				Enum: []interface{}{"single", "multi", "fleet", "selector", "all"},
				Description: "Targeting strategy: single (one cluster), multi (specific clusters), fleet (all in fleet), selector (label-based), all (all registered)",
			},
			"cluster": {
				Type: jsonschema.Type{jsonschema.TypeString},
				Description: "Single cluster name (for type=single)",
			},
			"clusters": {
				Type: jsonschema.Type{jsonschema.TypeArray},
				Items: &jsonschema.Schema{
					Type: jsonschema.Type{jsonschema.TypeString},
				},
				Description: "List of cluster names (for type=multi)",
			},
			"fleet": {
				Type: jsonschema.Type{jsonschema.TypeString},
				Description: "Fleet name (for type=fleet)",
			},
			"selector": {
				Type: jsonschema.Type{jsonschema.TypeString},
				Description: "Label selector (for type=selector), format: key1=value1,key2=value2",
			},
			"timeout": {
				Type: jsonschema.Type{jsonschema.TypeInteger},
				Description: "Operation timeout in seconds (default: 30)",
			},
		},
	}
}

// ResolveClusterNames resolves the target to actual cluster names using the registry
func (t *Target) ResolveClusterNames(registry interface{}) ([]string, error) {
	// This is a placeholder - actual implementation would query the registry
	// For now, return based on target type
	if err := t.Validate(); err != nil {
		return nil, err
	}

	switch t.Type {
	case TargetSingle:
		if t.Cluster == "" {
			return []string{"default"}, nil
		}
		return []string{t.Cluster}, nil
	case TargetMulti:
		return t.Clusters, nil
	case TargetAll, TargetFleet:
		// Would query registry for all clusters
		return []string{"default"}, nil
	default:
		return []string{"default"}, nil
	}
}

// Made with Bob
	return len(r.ClusterResults)
}

// Made with Bob
