package transforms

import (
	"testing"

	kialitypes "github.com/containers/kubernetes-mcp-server/pkg/kiali/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformWorkloadDetailsResponse(t *testing.T) {
	t.Run("empty payload returns error", func(t *testing.T) {
		_, err := TransformWorkloadDetailsResponse("")
		require.Error(t, err)
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		_, err := TransformWorkloadDetailsResponse(`{invalid`)
		require.Error(t, err)
	})

	t.Run("transforms full workload details", func(t *testing.T) {
		payload := `{
			"name": "productpage-v1",
			"namespace": "bookinfo",
			"cluster": "Kubernetes",
			"gvk": {"Group": "apps", "Version": "v1", "Kind": "Deployment"},
			"createdAt": "2026-03-03T07:20:48Z",
			"labels": {"app": "productpage", "version": "v1"},
			"serviceAccountNames": ["bookinfo-productpage"],
			"desiredReplicas": 1,
			"currentReplicas": 1,
			"availableReplicas": 1,
			"istioSidecar": true,
			"isAmbient": false,
			"pods": [
				{
					"name": "productpage-v1-846b9898b9-tvdd7",
					"status": "Running",
					"containers": [{"name": "productpage", "image": "quay.io/example:1.0", "isProxy": false, "isReady": true}],
					"istioInitContainers": [
						{"name": "istio-init", "image": "docker.io/istio/proxyv2:1.28.0", "isProxy": true, "isReady": true},
						{"name": "istio-proxy", "image": "docker.io/istio/proxyv2:1.28.0", "isProxy": true, "isReady": true}
					],
					"proxyStatus": {"CDS": "Synced", "EDS": "Synced", "LDS": "Synced", "RDS": "Synced"}
				}
			],
			"services": [{"name": "productpage"}],
			"validations": {
				"networking.istio.io/v1, Kind=Gateway": {"bookinfo-gateway.bookinfo": {"name": "bookinfo-gateway"}},
				"networking.istio.io/v1, Kind=VirtualService": {"bookinfo.bookinfo": {"name": "bookinfo"}},
				"workload": {"productpage-v1.bookinfo": {"name": "productpage-v1"}}
			},
			"health": {
				"requests": {"inbound": {"http": {"200": 0.979}}, "outbound": {"http": {"200": 1.95}}},
				"status": {"status": "Healthy"}
			}
		}`
		out, err := TransformWorkloadDetailsResponse(payload)
		require.NoError(t, err)
		require.NotNil(t, out)

		assert.Equal(t, "productpage-v1", out.Workload.Name)
		assert.Equal(t, "bookinfo", out.Workload.Namespace)
		assert.Equal(t, "Deployment", out.Workload.Kind)
		assert.Equal(t, map[string]string{"app": "productpage", "version": "v1"}, out.Workload.Labels)
		assert.Equal(t, "bookinfo-productpage", out.Workload.ServiceAccount)
		assert.Equal(t, "2026-03-03T07:20:48Z", out.Workload.CreatedAt)

		assert.Equal(t, "Healthy", out.Status.Overall)
		assert.Equal(t, 1, out.Status.Replicas.Desired)
		assert.Equal(t, 1, out.Status.Replicas.Current)
		assert.Equal(t, 1, out.Status.Replicas.Available)
		assert.Equal(t, "97.9%", out.Status.TrafficSuccessRate.Inbound)
		assert.Equal(t, "100%", out.Status.TrafficSuccessRate.Outbound)

		assert.Equal(t, "Sidecar", out.Istio.Mode)
		assert.Equal(t, "1.28.0", out.Istio.ProxyVersion)
		assert.Equal(t, map[string]string{"CDS": "Synced", "EDS": "Synced", "LDS": "Synced", "RDS": "Synced"}, out.Istio.SyncStatus)
		assert.Equal(t, []string{"bookinfo", "bookinfo-gateway"}, out.Istio.Validations)

		require.Len(t, out.Pods, 1)
		assert.Equal(t, "productpage-v1-846b9898b9-tvdd7", out.Pods[0].Name)
		assert.Equal(t, "Running", out.Pods[0].Status)
		assert.Equal(t, []string{"productpage"}, out.Pods[0].Containers)
		assert.Equal(t, "Ready", out.Pods[0].IstioInit)
		assert.Equal(t, "Ready", out.Pods[0].IstioProxy)

		assert.Equal(t, []string{"productpage"}, out.AssociatedServices)
	})

	t.Run("workload without pods still transforms", func(t *testing.T) {
		payload := `{
			"name": "w1",
			"namespace": "ns1",
			"cluster": "",
			"gvk": {"Kind": "Deployment"},
			"createdAt": "",
			"labels": null,
			"serviceAccountNames": [],
			"desiredReplicas": 0,
			"currentReplicas": 0,
			"availableReplicas": 0,
			"pods": [],
			"services": [],
			"validations": null,
			"health": {"requests": {"inbound": null, "outbound": null}, "status": {"status": "Unknown"}},
			"istioSidecar": false,
			"isAmbient": false
		}`
		out, err := TransformWorkloadDetailsResponse(payload)
		require.NoError(t, err)
		assert.Equal(t, "w1", out.Workload.Name)
		assert.Equal(t, "ns1", out.Workload.Namespace)
		assert.Equal(t, "Deployment", out.Workload.Kind)
		assert.NotNil(t, out.Workload.Labels)
		assert.Equal(t, "None", out.Istio.Mode)
		assert.Empty(t, out.Istio.ProxyVersion)
		assert.Empty(t, out.Istio.SyncStatus)
		assert.Equal(t, "Unknown", out.Status.Overall)
		assert.Empty(t, out.Pods)
		assert.Empty(t, out.AssociatedServices)
	})

	t.Run("istio mode Ambient when isAmbient true", func(t *testing.T) {
		payload := `{
			"name": "w1",
			"namespace": "ns1",
			"gvk": {"Kind": "Deployment"},
			"labels": {},
			"pods": [],
			"services": [],
			"validations": null,
			"health": {"requests": {}, "status": {"status": ""}},
			"istioSidecar": false,
			"isAmbient": true
		}`
		out, err := TransformWorkloadDetailsResponse(payload)
		require.NoError(t, err)
		assert.Equal(t, "Ambient", out.Istio.Mode)
	})
}

func Test_extractImageTag(t *testing.T) {
	tests := []struct {
		image string
		want  string
	}{
		{"", ""},
		{"docker.io/istio/proxyv2:1.28.0", "1.28.0"},
		{"quay.io/example:latest", "latest"},
		{"no-tag", ""},
		{"with:multiple:colons", "colons"},
		{"sha256:abc123", "abc123"},
		{"registry.io/img:sha256:abc", "abc"},
	}
	for _, tt := range tests {
		got := extractImageTag(tt.image)
		assert.Equal(t, tt.want, got, "extractImageTag(%q)", tt.image)
	}
}

func Test_collectWorkloadValidationNames(t *testing.T) {
	t.Run("nil returns empty", func(t *testing.T) {
		got := collectWorkloadValidationNames(nil)
		assert.Empty(t, got)
	})
	t.Run("excludes workload category", func(t *testing.T) {
		validations := map[string]map[string]kialitypes.ValidationEntry{
			"workload":                             {"w1.ns1": {Name: "w1"}},
			"networking.istio.io/v1, Kind=Gateway": {"gw.ns": {Name: "my-gateway"}},
		}
		got := collectWorkloadValidationNames(validations)
		assert.Equal(t, []string{"my-gateway"}, got)
	})
	t.Run("collects and sorts names", func(t *testing.T) {
		validations := map[string]map[string]kialitypes.ValidationEntry{
			"networking.istio.io/v1, Kind=VirtualService": {"vs.ns": {Name: "bookinfo"}},
			"networking.istio.io/v1, Kind=Gateway":        {"gw.ns": {Name: "bookinfo-gateway"}},
		}
		got := collectWorkloadValidationNames(validations)
		assert.Equal(t, []string{"bookinfo", "bookinfo-gateway"}, got)
	})
}
