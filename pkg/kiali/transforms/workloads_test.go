package transforms

import (
	"encoding/json"
	"testing"

	kialitypes "github.com/containers/kubernetes-mcp-server/pkg/kiali/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformWorkloadsListResponse(t *testing.T) {
	t.Run("empty payload returns error", func(t *testing.T) {
		_, err := TransformWorkloadsListResponse("")
		require.Error(t, err)
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		_, err := TransformWorkloadsListResponse(`{invalid`)
		require.Error(t, err)
	})

	t.Run("empty cluster uses default", func(t *testing.T) {
		payload := `{"cluster":"","workloads":[],"validations":null}`
		out, err := TransformWorkloadsListResponse(payload)
		require.NoError(t, err)
		require.NotNil(t, out)
		assert.Contains(t, out, "default")
		assert.Len(t, out["default"], 0)
	})

	t.Run("transforms workload with istio refs and labels", func(t *testing.T) {
		payload := `{
			"cluster": "Kubernetes",
			"workloads": [
				{
					"name": "productpage-v1",
					"namespace": "bookinfo",
					"cluster": "Kubernetes",
					"gvk": {"Group": "apps", "Version": "v1", "Kind": "Deployment"},
					"labels": {"app": "productpage", "version": "v1"},
					"health": {"status": {"status": "Healthy"}},
					"istioReferences": [
						{"objectGVK": {"Group": "networking.istio.io", "Kind": "Gateway"}, "name": "bookinfo-gateway", "namespace": "bookinfo", "cluster": ""}
					],
					"appLabel": true,
					"versionLabel": true
				}
			],
			"validations": {}
		}`
		out, err := TransformWorkloadsListResponse(payload)
		require.NoError(t, err)
		require.Len(t, out["Kubernetes"], 1)
		w := out["Kubernetes"][0]
		assert.Equal(t, "productpage-v1", w.Name)
		assert.Equal(t, "bookinfo", w.Namespace)
		assert.Equal(t, "Healthy", w.Health)
		assert.Equal(t, "Deployment", w.Type)
		assert.Contains(t, w.Details, "bookinfo-gateway(GW)")
		assert.Contains(t, w.Labels, "app=productpage")
	})

	t.Run("missing version label adds details", func(t *testing.T) {
		payload := `{
			"cluster": "c1",
			"workloads": [{"name": "w1", "namespace": "ns1", "gvk": {"Kind": "Deployment"}, "labels": {"app": "w1"}, "health": {"status": {"status": ""}}, "istioReferences": [], "appLabel": true, "versionLabel": false}],
			"validations": null
		}`
		out, err := TransformWorkloadsListResponse(payload)
		require.NoError(t, err)
		require.Len(t, out["c1"], 1)
		assert.Contains(t, out["c1"][0].Details, "Missing Version label")
	})

	t.Run("roundtrip marshal transform", func(t *testing.T) {
		resp := kialitypes.WorkloadsListResponse{
			Cluster: "Kubernetes",
			Workloads: []kialitypes.WorkloadListItem{
				{
					Name:         "details-v1",
					Namespace:    "bookinfo",
					Cluster:      "Kubernetes",
					GVK:          kialitypes.WorkloadGVK{Kind: "Deployment"},
					Labels:       map[string]string{"app": "details", "version": "v1"},
					Health:       kialitypes.WorkloadListHealth{Status: kialitypes.WorkloadHealthStatus{Status: "Healthy"}},
					IstioRefs:    nil,
					AppLabel:     true,
					VersionLabel: true,
					IstioSidecar: true,
				},
			},
			Validations: kialitypes.WorkloadsValidations{},
		}
		payload, err := json.Marshal(resp)
		require.NoError(t, err)
		out, err := TransformWorkloadsListResponse(string(payload))
		require.NoError(t, err)
		require.Len(t, out["Kubernetes"], 1)
		assert.Equal(t, "details-v1", out["Kubernetes"][0].Name)
		assert.Equal(t, "bookinfo", out["Kubernetes"][0].Namespace)
		assert.Equal(t, "Deployment", out["Kubernetes"][0].Type)
		assert.Equal(t, "Healthy", out["Kubernetes"][0].Health)
	})

	t.Run("istioSidecar false adds sidecar missing to details", func(t *testing.T) {
		payload := `{
			"cluster": "Kubernetes",
			"workloads": [
				{
					"name": "details-v1",
					"namespace": "bookinfo",
					"gvk": {"Group": "apps", "Version": "v1", "Kind": "Deployment"},
					"labels": {"app": "details", "version": "v1"},
					"health": {"status": {"status": "Healthy"}},
					"istioReferences": [],
					"appLabel": true,
					"versionLabel": true,
					"istioSidecar": false
				}
			],
			"validations": {}
		}`
		out, err := TransformWorkloadsListResponse(payload)
		require.NoError(t, err)
		require.Len(t, out["Kubernetes"], 1)
		assert.Contains(t, out["Kubernetes"][0].Details, "Istio sidecar container not found")
		assert.Contains(t, out["Kubernetes"][0].Details, "istio-injection")
	})
}
