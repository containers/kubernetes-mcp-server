package fusion

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/fusion/alltools"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/fusion/backup"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/fusion/datafoundation"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/fusion/storage"
)

// Toolset implements the IBM Fusion toolset
type Toolset struct{}

var _ api.Toolset = (*Toolset)(nil)

// GetName returns the name of the IBM Fusion toolset
func (t *Toolset) GetName() string {
	return "fusion"
}

// GetDescription returns a description of the IBM Fusion toolset
func (t *Toolset) GetDescription() string {
	return "IBM Fusion multi-cluster capabilities for OpenShift including Data Foundation, GDP, Backup, DR, Cataloging, CAS, Observability, Serviceability, Virtualization, and HCP"
}

// GetTools returns all tools provided by the IBM Fusion toolset
func (t *Toolset) GetTools(o api.Openshift) []api.ServerTool {
	return []api.ServerTool{
		// Storage
		storage.InitStorageSummary(),
		
		// Data Foundation
		datafoundation.InitStatusTool(),
		
		// Backup & Restore
		backup.InitJobsListTool(),
		
		// Global Data Platform
		alltools.InitGDPStatusTool(),
		
		// Disaster Recovery
		alltools.InitDRStatusTool(),
		
		// Data Cataloging
		alltools.InitCatalogStatusTool(),
		
		// Content Aware Storage
		alltools.InitCASStatusTool(),
		
		// Serviceability
		alltools.InitServiceabilitySummaryTool(),
		
		// Observability
		alltools.InitObservabilitySummaryTool(),
		
		// Virtualization
		alltools.InitVirtualizationStatusTool(),
		
		// Hosted Control Planes
		alltools.InitHCPStatusTool(),
	}
}

// GetPrompts returns prompts provided by the IBM Fusion toolset
func (t *Toolset) GetPrompts() []api.ServerPrompt {
	// No prompts for now
	return nil
}

// Made with Bob
