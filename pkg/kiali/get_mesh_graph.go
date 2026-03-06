package kiali

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/containers/kubernetes-mcp-server/pkg/kiali/transforms"
	kialitypes "github.com/containers/kubernetes-mcp-server/pkg/kiali/types"
)

// GetMeshStatusResponse contains the combined response from multiple Kiali API endpoints.
// Note: Health data is fetched from Kiali's health API and used internally to compute
// MeshHealthSummary, but the raw health data is not included in the response to reduce payload size.
// MeshHealthSummary contains all the key aggregated metrics needed for mesh health overview.
// MeshSummary is a simplified view derived from the raw mesh_status (mesh graph) API.
type GetMeshStatusResponse struct {
	MeshStatus        json.RawMessage               `json:"mesh_status,omitempty"`
	Namespaces        json.RawMessage               `json:"namespaces,omitempty"`
	MeshHealthSummary *kialitypes.MeshHealthSummary `json:"mesh_health_summary,omitempty"`
	Errors            map[string]string             `json:"errors,omitempty"`
}

// GetMeshStatus fetches the mesh status from the Kiali API.
// This returns information about mesh components like Istio, Kiali, Grafana, Prometheus
// and their interactions, versions, and health status.
func (k *Kiali) GetMeshStatus(ctx context.Context, namespaces []string, queryParams map[string]string) (string, error) {
	resp := GetMeshStatusResponse{
		Errors: make(map[string]string),
	}

	var errorsMu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(3)

	// Health - compute MeshHealthSummary inside the goroutine
	go func() {
		defer wg.Done()
		data, err := k.getHealth(ctx, namespaces, queryParams)
		if err != nil {
			errorsMu.Lock()
			resp.Errors["health"] = err.Error()
			errorsMu.Unlock()
			return
		}
		// Compute mesh health summary from health data
		if len(data) > 0 {
			summary := computeMeshHealthSummary(data, namespaces, queryParams)
			if summary != nil {
				resp.MeshHealthSummary = summary
			}
		}
	}()

	// Mesh status (raw + transformed summary)
	go func() {
		defer wg.Done()
		data, err := k.getMeshStatus(ctx)
		if err != nil {
			errorsMu.Lock()
			resp.Errors["mesh_status"] = err.Error()
			errorsMu.Unlock()
			return
		}
		resp.MeshStatus = data
		summary, err := transforms.TransformMeshStatus(string(data))
		if err == nil && summary != nil {
			errorsMu.Lock()
			summaryJSON, err := json.Marshal(summary)
			if err == nil {
				resp.MeshStatus = summaryJSON
			}
			errorsMu.Unlock()
		}
	}()

	// Namespaces
	go func() {
		defer wg.Done()
		data, err := k.getNamespaces(ctx, namespaces, queryParams)
		if err != nil {
			errorsMu.Lock()
			resp.Errors["namespaces"] = err.Error()
			errorsMu.Unlock()
			return
		}
		resp.Namespaces = data
	}()

	wg.Wait()

	// If no errors occurred, omit the errors map in the final JSON
	if len(resp.Errors) == 0 {
		resp.Errors = nil
	}

	encoded, err := json.Marshal(resp)
	if err != nil {
		return "", err
	}
	return string(encoded), nil

}

// GetMeshGraph fetches multiple Kiali endpoints in parallel and returns a combined response.
// Each field in the response corresponds to one API call result.
// - graph:       /api/namespaces/graph (optionally filtered by namespaces)
// - mesh_status: /api/mesh/graph
// - namespaces:  /api/namespaces
// - mesh_health_summary: computed from /api/clusters/health (health data is fetched but not included in response)
func (k *Kiali) GetMeshGraph(ctx context.Context, namespaces []string, queryParams map[string]string) (string, error) {

	// Graph
	data, err := k.getGraph(ctx, namespaces, queryParams)
	if err != nil {
		return "", err
	}

	encoded, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// getGraph wraps the Graph call and returns raw JSON.
func (k *Kiali) getGraph(ctx context.Context, namespaces []string, queryParams map[string]string) (json.RawMessage, error) {
	out, err := k.Graph(ctx, namespaces, queryParams)
	if err != nil {
		return nil, err
	}
	return json.RawMessage([]byte(out)), nil
}

// getHealth wraps the Health call and returns raw JSON.
func (k *Kiali) getHealth(ctx context.Context, namespaces []string, queryParams map[string]string) (json.RawMessage, error) {
	nsParam := strings.Join(namespaces, ",")
	out, err := k.Health(ctx, nsParam, queryParams)
	if err != nil {
		return nil, err
	}
	return json.RawMessage([]byte(out)), nil
}

// getMeshStatus wraps the MeshStatus call and returns raw JSON.
func (k *Kiali) getMeshStatus(ctx context.Context) (json.RawMessage, error) {
	out, err := k.MeshStatus(ctx)
	if err != nil {
		return nil, err
	}
	return json.RawMessage([]byte(out)), nil
}

// getNamespaces wraps the ListNamespaces call and returns raw JSON.
func (k *Kiali) getNamespaces(ctx context.Context, namespaces []string, queryParams map[string]string) (json.RawMessage, error) {
	out, err := k.ListNamespaces(ctx, namespaces, queryParams)
	if err != nil {
		return nil, err
	}
	return json.RawMessage([]byte(out)), nil
}
