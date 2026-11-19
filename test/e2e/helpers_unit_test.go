//go:build e2e

package e2e

import (
	"reflect"
	"testing"
)

func TestDeepMerge(t *testing.T) {
	tests := []struct {
		name string
		a, b map[string]any
		want map[string]any
	}{
		{
			name: "disjoint keys",
			a:    map[string]any{"a": 1},
			b:    map[string]any{"b": 2},
			want: map[string]any{"a": 1, "b": 2},
		},
		{
			name: "scalar overwrite",
			a:    map[string]any{"k": "old"},
			b:    map[string]any{"k": "new"},
			want: map[string]any{"k": "new"},
		},
		{
			name: "nested map merge",
			a:    map[string]any{"m": map[string]any{"a": 1}},
			b:    map[string]any{"m": map[string]any{"b": 2}},
			want: map[string]any{"m": map[string]any{"a": 1, "b": 2}},
		},
		{
			name: "slice of maps concatenation",
			a:    map[string]any{"s": []map[string]any{{"n": "x"}}},
			b:    map[string]any{"s": []map[string]any{{"n": "y"}}},
			want: map[string]any{"s": []map[string]any{{"n": "x"}, {"n": "y"}}},
		},
		{
			name: "volume values merge",
			a:    keycloakCAVolumeValues(),
			b:    stsAssertionVolumeValues(),
			want: map[string]any{
				"extraVolumes": []map[string]any{
					{"name": caSecretName, "secret": map[string]any{"secretName": caSecretName}},
					{"name": stsAssertionSecretName, "secret": map[string]any{"secretName": stsAssertionSecretName}},
				},
				"extraVolumeMounts": []map[string]any{
					{"name": caSecretName, "mountPath": caMountPath, "readOnly": true},
					{"name": stsAssertionSecretName, "mountPath": stsAssertionMountPath, "readOnly": true},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeValues(tt.a, tt.b)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeValues() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestImageSpec(t *testing.T) {
	tests := []struct {
		image                          string
		wantRegistry, wantRepo, wantVs string
	}{
		{
			image:        "localhost/kubernetes-mcp-server:e2e",
			wantRegistry: "localhost",
			wantRepo:     "kubernetes-mcp-server",
			wantVs:       "e2e",
		},
		{
			image:        "ghcr.io/org/sub/image:v1.0",
			wantRegistry: "ghcr.io",
			wantRepo:     "org/sub/image",
			wantVs:       "v1.0",
		},
		{
			image:        "registry.example.com:5000/myapp:v2",
			wantRegistry: "registry.example.com:5000",
			wantRepo:     "myapp",
			wantVs:       "v2",
		},
		{
			image:        "myrepo/myimage",
			wantRegistry: "docker.io",
			wantRepo:     "myrepo/myimage",
			wantVs:       "latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			spec := imageSpec(tt.image)
			reg, repo, vs := spec["registry"], spec["repository"], spec["version"]
			if reg != tt.wantRegistry || repo != tt.wantRepo || vs != tt.wantVs {
				t.Errorf("imageSpec(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tt.image, reg, repo, vs, tt.wantRegistry, tt.wantRepo, tt.wantVs)
			}
			if spec["pullPolicy"] != "IfNotPresent" {
				t.Errorf("imageSpec(%q) pullPolicy = %q, want IfNotPresent", tt.image, spec["pullPolicy"])
			}
		})
	}
}
