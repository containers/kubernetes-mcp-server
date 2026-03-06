package transforms

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	kialitypes "github.com/containers/kubernetes-mcp-server/pkg/kiali/types"
	"github.com/stretchr/testify/assert"
)

func TestTransformWorkloadDetailsResponse(t *testing.T) {
	mockResponse := kialitypes.WorkloadDetailsRaw{
		Name:      "test-wl",
		Namespace: "test-ns",
		GVK: kialitypes.WorkloadDetailsGVK{
			Kind: "Deployment",
		},
		ServiceAccountNames: []string{"default"},
		DesiredReplicas:     2,
		CurrentReplicas:     2,
		AvailableReplicas:   1,
		Health: kialitypes.WorkloadDetailsHealthRaw{
			Status: struct {
				Status string `json:"status"`
			}{Status: "Degraded"},
		},
		IstioSidecar: true,
		Pods: []kialitypes.WorkloadDetailsPodRaw{
			{
				Name:   "test-wl-pod-1",
				Status: "Running",
				Containers: []kialitypes.WorkloadDetailsContainerRaw{
					{Name: "app", IsProxy: false},
					{Name: "istio-proxy", IsProxy: true, Image: "docker.io/istio/proxyv2:1.18.0", IsReady: true},
				},
				IstioInitContainers: []kialitypes.WorkloadDetailsContainerRaw{
					{Name: "istio-init", IsProxy: true, IsReady: true},
				},
			},
		},
		Services: []kialitypes.WorkloadDetailsServiceRef{
			{Name: "test-svc"},
		},
	}

	mockResponse.Health.Requests.Inbound = map[string]map[string]float64{
		"http": {"200": 0.99},
	}
	mockResponse.Health.Requests.Outbound = map[string]map[string]float64{
		"http": {"200": 1.05}, // Testing > 1.0 logic
	}

	payload, _ := json.Marshal(mockResponse)

	res, err := TransformWorkloadDetailsResponse(string(payload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Workload.Name != "test-wl" {
		t.Errorf("expected workload name 'test-wl', got %s", res.Workload.Name)
	}
	if res.Workload.Kind != "Deployment" {
		t.Errorf("expected kind 'Deployment', got %s", res.Workload.Kind)
	}
	if res.Workload.ServiceAccount != "default" {
		t.Errorf("expected service account 'default', got %s", res.Workload.ServiceAccount)
	}

	if res.Status.Overall != "Degraded" {
		t.Errorf("expected status 'Degraded', got %s", res.Status.Overall)
	}
	if res.Status.Replicas.Desired != 2 || res.Status.Replicas.Available != 1 {
		t.Errorf("expected desired=2, available=1, got %+v", res.Status.Replicas)
	}
	if res.Status.TrafficSuccessRate.Inbound != "99.0%" {
		t.Errorf("expected inbound rate '99.0%%', got %s", res.Status.TrafficSuccessRate.Inbound)
	}
	if res.Status.TrafficSuccessRate.Outbound != "100%" {
		t.Errorf("expected outbound rate '100%%', got %s", res.Status.TrafficSuccessRate.Outbound)
	}

	if res.Istio.Mode != "Sidecar" {
		t.Errorf("expected istio mode 'Sidecar', got %s", res.Istio.Mode)
	}
	if res.Istio.ProxyVersion != "1.18.0" {
		t.Errorf("expected proxy version '1.18.0', got %s", res.Istio.ProxyVersion)
	}

	if len(res.Pods) != 1 {
		t.Fatalf("expected 1 pod")
	}
	pod := res.Pods[0]
	if pod.Name != "test-wl-pod-1" {
		t.Errorf("expected pod name 'test-wl-pod-1', got %s", pod.Name)
	}
	if len(pod.Containers) != 1 || pod.Containers[0] != "app" {
		t.Errorf("expected 1 container 'app', got %+v", pod.Containers)
	}
	if pod.IstioInit != "Ready" {
		t.Errorf("expected IstioInit 'Ready', got %s", pod.IstioInit)
	}
	if pod.IstioProxy != "Ready" {
		t.Errorf("expected IstioProxy 'Ready', got %s", pod.IstioProxy)
	}

	if len(res.AssociatedServices) != 1 || res.AssociatedServices[0] != "test-svc" {
		t.Errorf("expected associated service 'test-svc', got %+v", res.AssociatedServices)
	}
}

func TestExtractImageTag(t *testing.T) {
	tests := []struct {
		image    string
		expected string
	}{
		{"", ""},
		{"docker.io/istio/proxyv2", ""},
		{"docker.io/istio/proxyv2:1.20.0", "1.20.0"},
		{"docker.io/istio/proxyv2:sha256:abcd", "abcd"},
	}

	for _, tt := range tests {
		actual := extractImageTag(tt.image)
		if actual != tt.expected {
			t.Errorf("extractImageTag(%q) = %q, expected %q", tt.image, actual, tt.expected)
		}
	}
}

func TestTransformWorkloadDetailsResponse_DataTest(t *testing.T) {
	inputData, err := os.ReadFile(filepath.Join("__datatests__", "workloadDetail.json"))
	if err != nil {
		t.Fatalf("failed to read input file: %v", err)
	}

	expectedData, err := os.ReadFile(filepath.Join("__datatests__", "workloadDetail_expected.json"))
	if err != nil {
		t.Fatalf("failed to read expected file: %v", err)
	}

	var expected kialitypes.WorkloadDetailsFormatted
	if err := json.Unmarshal(expectedData, &expected); err != nil {
		t.Fatalf("failed to unmarshal expected data: %v", err)
	}

	actual, err := TransformWorkloadDetailsResponse(string(inputData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assert.Equal(t, expected, *actual)
}
