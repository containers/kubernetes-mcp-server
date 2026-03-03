package transforms

import (
	"encoding/json"
	"testing"

	kialitypes "github.com/containers/kubernetes-mcp-server/pkg/kiali/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformServicesListResponse(t *testing.T) {
	t.Run("empty payload returns error", func(t *testing.T) {
		_, err := TransformServicesListResponse("")
		require.Error(t, err)
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		_, err := TransformServicesListResponse(`{invalid`)
		require.Error(t, err)
	})

	t.Run("empty cluster uses default", func(t *testing.T) {
		payload := `{"cluster":"","services":[],"validations":null}`
		out, err := TransformServicesListResponse(payload)
		require.NoError(t, err)
		require.NotNil(t, out)
		assert.Contains(t, out, "default")
		assert.Len(t, out["default"], 0)
	})

	t.Run("transforms service with istio refs and labels", func(t *testing.T) {
		payload := `{
			"cluster": "Kubernetes",
			"services": [
				{
					"name": "productpage",
					"namespace": "bookinfo",
					"labels": {"app": "productpage", "service": "productpage"},
					"health": {"status": {"status": "Healthy"}},
					"istioReferences": [
						{"objectGVK": {"Group": "networking.istio.io", "Kind": "VirtualService"}, "name": "bookinfo", "namespace": "bookinfo", "cluster": ""},
						{"objectGVK": {"Group": "networking.istio.io", "Kind": "Gateway"}, "name": "bookinfo-gateway", "namespace": "bookinfo", "cluster": ""}
					],
					"appLabel": true,
					"versionLabel": true
				}
			],
			"validations": {"service": {"productpage.bookinfo": {"valid": true, "checks": []}}}
		}`
		out, err := TransformServicesListResponse(payload)
		require.NoError(t, err)
		require.Len(t, out["Kubernetes"], 1)
		svc := out["Kubernetes"][0]
		assert.Equal(t, "productpage", svc.Name)
		assert.Equal(t, "bookinfo", svc.Namespace)
		assert.Equal(t, "Healthy", svc.Health)
		assert.Equal(t, "True", svc.Configuration)
		assert.Contains(t, svc.Details, "bookinfo(VS)")
		assert.Contains(t, svc.Details, "bookinfo-gateway(GW)")
		assert.Contains(t, svc.Labels, "app=productpage")
	})

	t.Run("invalid validation adds Code(message) to configuration", func(t *testing.T) {
		payload := `{
			"cluster": "c1",
			"services": [{"name": "s1", "namespace": "ns1", "labels": {}, "health": {"status": {"status": ""}}, "istioReferences": [], "appLabel": true, "versionLabel": true}],
			"validations": {"service": {"s1.ns1": {"valid": false, "checks": [{"code": "KIA0601", "message": "Check failed"}]}}}
		}`
		out, err := TransformServicesListResponse(payload)
		require.NoError(t, err)
		require.Len(t, out["c1"], 1)
		assert.Equal(t, "KIA0601(Check failed)", out["c1"][0].Configuration)
	})

	t.Run("missing app and version label adds details", func(t *testing.T) {
		payload := `{
			"cluster": "c1",
			"services": [{"name": "s1", "namespace": "ns1", "labels": {}, "health": {"status": {"status": "Healthy"}}, "istioReferences": [], "appLabel": false, "versionLabel": false}],
			"validations": null
		}`
		out, err := TransformServicesListResponse(payload)
		require.NoError(t, err)
		require.Len(t, out["c1"], 1)
		assert.Contains(t, out["c1"][0].Details, "Missing App and Version label")
	})
}

func TestTransformServiceDetailsResponse(t *testing.T) {
	t.Run("empty payload returns error", func(t *testing.T) {
		_, err := TransformServiceDetailsResponse("")
		require.Error(t, err)
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		_, err := TransformServiceDetailsResponse(`{invalid`)
		require.Error(t, err)
	})

	t.Run("transforms full service details", func(t *testing.T) {
		payload := `{
			"service": {
				"name": "details",
				"namespace": "bookinfo",
				"type": "ClusterIP",
				"ip": "10.96.1.252",
				"ports": [{"name": "http", "port": 9080, "protocol": "TCP"}],
				"selectors": {"app": "details"}
			},
			"endpoints": [{"addresses": [{"kind": "Pod", "name": "details-v1-abc", "ip": "10.244.0.12"}]}],
			"workloads": [{"name": "details-v1", "namespace": "bookinfo", "gvk": {"Kind": "Deployment"}, "labels": {"app": "details", "version": "v1"}, "serviceAccountNames": ["bookinfo-details"], "podCount": 1}],
			"health": {"requests": {"inbound": {"http": {"200": 0.977}}}, "status": {"status": "Healthy"}},
			"isAmbient": false,
			"istioSidecar": true,
			"namespaceMTLS": {"autoMTLSEnabled": true, "status": "MTLS_NOT_ENABLED"},
			"virtualServices": [],
			"destinationRules": [],
			"validations": {
				"networking.istio.io/v1, Kind=Gateway": {"gw.bookinfo": {"name": "bookinfo-gateway"}},
				"networking.istio.io/v1, Kind=VirtualService": {"vs.bookinfo": {"name": "bookinfo"}}
			}
		}`
		out, err := TransformServiceDetailsResponse(payload)
		require.NoError(t, err)
		require.NotNil(t, out)
		assert.Equal(t, "details", out.Service.Name)
		assert.Equal(t, "bookinfo", out.Service.Namespace)
		assert.Equal(t, "ClusterIP", out.Service.Type)
		assert.Equal(t, "10.96.1.252", out.Service.IP)
		require.Len(t, out.Service.Ports, 1)
		assert.Equal(t, "http", out.Service.Ports[0].Name)
		assert.Equal(t, 9080, out.Service.Ports[0].Port)
		assert.Equal(t, "TCP", out.Service.Ports[0].Protocol)
		assert.Equal(t, map[string]string{"app": "details"}, out.Service.Selectors)

		assert.False(t, out.IstioConfig.IsAmbient)
		assert.True(t, out.IstioConfig.HasSidecar)
		assert.Equal(t, "AUTO_ENABLED", out.IstioConfig.MTLSMode)
		assert.Equal(t, []string{"bookinfo", "bookinfo-gateway"}, out.IstioConfig.Validations)

		require.Len(t, out.Workloads, 1)
		assert.Equal(t, "details-v1", out.Workloads[0].Name)
		assert.Equal(t, "Deployment", out.Workloads[0].Kind)
		assert.Equal(t, "bookinfo-details", out.Workloads[0].ServiceAccount)
		assert.Equal(t, 1, out.Workloads[0].PodCount)

		assert.Equal(t, "Healthy", out.HealthStatus)
		assert.Equal(t, "97.7%", out.InboundSuccessRate2xx)

		require.Len(t, out.Endpoints, 1)
		assert.Equal(t, "details-v1-abc", out.Endpoints[0].PodName)
		assert.Equal(t, "10.244.0.12", out.Endpoints[0].IP)
	})
}

func Test_formatDetails(t *testing.T) {
	ref := func(kind, name string) kialitypes.IstioRef {
		r := kialitypes.IstioRef{}
		r.ObjectGVK.Kind = kind
		r.Name = name
		return r
	}
	tests := []struct {
		name string
		refs []kialitypes.IstioRef
		want string
	}{
		{"empty", nil, ""},
		{"single", []kialitypes.IstioRef{ref("Gateway", "gw1")}, "gw1(GW)"},
		{"multiple sorted", []kialitypes.IstioRef{ref("VirtualService", "vs1"), ref("Gateway", "gw1")}, "gw1(GW), vs1(VS)"},
		{"unknown kind uses kind name", []kialitypes.IstioRef{ref("SomeCRD", "crd1")}, "crd1(SomeCRD)"},
		{"empty name becomes <no name>", []kialitypes.IstioRef{ref("Gateway", "")}, "<no name>(GW)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDetails(tt.refs)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_formatMissingLabelDetails(t *testing.T) {
	tests := []struct {
		appLabel     bool
		versionLabel bool
		contains     string
		empty        bool
	}{
		{true, true, "", true},
		{false, true, "Missing App label", false},
		{true, false, "Missing Version label", false},
		{false, false, "Missing App and Version label", false},
	}
	for _, tt := range tests {
		got := formatMissingLabelDetails(tt.appLabel, tt.versionLabel)
		if tt.empty {
			assert.Empty(t, got)
		} else {
			assert.Contains(t, got, tt.contains)
		}
	}
}

func Test_formatMissingSidecarDetail(t *testing.T) {
	assert.Equal(t, "", formatMissingSidecarDetail(true))
	got := formatMissingSidecarDetail(false)
	assert.Contains(t, got, "Istio sidecar container not found")
	assert.Contains(t, got, "istio-injection")
}

func Test_joinDetailParts(t *testing.T) {
	assert.Equal(t, "", joinDetailParts("", ""))
	assert.Equal(t, "a", joinDetailParts("a", ""))
	assert.Equal(t, "a, b", joinDetailParts("a", "b"))
	assert.Equal(t, "a, b", joinDetailParts("a", "", "b"))
}

func Test_formatLabels(t *testing.T) {
	assert.Equal(t, "None", formatLabels(nil))
	assert.Equal(t, "None", formatLabels(map[string]string{}))
	assert.Equal(t, "app=productpage", formatLabels(map[string]string{"app": "productpage"}))
	// Keys are sorted
	out := formatLabels(map[string]string{"z": "1", "a": "2"})
	assert.Equal(t, "a=2, z=1", out)
}

func Test_formatPercent(t *testing.T) {
	assert.Equal(t, "0%", formatPercent(0))
	assert.Equal(t, "0%", formatPercent(-0.1))
	assert.Equal(t, "100%", formatPercent(1))
	assert.Equal(t, "100%", formatPercent(1.5))
	assert.Equal(t, "97.7%", formatPercent(0.977))
	assert.Equal(t, "50.0%", formatPercent(0.5))
}

func TestTransformServicesListResponse_roundtrip(t *testing.T) {
	// Build minimal valid response and ensure transform then marshal is consistent
	resp := kialitypes.ServicesListResponse{
		Cluster: "Kubernetes",
		Services: []kialitypes.ServiceListItem{
			{
				Name:         "svc1",
				Namespace:    "ns1",
				Labels:       map[string]string{"app": "svc1"},
				Health:       kialitypes.ServiceListHealth{Status: kialitypes.ServiceHealthStatus{Status: "Healthy"}},
				IstioRefs:    []kialitypes.IstioRef{},
				AppLabel:     true,
				VersionLabel: true,
				IstioSidecar: true,
			},
		},
		Validations: kialitypes.ServicesValidations{},
	}
	payload, err := json.Marshal(resp)
	require.NoError(t, err)
	out, err := TransformServicesListResponse(string(payload))
	require.NoError(t, err)
	require.Len(t, out["Kubernetes"], 1)
	assert.Equal(t, "svc1", out["Kubernetes"][0].Name)
	assert.Equal(t, "ns1", out["Kubernetes"][0].Namespace)
}

func TestTransformServicesListResponse_istioSidecarFalse_addsSidecarDetail(t *testing.T) {
	resp := kialitypes.ServicesListResponse{
		Cluster: "Kubernetes",
		Services: []kialitypes.ServiceListItem{
			{
				Name:         "details",
				Namespace:    "bookinfo",
				Labels:       map[string]string{"app": "details"},
				Health:       kialitypes.ServiceListHealth{Status: kialitypes.ServiceHealthStatus{Status: "Degraded"}},
				IstioRefs:    []kialitypes.IstioRef{},
				AppLabel:     true,
				VersionLabel: true,
				IstioSidecar: false,
			},
		},
		Validations: kialitypes.ServicesValidations{},
	}
	payload, err := json.Marshal(resp)
	require.NoError(t, err)
	out, err := TransformServicesListResponse(string(payload))
	require.NoError(t, err)
	require.Len(t, out["Kubernetes"], 1)
	assert.Contains(t, out["Kubernetes"][0].Details, "Istio sidecar container not found")
	assert.Contains(t, out["Kubernetes"][0].Details, "istio-injection")
}
