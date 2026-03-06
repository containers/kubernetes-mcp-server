package transforms

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	kialitypes "github.com/containers/kubernetes-mcp-server/pkg/kiali/types"
	"github.com/stretchr/testify/assert"
)

func TestTransformServiceDetailsResponse(t *testing.T) {
	mockResponse := kialitypes.ServiceDetailsRaw{
		Service: kialitypes.ServiceDetailsServiceRaw{
			Name:      "test-svc",
			Namespace: "test-ns",
			Type:      "ClusterIP",
			IP:        "1.2.3.4",
			Ports: []kialitypes.ServiceDetailsPortRaw{
				{Name: "http", Port: 8080, Protocol: "TCP"},
			},
			Selectors: map[string]string{"app": "test"},
		},
		Health: kialitypes.ServiceDetailsHealthRaw{
			Status: struct {
				Status string `json:"status"`
			}{Status: "Degraded"},
		},
		IsAmbient:    true,
		IstioSidecar: false,
		Validations: map[string]map[string]kialitypes.ValidationEntry{
			"virtualservice": {
				"test-vs": {Name: "test-vs"},
			},
		},
		VirtualServices: []kialitypes.VirtualServiceRef{
			{Name: "test-vs"},
		},
	}

	// Mocking health requests separately due to complex nested map
	mockResponse.Health.Requests.Inbound = map[string]map[string]float64{
		"http": {
			"200": 0.95,
		},
	}

	payload, _ := json.Marshal(mockResponse)

	res, err := TransformServiceDetailsResponse(string(payload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Service.Name != "test-svc" {
		t.Errorf("expected service name 'test-svc', got %s", res.Service.Name)
	}
	if res.HealthStatus != "Degraded" {
		t.Errorf("expected health 'Degraded', got %s", res.HealthStatus)
	}
	if res.InboundSuccessRate2xx != "95.0%" {
		t.Errorf("expected inbound rate '95.0%%', got %s", res.InboundSuccessRate2xx)
	}
	if !res.IstioConfig.IsAmbient {
		t.Errorf("expected IsAmbient to be true")
	}
	if len(res.IstioConfig.VirtualServices) != 1 || res.IstioConfig.VirtualServices[0] != "test-vs" {
		t.Errorf("expected VirtualServices to contain 'test-vs'")
	}
	if len(res.Service.Ports) != 1 || res.Service.Ports[0].Port != 8080 {
		t.Errorf("expected port 8080")
	}
}

func TestTransformServiceDetailsResponse_DataTest(t *testing.T) {
	inputData, err := os.ReadFile(filepath.Join("__datatests__", "serviceDetail.json"))
	if err != nil {
		t.Fatalf("failed to read input file: %v", err)
	}

	expectedData, err := os.ReadFile(filepath.Join("__datatests__", "serviceDetail_expected.json"))
	if err != nil {
		t.Fatalf("failed to read expected file: %v", err)
	}

	var expected kialitypes.ServiceDetailsFormatted
	if err := json.Unmarshal(expectedData, &expected); err != nil {
		t.Fatalf("failed to unmarshal expected data: %v", err)
	}

	actual, err := TransformServiceDetailsResponse(string(inputData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assert.Equal(t, expected, *actual)
}
