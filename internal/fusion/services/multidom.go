package services

import (
	"context"
	"fmt"

	"github.com/containers/kubernetes-mcp-server/internal/fusion/clients"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GDPService provides Global Data Platform operations
type GDPService struct{}

func NewGDPService() *GDPService { return &GDPService{} }

func (s *GDPService) GetStatus(ctx context.Context, client *clients.ClusterClient) (*ComponentStatus, error) {
	status := &ComponentStatus{}
	
	// Check for IBM Spectrum Scale/GDP namespaces
	gdpNamespaces := []string{"ibm-spectrum-scale", "ibm-gdp"}
	for _, ns := range gdpNamespaces {
		if CheckNamespaceExists(ctx, client, ns) {
			status.Installed = true
			status.Ready = true
			status.Message = fmt.Sprintf("GDP found in namespace: %s", ns)
			return status, nil
		}
	}
	
	*status = NotInstalledStatus("GDP/Spectrum Scale not found")
	return status, nil
}

// DRService provides Disaster Recovery operations
type DRService struct{}

func NewDRService() *DRService { return &DRService{} }

func (s *DRService) GetStatus(ctx context.Context, client *clients.ClusterClient) (*ComponentStatus, error) {
	status := &ComponentStatus{}
	
	// Check for Metro DR or Regional DR CRDs
	drGVRs := []schema.GroupVersionResource{
		{Group: "ramendr.openshift.io", Version: "v1alpha1", Resource: "drpolicies"},
		{Group: "ramendr.openshift.io", Version: "v1alpha1", Resource: "drclusters"},
	}
	
	for _, gvr := range drGVRs {
		if CheckCRDExists(ctx, client, gvr) {
			status.Installed = true
			status.Ready = true
			status.Message = "DR CRDs found (Ramen DR)"
			return status, nil
		}
	}
	
	*status = NotInstalledStatus("DR components not found")
	return status, nil
}

// CatalogService provides Data Cataloging operations
type CatalogService struct{}

func NewCatalogService() *CatalogService { return &CatalogService{} }

func (s *CatalogService) GetStatus(ctx context.Context, client *clients.ClusterClient) (*ComponentStatus, error) {
	status := &ComponentStatus{}
	
	// Check for catalog namespaces
	catalogNamespaces := []string{"ibm-data-catalog", "openshift-data-catalog"}
	for _, ns := range catalogNamespaces {
		if CheckNamespaceExists(ctx, client, ns) {
			status.Installed = true
			status.Ready = true
			status.Message = fmt.Sprintf("Data Catalog found in namespace: %s", ns)
			return status, nil
		}
	}
	
	*status = NotInstalledStatus("Data Catalog not found")
	return status, nil
}

// CASService provides Content Aware Storage operations
type CASService struct{}

func NewCASService() *CASService { return &CASService{} }

func (s *CASService) GetStatus(ctx context.Context, client *clients.ClusterClient) (*ComponentStatus, error) {
	status := &ComponentStatus{}
	
	// Check for CAS namespace
	if CheckNamespaceExists(ctx, client, "ibm-cas") {
		status.Installed = true
		status.Ready = true
		status.Message = "CAS found in namespace: ibm-cas"
		return status, nil
	}
	
	*status = NotInstalledStatus("Content Aware Storage not found")
	return status, nil
}

// ServiceabilityService provides serviceability operations
type ServiceabilityService struct{}

func NewServiceabilityService() *ServiceabilityService { return &ServiceabilityService{} }

type ServiceabilitySummary struct {
	ComponentStatus
	MustGatherAvailable bool   `json:"mustGatherAvailable"`
	LoggingConfigured   bool   `json:"loggingConfigured"`
	Namespace           string `json:"namespace,omitempty"`
}

func (s *ServiceabilityService) GetSummary(ctx context.Context, client *clients.ClusterClient) (*ServiceabilitySummary, error) {
	summary := &ServiceabilitySummary{}
	
	// Check for must-gather tools
	if CheckNamespaceExists(ctx, client, "openshift-must-gather-operator") {
		summary.MustGatherAvailable = true
	}
	
	// Check for logging
	if CheckNamespaceExists(ctx, client, "openshift-logging") {
		summary.LoggingConfigured = true
		summary.Namespace = "openshift-logging"
	}
	
	summary.Installed = summary.MustGatherAvailable || summary.LoggingConfigured
	summary.Ready = summary.Installed
	summary.Message = "Serviceability components detected"
	
	if !summary.Installed {
		summary.ComponentStatus = NotInstalledStatus("No serviceability components found")
	}
	
	return summary, nil
}

// ObservabilityService provides observability operations
type ObservabilityService struct{}

func NewObservabilityService() *ObservabilityService { return &ObservabilityService{} }

type ObservabilitySummary struct {
	ComponentStatus
	PrometheusInstalled bool   `json:"prometheusInstalled"`
	GrafanaInstalled    bool   `json:"grafanaInstalled"`
	OtelInstalled       bool   `json:"otelInstalled"`
	Namespace           string `json:"namespace,omitempty"`
}

func (s *ObservabilityService) GetSummary(ctx context.Context, client *clients.ClusterClient) (*ObservabilitySummary, error) {
	summary := &ObservabilitySummary{}
	
	// Check for Prometheus
	if CheckNamespaceExists(ctx, client, "openshift-monitoring") {
		summary.PrometheusInstalled = true
		summary.Namespace = "openshift-monitoring"
	}
	
	// Check for Grafana
	if CheckNamespaceExists(ctx, client, "openshift-grafana") {
		summary.GrafanaInstalled = true
	}
	
	// Check for OpenTelemetry
	otelGVR := schema.GroupVersionResource{
		Group:    "opentelemetry.io",
		Version:  "v1alpha1",
		Resource: "opentelemetrycollectors",
	}
	if CheckCRDExists(ctx, client, otelGVR) {
		summary.OtelInstalled = true
	}
	
	summary.Installed = summary.PrometheusInstalled || summary.GrafanaInstalled || summary.OtelInstalled
	summary.Ready = summary.Installed
	summary.Message = "Observability stack detected"
	
	if !summary.Installed {
		summary.ComponentStatus = NotInstalledStatus("No observability components found")
	}
	
	return summary, nil
}

// VirtualizationService provides virtualization operations
type VirtualizationService struct{}

func NewVirtualizationService() *VirtualizationService { return &VirtualizationService{} }

type VirtualizationStatus struct {
	ComponentStatus
	KubeVirtInstalled bool   `json:"kubevirtInstalled"`
	VMCount           int    `json:"vmCount"`
	Namespace         string `json:"namespace,omitempty"`
}

func (s *VirtualizationService) GetStatus(ctx context.Context, client *clients.ClusterClient) (*VirtualizationStatus, error) {
	status := &VirtualizationStatus{}
	
	// Check for KubeVirt/OpenShift Virtualization
	virtNamespaces := []string{"openshift-cnv", "kubevirt"}
	for _, ns := range virtNamespaces {
		if CheckNamespaceExists(ctx, client, ns) {
			status.KubeVirtInstalled = true
			status.Namespace = ns
			break
		}
	}
	
	if !status.KubeVirtInstalled {
		status.ComponentStatus = NotInstalledStatus("KubeVirt/OpenShift Virtualization not found")
		return status, nil
	}
	
	status.Installed = true
	status.Ready = true
	
	// Check for VM CRD
	vmGVR := schema.GroupVersionResource{
		Group:    "kubevirt.io",
		Version:  "v1",
		Resource: "virtualmachines",
	}
	if CheckCRDExists(ctx, client, vmGVR) {
		status.Message = "KubeVirt installed with VM CRDs"
	} else {
		status.Message = "KubeVirt namespace found but CRDs not detected"
		status.Ready = false
	}
	
	return status, nil
}

// HCPService provides Hosted Control Planes operations
type HCPService struct{}

func NewHCPService() *HCPService { return &HCPService{} }

type HCPStatus struct {
	ComponentStatus
	HyperShiftInstalled bool   `json:"hypershiftInstalled"`
	HostedClusterCount  int    `json:"hostedClusterCount"`
	Namespace           string `json:"namespace,omitempty"`
}

func (s *HCPService) GetStatus(ctx context.Context, client *clients.ClusterClient) (*HCPStatus, error) {
	status := &HCPStatus{}
	
	// Check for HyperShift namespace
	if CheckNamespaceExists(ctx, client, "hypershift") {
		status.HyperShiftInstalled = true
		status.Namespace = "hypershift"
	}
	
	if !status.HyperShiftInstalled {
		status.ComponentStatus = NotInstalledStatus("HyperShift/HCP not found")
		return status, nil
	}
	
	status.Installed = true
	status.Ready = true
	
	// Check for HostedCluster CRD
	hcGVR := schema.GroupVersionResource{
		Group:    "hypershift.openshift.io",
		Version:  "v1beta1",
		Resource: "hostedclusters",
	}
	if CheckCRDExists(ctx, client, hcGVR) {
		status.Message = "HyperShift installed with HostedCluster CRDs"
	} else {
		status.Message = "HyperShift namespace found but CRDs not detected"
		status.Ready = false
	}
	
	return status, nil
}

// Made with Bob