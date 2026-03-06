package transforms

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	kialitypes "github.com/containers/kubernetes-mcp-server/pkg/kiali/types"
	"github.com/stretchr/testify/assert"
)

func TestTransformWorkloadsListResponse(t *testing.T) {
	mockResponse := kialitypes.WorkloadsListResponse{
		Cluster: "my-cluster",
		Workloads: []kialitypes.WorkloadListItem{
			{
				Name:      "test-wl",
				Namespace: "test-ns",
				GVK: kialitypes.WorkloadGVK{
					Kind: "Deployment",
				},
				Health: kialitypes.WorkloadListHealth{
					Status: kialitypes.WorkloadHealthStatus{
						Status: "Degraded",
					},
				},
				Labels: map[string]string{
					"app":     "test",
					"version": "v1",
				},
				AppLabel:     true,
				VersionLabel: true,
				IstioRefs: []kialitypes.IstioRef{
					{
						ObjectGVK: struct {
							Group string `json:"Group"`
							Kind  string `json:"Kind"`
						}{Kind: "PeerAuthentication"},
						Name: "default",
					},
				},
			},
			{
				Name:      "test-wl-missing-labels",
				Namespace: "test-ns",
				GVK: kialitypes.WorkloadGVK{
					Kind: "DaemonSet",
				},
				AppLabel:     false,
				VersionLabel: false,
			},
		},
	}

	payload, _ := json.Marshal(mockResponse)

	res, err := TransformWorkloadsListResponse(string(payload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	clusterRes, ok := res["my-cluster"]
	if !ok {
		t.Fatalf("expected cluster 'my-cluster' in result")
	}

	if len(clusterRes) != 2 {
		t.Fatalf("expected 2 workloads, got %d", len(clusterRes))
	}

	wl1 := clusterRes[0]
	if wl1.Name != "test-wl" {
		t.Errorf("expected name 'test-wl', got %s", wl1.Name)
	}
	if wl1.Type != "Deployment" {
		t.Errorf("expected type 'Deployment', got %s", wl1.Type)
	}
	if wl1.Health != "Degraded" {
		t.Errorf("expected health 'Degraded', got %s", wl1.Health)
	}
	if wl1.Labels != "app=test, version=v1" {
		t.Errorf("expected labels 'app=test, version=v1', got %s", wl1.Labels)
	}
	if wl1.Details != "default(PA)" {
		t.Errorf("expected details 'default(PA)', got %s", wl1.Details)
	}

	wl2 := clusterRes[1]
	expectedMissingDetails := "Missing App and Version label (This workload won't be linked with an application. The label is recommended as it affects telemetry. Missing labels may impact telemetry reported by the Istio proxy.)"
	if wl2.Details != expectedMissingDetails {
		t.Errorf("expected missing labels details, got %s", wl2.Details)
	}
}

func TestTransformWorkloadsListResponseDefaultCluster(t *testing.T) {
	mockResponse := kialitypes.WorkloadsListResponse{
		Workloads: []kialitypes.WorkloadListItem{
			{
				Name: "test-wl",
			},
		},
	}

	payload, _ := json.Marshal(mockResponse)

	res, err := TransformWorkloadsListResponse(string(payload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := res["default"]; !ok {
		t.Fatalf("expected 'default' cluster for empty cluster name")
	}
}

func TestTransformWorkloadsListResponse_DataTest(t *testing.T) {
	inputData, err := os.ReadFile(filepath.Join("__datatests__", "workloads.json"))
	if err != nil {
		t.Fatalf("failed to read input file: %v", err)
	}

	expectedData, err := os.ReadFile(filepath.Join("__datatests__", "workloads_expected.json"))
	if err != nil {
		t.Fatalf("failed to read expected file: %v", err)
	}

	var expected kialitypes.WorkloadsByCluster
	if err := json.Unmarshal(expectedData, &expected); err != nil {
		t.Fatalf("failed to unmarshal expected data: %v", err)
	}

	actual, err := TransformWorkloadsListResponse(string(inputData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assert.Equal(t, expected, actual)
}
