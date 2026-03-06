package kiali

// WorkloadDetailsRaw is the raw response from Kiali workload details API (for unmarshaling).
type WorkloadDetailsRaw struct {
	Name                string                                `json:"name"`
	Namespace           string                                `json:"namespace"`
	Cluster             string                                `json:"cluster"`
	GVK                 WorkloadDetailsGVK                    `json:"gvk"`
	CreatedAt           string                                `json:"createdAt"`
	Labels              map[string]string                     `json:"labels"`
	ServiceAccountNames []string                              `json:"serviceAccountNames"`
	DesiredReplicas     int                                   `json:"desiredReplicas"`
	CurrentReplicas     int                                   `json:"currentReplicas"`
	AvailableReplicas   int                                   `json:"availableReplicas"`
	Pods                []WorkloadDetailsPodRaw               `json:"pods"`
	Services            []WorkloadDetailsServiceRef           `json:"services"`
	Validations         map[string]map[string]ValidationEntry `json:"validations"`
	Health              WorkloadDetailsHealthRaw              `json:"health"`
	IstioSidecar        bool                                  `json:"istioSidecar"`
	IsAmbient           bool                                  `json:"isAmbient"`
}

// WorkloadDetailsGVK is the GVK for the workload.
type WorkloadDetailsGVK struct {
	Group   string `json:"Group"`
	Version string `json:"Version"`
	Kind    string `json:"Kind"`
}

// WorkloadDetailsPodRaw is a pod in the workload details.
type WorkloadDetailsPodRaw struct {
	Name                string                        `json:"name"`
	Status              string                        `json:"status"`
	Containers          []WorkloadDetailsContainerRaw `json:"containers"`
	IstioInitContainers []WorkloadDetailsContainerRaw `json:"istioInitContainers"`
	ProxyStatus         map[string]string             `json:"proxyStatus"`
}

// WorkloadDetailsContainerRaw is a container in a pod.
type WorkloadDetailsContainerRaw struct {
	Name    string `json:"name"`
	Image   string `json:"image"`
	IsProxy bool   `json:"isProxy"`
	IsReady bool   `json:"isReady"`
}

// WorkloadDetailsServiceRef is a service reference in workload details.
type WorkloadDetailsServiceRef struct {
	Name string `json:"name"`
}

// WorkloadDetailsHealthRaw is health in the workload details.
type WorkloadDetailsHealthRaw struct {
	Requests struct {
		Inbound  map[string]map[string]float64 `json:"inbound"`
		Outbound map[string]map[string]float64 `json:"outbound"`
	} `json:"requests"`
	Status struct {
		Status string `json:"status"`
	} `json:"status"`
}

// --- Formatted output types ---

// WorkloadDetailsFormatted is the transformed workload details returned to callers.
type WorkloadDetailsFormatted struct {
	Workload           WorkloadDetailsWorkloadFormatted `json:"workload"`
	Status             WorkloadDetailsStatusFormatted   `json:"status"`
	Istio              WorkloadDetailsIstioFormatted    `json:"istio"`
	Pods               []WorkloadDetailsPodFormatted    `json:"pods"`
	AssociatedServices []string                         `json:"associated_services"`
}

// WorkloadDetailsWorkloadFormatted is the workload section of the formatted output.
type WorkloadDetailsWorkloadFormatted struct {
	Name           string            `json:"name"`
	Namespace      string            `json:"namespace"`
	Kind           string            `json:"kind"`
	Labels         map[string]string `json:"labels"`
	ServiceAccount string            `json:"service_account"`
	CreatedAt      string            `json:"created_at"`
}

// WorkloadDetailsStatusFormatted is the status section.
type WorkloadDetailsStatusFormatted struct {
	Overall            string                           `json:"overall"`
	Replicas           WorkloadDetailsReplicasFormatted `json:"replicas"`
	TrafficSuccessRate WorkloadDetailsTrafficFormatted  `json:"traffic_success_rate"`
}

// WorkloadDetailsReplicasFormatted is replicas counts.
type WorkloadDetailsReplicasFormatted struct {
	Desired   int `json:"desired"`
	Current   int `json:"current"`
	Available int `json:"available"`
}

// WorkloadDetailsTrafficFormatted is inbound/outbound success rates.
type WorkloadDetailsTrafficFormatted struct {
	Inbound  string `json:"inbound"`
	Outbound string `json:"outbound"`
}

// WorkloadDetailsIstioFormatted is the istio section.
type WorkloadDetailsIstioFormatted struct {
	Mode         string            `json:"mode"`
	ProxyVersion string            `json:"proxy_version"`
	SyncStatus   map[string]string `json:"sync_status"`
	Validations  []string          `json:"validations"`
}

// WorkloadDetailsPodFormatted is one pod in the formatted output.
type WorkloadDetailsPodFormatted struct {
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	Containers []string `json:"containers"`
	IstioInit  string   `json:"istio_init"`
	IstioProxy string   `json:"istio_proxy"`
}
