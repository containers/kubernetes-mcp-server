package config

import "testing"

func TestIsDeniedResource(t *testing.T) {
	tests := []struct {
		name       string
		apiVersion string
		kind       string
		denied     []GroupVersionKind
		wantDenied bool
	}{
		{
			name:       "no denials",
			apiVersion: "v1",
			kind:       "Pod",
		},
		{
			name:       "exact kind",
			apiVersion: "v1",
			kind:       "Secret",
			denied:     []GroupVersionKind{{Version: "v1", Kind: "Secret"}},
			wantDenied: true,
		},
		{
			name:       "group version wildcard",
			apiVersion: "apps/v1",
			kind:       "Deployment",
			denied:     []GroupVersionKind{{Group: "apps", Version: "v1"}},
			wantDenied: true,
		},
		{
			name:       "different kind",
			apiVersion: "v1",
			kind:       "ConfigMap",
			denied:     []GroupVersionKind{{Version: "v1", Kind: "Secret"}},
		},
		{
			name:       "different api version",
			apiVersion: "apps/v1",
			kind:       "Deployment",
			denied:     []GroupVersionKind{{Group: "apps", Version: "v1beta1", Kind: "Deployment"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDeniedResource(tt.apiVersion, tt.kind, tt.denied); got != tt.wantDenied {
				t.Fatalf("IsDeniedResource() = %v, want %v", got, tt.wantDenied)
			}
		})
	}
}
