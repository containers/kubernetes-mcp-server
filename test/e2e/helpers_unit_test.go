//go:build e2e

package e2e

import (
	"testing"
)

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
