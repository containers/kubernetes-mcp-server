package kiali

// ServiceDetailsRaw is the raw response from Kiali service details API (for unmarshaling).
type ServiceDetailsRaw struct {
	Service          ServiceDetailsServiceRaw              `json:"service"`
	Endpoints        []ServiceDetailsEndpointRaw           `json:"endpoints"`
	Workloads        []ServiceDetailsWorkloadRaw           `json:"workloads"`
	Health           ServiceDetailsHealthRaw               `json:"health"`
	IsAmbient        bool                                  `json:"isAmbient"`
	IstioSidecar     bool                                  `json:"istioSidecar"`
	NamespaceMTLS    ServiceDetailsNamespaceMTLS           `json:"namespaceMTLS"`
	VirtualServices  []VirtualServiceRef                   `json:"virtualServices"`
	DestinationRules []DestinationRuleRef                  `json:"destinationRules"`
	Validations      map[string]map[string]ValidationEntry `json:"validations"`
}

// ServiceDetailsServiceRaw is the service object in the details response.
type ServiceDetailsServiceRaw struct {
	Name      string                  `json:"name"`
	Namespace string                  `json:"namespace"`
	Type      string                  `json:"type"`
	IP        string                  `json:"ip"`
	Ports     []ServiceDetailsPortRaw `json:"ports"`
	Selectors map[string]string       `json:"selectors"`
}

// ServiceDetailsPortRaw is a port entry in the service.
type ServiceDetailsPortRaw struct {
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

// ServiceDetailsEndpointRaw is an endpoint (pod) in the details response.
type ServiceDetailsEndpointRaw struct {
	Addresses []ServiceDetailsAddressRaw `json:"addresses"`
}

// ServiceDetailsAddressRaw is one address (pod) in an endpoint.
type ServiceDetailsAddressRaw struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	IP   string `json:"ip"`
}

// ServiceDetailsWorkloadGVK is GVK for a workload in service details.
type ServiceDetailsWorkloadGVK struct {
	Kind string `json:"Kind"`
}

// ServiceDetailsWorkloadRaw is a workload in the service details.
type ServiceDetailsWorkloadRaw struct {
	Name                string                    `json:"name"`
	Namespace           string                    `json:"namespace"`
	GVK                 ServiceDetailsWorkloadGVK `json:"gvk"`
	Labels              map[string]string         `json:"labels"`
	ServiceAccountNames []string                  `json:"serviceAccountNames"`
	PodCount            int                       `json:"podCount"`
}

// ServiceDetailsHealthRaw is health in the service details.
type ServiceDetailsHealthRaw struct {
	Requests struct {
		Inbound map[string]map[string]float64 `json:"inbound"`
	} `json:"requests"`
	Status struct {
		Status string `json:"status"`
	} `json:"status"`
}

// ServiceDetailsNamespaceMTLS is mTLS info for the namespace.
type ServiceDetailsNamespaceMTLS struct {
	AutoMTLSEnabled bool   `json:"autoMTLSEnabled"`
	Status          string `json:"status"`
}

// VirtualServiceRef and DestinationRuleRef are references with at least a name (for name extraction).
type VirtualServiceRef struct {
	Name string `json:"name"`
}

// DestinationRuleRef is a destination rule reference.
type DestinationRuleRef struct {
	Name string `json:"name"`
}

// ValidationEntry has the name of a validated object.
type ValidationEntry struct {
	Name string `json:"name"`
}

// --- Formatted output types ---

// ServiceDetailsFormatted is the transformed service details returned to callers.
type ServiceDetailsFormatted struct {
	Service               ServiceDetailsServiceFormatted    `json:"service"`
	IstioConfig           ServiceDetailsIstioConfig         `json:"istio_config"`
	Workloads             []ServiceDetailsWorkloadFormatted `json:"workloads"`
	HealthStatus          string                            `json:"health_status"`
	InboundSuccessRate2xx string                            `json:"inbound_success_rate_2xx"`
	Endpoints             []ServiceDetailsEndpointFormatted `json:"endpoints"`
}

// ServiceDetailsServiceFormatted is the service section of the formatted output.
type ServiceDetailsServiceFormatted struct {
	Name      string                        `json:"name"`
	Namespace string                        `json:"namespace"`
	Type      string                        `json:"type"`
	IP        string                        `json:"ip"`
	Ports     []ServiceDetailsPortFormatted `json:"ports"`
	Selectors map[string]string             `json:"selectors"`
}

// ServiceDetailsPortFormatted is a port in the formatted output.
type ServiceDetailsPortFormatted struct {
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

// ServiceDetailsIstioConfig is the istio_config section.
type ServiceDetailsIstioConfig struct {
	IsAmbient        bool     `json:"is_ambient"`
	HasSidecar       bool     `json:"has_sidecar"`
	MTLSMode         string   `json:"mtls_mode"`
	VirtualServices  []string `json:"virtual_services"`
	DestinationRules []string `json:"destination_rules"`
	Validations      []string `json:"validations"`
}

// ServiceDetailsWorkloadFormatted is one workload in the formatted output.
type ServiceDetailsWorkloadFormatted struct {
	Name           string            `json:"name"`
	Kind           string            `json:"kind"`
	Labels         map[string]string `json:"labels"`
	ServiceAccount string            `json:"service_account"`
	PodCount       int               `json:"pod_count"`
}

// ServiceDetailsEndpointFormatted is one endpoint in the formatted output.
type ServiceDetailsEndpointFormatted struct {
	PodName string `json:"pod_name"`
	IP      string `json:"ip"`
}
