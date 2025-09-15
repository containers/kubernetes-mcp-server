package kubernetes

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

// isAllowed checks the resource is in denied list or not.
// If it is in denied list, this function returns false.
func isAllowed(
	staticConfig *config.StaticConfig, // TODO: maybe just use the denied resource slice
	gvk *schema.GroupVersionKind,
) bool {
	if staticConfig == nil {
		return true
	}

	for _, val := range staticConfig.DeniedResources {
		// If kind is empty, that means Group/Version pair is denied entirely
		if val.Kind == "" {
			if gvk.Group == val.Group && gvk.Version == val.Version {
				return false
			}
		}
		if gvk.Group == val.Group &&
			gvk.Version == val.Version &&
			gvk.Kind == val.Kind {
			return false
		}
	}

	return true
}

func isNotAllowedError(gvk *schema.GroupVersionKind) error {
	return fmt.Errorf("resource not allowed: %s", gvk.String())
}
