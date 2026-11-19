package discovery

import "testing"

func TestServiceDNSGatewayURL(t *testing.T) {
	tests := []struct {
		namespace, stack, mode, want string
	}{
		{
			namespace: "obs-mcp-loki",
			stack:     "obs-mcp-loki",
			mode:      "openshift-network",
			want:      "https://obs-mcp-loki-gateway-http.obs-mcp-loki.svc:8080/api/logs/v1",
		},
		{
			namespace: "monitoring",
			stack:     "lokistack",
			mode:      "passthrough",
			want:      "http://lokistack-gateway-http.monitoring.svc:8080",
		},
		{
			namespace: "logging",
			stack:     "logging-loki",
			mode:      "",
			want:      "http://logging-loki-gateway-http.logging.svc:8080",
		},
	}
	for _, tt := range tests {
		t.Run(tt.mode+"/"+tt.stack, func(t *testing.T) {
			got := ServiceDNSGatewayURL(tt.namespace, tt.stack, tt.mode)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
