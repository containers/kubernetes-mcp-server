package fusion

import (
	"github.com/containers/kubernetes-mcp-server/internal/fusion/config"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"k8s.io/klog/v2"
)

// RegisterTools registers IBM Fusion tools if enabled via configuration
// This is the single integration point with the upstream toolsets registry
func RegisterTools() {
	cfg := config.LoadFromEnv()

	if !cfg.Enabled {
		klog.V(2).Info("IBM Fusion tools are disabled (FUSION_TOOLS_ENABLED not set to true)")
		return
	}

	klog.V(1).Info("Registering IBM Fusion toolset")
	toolsets.Register(&Toolset{})
}

func init() {
	// Hook into the upstream toolsets package
	// This replaces the no-op function with our registration logic
	toolsets.SetFusionRegistration(RegisterTools)
}

// Made with Bob
