package transforms

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	kialitypes "github.com/containers/kubernetes-mcp-server/pkg/kiali/types"
	"github.com/stretchr/testify/assert"
)

func TestTransformServicesListResponse_DataTest(t *testing.T) {
	inputData, err := os.ReadFile(filepath.Join("__datatests__", "services.json"))
	if err != nil {
		t.Fatalf("failed to read input file: %v", err)
	}

	expectedData, err := os.ReadFile(filepath.Join("__datatests__", "services_expected.json"))
	if err != nil {
		t.Fatalf("failed to read expected file: %v", err)
	}

	var expected kialitypes.ServicesByCluster
	if err := json.Unmarshal(expectedData, &expected); err != nil {
		t.Fatalf("failed to unmarshal expected data: %v", err)
	}

	actual, err := TransformServicesListResponse(string(inputData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assert.Equal(t, len(expected), len(actual))
	for cluster, expectedServices := range expected {
		assert.ElementsMatch(t, expectedServices, actual[cluster], "Cluster %s services mismatch", cluster)
	}
}

func TestTransformServicesListResponse(t *testing.T) {
	mockResponse := kialitypes.ServicesListResponse{
		Cluster: "test-cluster",
		Services: []kialitypes.ServiceListItem{
			{
				Name:      "test-svc",
				Namespace: "test-ns",
				Labels: map[string]string{
					"app": "test-app",
				},
				Health: kialitypes.ServiceListHealth{
					Status: kialitypes.ServiceHealthStatus{
						Status: "Healthy",
					},
				},
				IstioRefs: []kialitypes.IstioRef{
					{
						ObjectGVK: struct {
							Group string `json:"Group"`
							Kind  string `json:"Kind"`
						}{Kind: "VirtualService"},
						Name: "test-vs",
					},
				},
				AppLabel:     true,
				VersionLabel: false,
			},
		},
		Validations: kialitypes.ServicesValidations{
			Service: map[string]kialitypes.ServiceValidation{
				"test-svc.test-ns": {
					Valid: false,
					Checks: []kialitypes.Check{
						{Code: "KIA0001", Message: "Some error"},
					},
				},
			},
		},
	}

	payload, _ := json.Marshal(mockResponse)

	res, err := TransformServicesListResponse(string(payload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	clusterRes, ok := res["test-cluster"]
	if !ok {
		t.Fatalf("expected cluster 'test-cluster' in result")
	}

	if len(clusterRes) != 1 {
		t.Fatalf("expected 1 service, got %d", len(clusterRes))
	}

	svc := clusterRes[0]
	if svc.Name != "test-svc" {
		t.Errorf("expected name 'test-svc', got %s", svc.Name)
	}
	if svc.Health != "Healthy" {
		t.Errorf("expected health 'Healthy', got %s", svc.Health)
	}
	if svc.Configuration != "KIA0001(Some error)" {
		t.Errorf("expected configuration 'KIA0001(Some error)', got %s", svc.Configuration)
	}
	if svc.Labels != "app=test-app" {
		t.Errorf("expected labels 'app=test-app', got %s", svc.Labels)
	}
}
